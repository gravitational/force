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

// checkAndSetDefaults checks and sets defaults
func checkAndSetJobDefaults(j *batchv1.Job) error {
	if j.Name == "" {
		return trace.BadParameter("specify a job name")
	}
	if j.Kind == "" {
		j.Kind = KindJob
	}
	if len(j.Spec.Template.Spec.Containers) == 0 {
		return trace.BadParameter("the job needs at least one container")
	}
	// by default, do not retry the job
	if j.Spec.BackoffLimit == nil {
		j.Spec.BackoffLimit = Int32Ptr(0)
	}
	if j.Spec.Template.Spec.RestartPolicy == "" {
		j.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
	}
	if j.Spec.TTLSecondsAfterFinished == nil {
		// 48 hours to clean up old jobs
		j.Spec.TTLSecondsAfterFinished = Int32Ptr(JobTTLSeconds)
	}
	if j.Spec.ActiveDeadlineSeconds == nil {
		j.Spec.ActiveDeadlineSeconds = Int64Ptr(ActiveDeadlineSeconds)
	}
	if j.Namespace == "" {
		j.Namespace = DefaultNamespace
	}
	for i := range j.Spec.Template.Spec.Containers {
		if err := checkAndSetContainerDefaults(&j.Spec.Template.Spec.Containers[i]); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

func checkAndSetContainerDefaults(c *corev1.Container) error {
	if c.Image == "" {
		return trace.BadParameter("specify Container{Image: ``}")
	}
	if c.Name == "" {
		return trace.BadParameter("specify Container{Name: ``}")
	}
	if c.ImagePullPolicy == "" {
		c.ImagePullPolicy = corev1.PullAlways
	}
	return nil
}
