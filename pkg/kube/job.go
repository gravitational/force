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
	Completions           int
	ActiveDeadlineSeconds int
	BackoffLimit          int
	// TTLSeconds provides auto cleanup of the job
	TTLSeconds      int
	Name            string
	Namespace       string
	Containers      []Container
	SecurityContext *PodSecurityContext
	Volumes         []Volume
}

type PodSecurityContext struct {
	RunAsUser  int
	RunAsGroup int
}

func (s *PodSecurityContext) CheckAndSetDefaults() error {
	return nil
}

func (s *PodSecurityContext) Spec() (*corev1.PodSecurityContext, error) {
	return &corev1.PodSecurityContext{
		RunAsUser:  force.PInt64(int64(s.RunAsUser)),
		RunAsGroup: force.PInt64(int64(s.RunAsGroup)),
	}, nil
}

// CheckAndSetDefaults checks and sets defaults
func (j *Job) CheckAndSetDefaults() error {
	if j.Name == "" {
		return trace.BadParameter("specify a job name")
	}
	if j.TTLSeconds == 0 {
		// 48 hours to clean up old jobs
		j.TTLSeconds = JobTTLSeconds
	}
	if j.ActiveDeadlineSeconds == 0 {
		j.ActiveDeadlineSeconds = ActiveDeadlineSeconds
	}
	if j.Namespace == "" {
		j.Namespace = DefaultNamespace
	}
	if len(j.Containers) == 0 {
		return trace.BadParameter("the job needs at least one container")
	}
	if j.SecurityContext != nil {
		if err := j.SecurityContext.CheckAndSetDefaults(); err != nil {
			return trace.Wrap(err)
		}
	}
	for i := range j.Containers {
		if err := j.Containers[i].CheckAndSetDefaults(); err != nil {
			return trace.Wrap(err)
		}
	}
	for i := range j.Volumes {
		if err := j.Volumes[i].CheckAndSetDefaults(); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// Spec returns kubernetes version of the job spec
func (j *Job) Spec() (*batchv1.Job, error) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      j.Name,
			Namespace: j.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
	if j.BackoffLimit != 0 {
		job.Spec.BackoffLimit = force.PInt32(int32(j.BackoffLimit))
	}
	if j.Completions != 0 {
		job.Spec.Completions = force.PInt32(int32(j.Completions))
	}
	if j.SecurityContext != nil {
		spec, err := j.SecurityContext.Spec()
		if err != nil {
			return nil, trace.Wrap(err)
		}
		job.Spec.Template.Spec.SecurityContext = spec
	}
	for _, c := range j.Containers {
		spec, err := c.Spec()
		if err != nil {
			return nil, trace.Wrap(err)
		}
		job.Spec.Template.Spec.Containers = append(job.Spec.Template.Spec.Containers, *spec)
	}
	for _, v := range j.Volumes {
		spec, err := v.Spec()
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
	return fmt.Sprintf("%v/%v", namespace(meta.Namespace), meta.Name)
}

// namespace returns a default namespace if the specified namespace is empty
func namespace(namespace string) string {
	if namespace == "" {
		return DefaultNamespace
	}
	return namespace
}
