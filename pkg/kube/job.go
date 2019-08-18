package kube

import (
	"context"
	"fmt"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	watch "k8s.io/apimachinery/pkg/watch"
)

//
const (
	// ActiveDeadlineSeconds is an active deadline for a job to run
	// 4 hours is default to avoid crashing jobs
	ActiveDeadlineSeconds = 60 * 60 * 4
	// JobTTLSeconds is a default ttl for jobs, before they will be garbage
	// collected
	JobTTLSeconds = ActiveDeadlineSeconds * 2
	// DefaultNamespace is a default kubernetes namespace
	DefaultNamespace = "default"
	KindJob          = "Job"
	KindPod          = "Pod"
)

// Job is a simplified job spec
type Job struct {
	// Completions specifies job completions,
	// Force's default is not set
	Completions           force.IntVar
	ActiveDeadlineSeconds force.IntVar
	BackoffLimit          force.IntVar
	// TTLSeconds provides auto cleanup of the job
	TTLSeconds      force.IntVar
	Name            force.StringVar
	Namespace       force.StringVar
	Containers      []Container
	SecurityContext *PodSecurityContext
	Volumes         []Volume
}

type PodSecurityContext struct {
	RunAsUser  force.IntVar
	RunAsGroup force.IntVar
}

func (s *PodSecurityContext) CheckAndSetDefaults(ctx force.ExecutionContext) error {
	return nil
}

func (s *PodSecurityContext) Spec(ctx force.ExecutionContext) (*corev1.PodSecurityContext, error) {
	runAsUser, err := force.EvalPInt64(ctx, s.RunAsUser)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	runAsGroup, err := force.EvalPInt64(ctx, s.RunAsGroup)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &corev1.PodSecurityContext{
		RunAsUser:  runAsUser,
		RunAsGroup: runAsGroup,
	}, nil
}

// CheckAndSetDefaults checks and sets defaults
func (j *Job) CheckAndSetDefaults(ctx force.ExecutionContext) error {
	name, err := force.EvalString(ctx, j.Name)
	if err != nil {
		return trace.Wrap(err)
	}
	if name == "" {
		return trace.BadParameter("specify a job name")
	}
	if j.BackoffLimit == nil {
		j.BackoffLimit = force.Int(0)
	}
	if j.TTLSeconds == nil {
		// 48 hours to clean up old jobs
		j.TTLSeconds = force.Int(JobTTLSeconds)
	}
	if j.ActiveDeadlineSeconds == nil {
		j.ActiveDeadlineSeconds = force.Int(ActiveDeadlineSeconds)
	}
	if j.Namespace == nil {
		j.Namespace = force.String(DefaultNamespace)
	}
	if len(j.Containers) == 0 {
		return trace.BadParameter("the job needs at least one container")
	}
	if j.SecurityContext != nil {
		if err := j.SecurityContext.CheckAndSetDefaults(ctx); err != nil {
			return trace.Wrap(err)
		}
	}
	for i := range j.Containers {
		if err := j.Containers[i].CheckAndSetDefaults(ctx); err != nil {
			return trace.Wrap(err)
		}
	}
	for i := range j.Volumes {
		if err := j.Volumes[i].CheckAndSetDefaults(ctx); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// Spec returns kubernetes version of the job spec
func (j *Job) Spec(ctx force.ExecutionContext) (*batchv1.Job, error) {
	name, err := force.EvalString(ctx, j.Name)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	namespace, err := force.EvalString(ctx, j.Namespace)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	backoffLimit, err := force.EvalPInt32(ctx, j.BackoffLimit)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	completions, err := force.EvalPInt32(ctx, j.Completions)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: backoffLimit,
			Completions:  completions,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
	if j.SecurityContext != nil {
		spec, err := j.SecurityContext.Spec(ctx)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		job.Spec.Template.Spec.SecurityContext = spec
	}
	for _, c := range j.Containers {
		spec, err := c.Spec(ctx)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		job.Spec.Template.Spec.Containers = append(job.Spec.Template.Spec.Containers, *spec)
	}
	for _, v := range j.Volumes {
		spec, err := v.Spec(ctx)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, *spec)
	}
	return job, nil
}

func evalJobStatus(ctx context.Context, eventsC <-chan watch.Event) error {
	log := force.Log(ctx)
	for {
		select {
		case event, ok := <-eventsC:
			if !ok {
				return trace.Retry(nil, "events channel closed")
			}
			log.Debugf(describe(event.Object))
			job, ok := event.Object.(*batchv1.Job)
			if !ok {
				log.Warningf("Unexpected resource type: %T?, expected %T.", event.Object, job)
				continue
			}
			if success := findSuccess(*job); success != nil {
				return nil
			}
			if failure := findFailure(*job); failure != nil {
				return trace.BadParameter(failure.Message)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// findSuccess finds condition that indicates job completion
func findSuccess(job batchv1.Job) *batchv1.JobCondition {
	for i := range job.Status.Conditions {
		condition := job.Status.Conditions[i]
		if condition.Type == batchv1.JobComplete {
			return &condition
		}
	}
	return nil
}

// findFailure returns failed condition if it's present
func findFailure(job batchv1.Job) *batchv1.JobCondition {
	for i := range job.Status.Conditions {
		condition := job.Status.Conditions[i]
		if condition.Type == batchv1.JobFailed {
			return &condition
		}
	}
	return nil
}

func describe(v interface{}) string {
	switch val := v.(type) {
	case *corev1.Pod:
		return fmt.Sprintf("Pod %v in namespace %v", val.Name, val.Namespace)
	case *batchv1.Job:
		return fmt.Sprintf("Job %v in namespace %v", val.Name, val.Namespace)
	case corev1.Pod:
		return fmt.Sprintf("Pod %v in namespace %v", val.Name, val.Namespace)
	case batchv1.Job:
		return fmt.Sprintf("Job %v in namespace %v", val.Name, val.Namespace)
	}
	return "<unknown>"
}

// formatMeta formats this meta as text
func formatMeta(meta metav1.ObjectMeta) string {
	if meta.Namespace == "" {
		return meta.Name
	}
	return fmt.Sprintf("%v/%v", Namespace(meta.Namespace), meta.Name)
}

// Namespace returns a default namespace if the specified namespace is empty
func Namespace(namespace string) string {
	if namespace == "" {
		return DefaultNamespace
	}
	return namespace
}
