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
	// JobTTLSeconds is a default ttl for a job
	JobTTLSeconds = 60 * 60 * 48
	// ActiveDeadlineSeconds is an active deadline for a job to run
	// 8 hours is default to avoid crashing jobs
	ActiveDeadlineSeconds = 60 * 60 * 4
	// DefaultNamespace is a default kubernetes namespace
	DefaultNamespace = "default"
	KindJob          = "Job"
	KindPod          = "Pod"
)

// Job is a simplified job spec
type Job struct {
	// Completions specifies job completions,
	// Force's default is 1
	Completions           int
	ActiveDeadlineSeconds int
	// TTLSeconds provides auto cleanup of the job
	TTLSeconds int
	Name       force.StringVar
	Namespace  force.StringVar
	Containers []Container
}

// CheckAndSetDefaults checks and sets defaults
func (j *Job) CheckAndSetDefaults(ctx force.ExecutionContext) error {
	if j.Name == nil || j.Name.Value(ctx) == "" {
		return trace.BadParameter("specify a job name")
	}
	if j.TTLSeconds == 0 {
		// 48 hours to clean up old jobs
		j.TTLSeconds = JobTTLSeconds
	}
	if j.ActiveDeadlineSeconds == 0 {
		// 48 hours to clean up old jobs
		j.ActiveDeadlineSeconds = ActiveDeadlineSeconds
	}
	if j.Namespace == nil {
		j.Namespace = force.String(DefaultNamespace)
	}
	if len(j.Containers) == 0 {
		return trace.BadParameter("the job needs at least one container")
	}
	for i := range j.Containers {
		if err := j.Containers[i].CheckAndSetDefaults(ctx); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// Spec returns kubernetes version of the job spec
func (j *Job) Spec(ctx force.ExecutionContext) *batchv1.Job {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      j.Name.Value(ctx),
			Namespace: j.Namespace.Value(ctx),
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
	for _, c := range j.Containers {
		job.Spec.Template.Spec.Containers = append(job.Spec.Template.Spec.Containers, c.Spec(ctx))
	}
	return job
}

// Container is a container to run
type Container struct {
	Name    force.StringVar
	Image   force.StringVar
	Command []force.StringVar
}

func (c *Container) CheckAndSetDefaults(ctx force.ExecutionContext) error {
	if c.Image == nil || c.Image.Value(ctx) == "" {
		return trace.BadParameter("specify Container{Image: ``}")
	}
	if c.Name == nil || c.Name.Value(ctx) == "" {
		return trace.BadParameter("specify Container{Name: ``}")
	}
	return nil
}

// Spec returns kubernetes spec
func (c *Container) Spec(ctx force.ExecutionContext) corev1.Container {
	return corev1.Container{
		Name:    c.Name.Value(ctx),
		Image:   c.Image.Value(ctx),
		Command: force.EvalStringVars(ctx, c.Command),
	}
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
				log.Infof("Completed: %v.", success.Message)
				return nil
			}
			if failure := findFailure(*job); failure != nil {
				log.Errorf("Failed: %v.", failure.Message)
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
		return fmt.Sprintf("Pod %q in namespace %q", val.Name, val.Namespace)
	case *batchv1.Job:
		return fmt.Sprintf("Job %q in namespace %q", val.Name, val.Namespace)
	case corev1.Pod:
		return fmt.Sprintf("Pod %q in namespace %q", val.Name, val.Namespace)
	case batchv1.Job:
		return fmt.Sprintf("Job %q in namespace %q", val.Name, val.Namespace)
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

// NamespaceOrDefault returns a default namespace if the specified namespace is empty
func Namespace(namespace string) string {
	if namespace == "" {
		return DefaultNamespace
	}
	return namespace
}
