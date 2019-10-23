package kube

import (
	"context"
	"io"
	"strings"

	"github.com/gravitational/force"

	"github.com/gravitational/force/pkg/retry"

	"github.com/cenkalti/backoff"
	"github.com/gravitational/trace"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	watch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// NewRun specifies new job runners
type NewRun struct {
}

// NewRun returns functions creating kubernetes job runner action
func (n *NewRun) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(job interface{}) force.Action {
		return &RunAction{job: job}
	}
}

// RunAction runs kubernetes job to it's completion
type RunAction struct {
	job interface{}
}

func (r *RunAction) Type() interface{} {
	return 0
}

// Eval runs kubernetes job
func (r *RunAction) Eval(ctx force.ExecutionContext) (interface{}, error) {
	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return nil, trace.BadParameter("initialize kube plugin")
	}
	plugin := pluginI.(*Plugin)

	var spec batchv1.Job
	if err := force.EvalInto(ctx, r.job, &spec); err != nil {
		return nil, trace.Wrap(err)
	}
	if err := checkAndSetJobDefaults(&spec); err != nil {
		return nil, trace.Wrap(err)
	}
	log := force.Log(ctx)

	jobs := plugin.client.BatchV1().Jobs(spec.Namespace)
	job, err := jobs.Create(&spec)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	log.Infof("Created job %v in namespace %v.", spec.Name, spec.Namespace)
	writer := force.Writer(log.AddFields(map[string]interface{}{"job": job.Name}))
	defer writer.Close()

	// waitCtx will get cancelled once job is done
	// signalling that stream logs should gracefully wrap up the operations
	waitCtx, waitCancel := context.WithCancel(ctx)
	defer waitCancel()

	waitC := make(chan error, 1)
	go func() {
		defer waitCancel()
		waitC <- r.wait(ctx, plugin.client, *job)
	}()

	// wait for stream logs to finish, so it can capture all the available logs
	err = r.streamLogs(ctx, waitCtx, plugin.client, *job, writer)
	if err != nil {
		// report the error, but return job status returned by wait
		log.WithError(err).Warningf("Streaming logs for %v has failed.", job.Name)
	}

	select {
	case err := <-waitC:
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return 0, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// jobCheckAndSetDefaults checks and sets defaults
func jobCheckAndSetDefaults(j *batchv1.Job) error {
	if j.Spec.BackoffLimit == nil {
		i := int32(0)
		j.Spec.BackoffLimit = &i
	}
	if j.Spec.TTLSecondsAfterFinished == nil {
		i := int32(JobTTLSeconds)
		// 48 hours to clean up old jobs
		j.Spec.TTLSecondsAfterFinished = &i
	}
	if j.Spec.ActiveDeadlineSeconds == nil {
		i := int64(ActiveDeadlineSeconds)
		j.Spec.ActiveDeadlineSeconds = &i
	}
	if j.Namespace == "" {
		j.Namespace = DefaultNamespace
	}
	if len(j.Spec.Template.Spec.Containers) == 0 {
		return trace.BadParameter("the job needs at least one container")
	}
	return nil
}

// MarshalCode marshals the action into code representation
func (s *RunAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		FnName:  KeyRun,
		Args:    []interface{}{s.job},
	}
	return call.MarshalCode(ctx)
}

// streamLogs streams logs until the job is either failed or done, the context
// ctx should cancel whenever the job is done
func (r *RunAction) streamLogs(ctx context.Context, jobCtx context.Context, client *kubernetes.Clientset, job batchv1.Job, out io.Writer) error {
	// watches is a list of active watches
	var watches []context.Context
	interval := retry.NewUnlimitedExponentialBackOff()
	err := retry.WithInterval(ctx, interval, func() error {
		watcher, err := r.newPodWatch(client, job)
		if err != nil {
			return &backoff.PermanentError{err}
		}
		defer watcher.Stop()
		watches, err = r.monitorPods(ctx, client, watcher.ResultChan(), jobCtx, job, out, watches)
		if err != nil && !trace.IsRetryError(err) {
			return &backoff.PermanentError{Err: err}
		}
		return trace.Wrap(err)
	})
	return trace.Wrap(err)
}

// Wait waits for job to complete or fail, cancel on the context cancels
// the wait call that is otherwise blocking
func (r *RunAction) wait(ctx context.Context, client *kubernetes.Clientset, job batchv1.Job) error {
	interval := retry.NewUnlimitedExponentialBackOff()
	err := retry.WithInterval(ctx, interval, func() error {
		watcher, err := r.newJobWatch(client, job)
		if err != nil {
			return &backoff.PermanentError{Err: err}
		}
		err = evalJobStatus(ctx, watcher.ResultChan())
		watcher.Stop()
		if err != nil && !trace.IsRetryError(err) {
			return &backoff.PermanentError{Err: err}
		}
		return trace.Wrap(err)
	})
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func (r *RunAction) newJobWatch(client *kubernetes.Clientset, job batchv1.Job) (watch.Interface, error) {
	jobs := client.BatchV1().Jobs(job.Namespace)
	watcher, err := jobs.Watch(metav1.ListOptions{
		TypeMeta: metav1.TypeMeta{
			Kind: job.Kind,
		},
		FieldSelector: fields.Set{"metadata.name": job.Name}.String(),
		Watch:         true,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return watcher, nil
}

func (r *RunAction) newPodWatch(client *kubernetes.Clientset, job batchv1.Job) (watch.Interface, error) {
	pods := client.CoreV1().Pods(job.Namespace)
	watcher, err := pods.Watch(metav1.ListOptions{
		TypeMeta: metav1.TypeMeta{
			Kind: KindPod,
		},
		LabelSelector: labels.Set{"job-name": job.Name}.String(),
		Watch:         true,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return watcher, nil
}

func (r *RunAction) monitorPods(ctx context.Context, client *kubernetes.Clientset, eventsC <-chan watch.Event, jobCtx context.Context, job batchv1.Job, w io.Writer, watches []context.Context) ([]context.Context, error) {
	// podSet keeps state of currently monitored pods
	podSet := map[string]corev1.Pod{}

	log := force.Log(ctx)
	var err error
	watches, err = r.checkJob(ctx, client, job, podSet, watches, w)
	if err == nil {
		return watches, nil
	}
	for {
		select {
		case _, ok := <-eventsC:
			if !ok {
				return watches, trace.Retry(nil, "events channel closed")
			}
			watches, err = r.checkJob(ctx, client, job, podSet, watches, w)
			if err == nil {
				return watches, nil
			} else if !trace.IsCompareFailed(err) {
				log.WithError(err).Warningf("Job %v has failed.", job.Name)
			}
			// global context signalled exit
		case <-ctx.Done():
			return watches, nil
			// stop watching for new job events if job is done
		case <-jobCtx.Done():
			for _, w := range watches {
				select {
				// if global context done, don't wait
				case <-ctx.Done():
					return watches, ctx.Err()
					// otherwise, gracefully wait for all streams to complete
				case <-w.Done():
				}
			}
			return watches, nil
		}
	}
}

// checkJob checks job for new pods arrivals and returns job status
func (r *RunAction) checkJob(ctx context.Context, client *kubernetes.Clientset, job batchv1.Job, podSet map[string]corev1.Pod, watches []context.Context, out io.Writer) ([]context.Context, error) {
	firstRun := len(watches) == 0
	newSet, err := r.collectPods(client, job)
	if err != nil {
		return watches, trace.Wrap(err)
	}
	diffs := diffPodSets(podSet, newSet)
	for _, diff := range diffs {
		pod := *diff.new
		// record new version of the pod state
		podSet[pod.Name] = pod
		for _, containerDiff := range diff.containers {
			// stream logs for running containers, or if the first run,
			// output logs anyways
			if containerDiff.new.State.Running != nil || (firstRun && containerDiff.new.State.Terminated != nil) {
				watchCtx, watchCancel := context.WithCancel(ctx)
				watches = append(watches, watchCtx)
				go func() {
					defer watchCancel()
					r.streamPodContainerLogs(ctx, client, pod, *containerDiff.new, out)
				}()
			}
		}
	}
	return watches, r.getJobStatus(client, job)
}

// collectPods collects pods created by this job and returns map
// with podName: pod pairs
func (r *RunAction) collectPods(client *kubernetes.Clientset, job batchv1.Job) (map[string]corev1.Pod, error) {
	set := podSelector(job)
	podList, err := client.CoreV1().Pods(job.Namespace).List(metav1.ListOptions{
		LabelSelector: set.AsSelector().String(),
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	pods := make(map[string]corev1.Pod)
	for _, pod := range podList.Items {
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == KindJob && ref.UID == job.UID {
				pods[pod.Name] = pod
			}
		}
	}

	return pods, nil
}

// streamPodContainerLogs attempts to stream pod logs to the provided out writer
func (r *RunAction) streamPodContainerLogs(ctx context.Context, client *kubernetes.Clientset, pod corev1.Pod, container corev1.ContainerStatus, out io.Writer) error {
	log := force.Log(ctx)
	req := client.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: container.Name,
		Follow:    true,
	})
	readCloser, err := req.Stream()
	if err != nil {
		return trace.Wrap(err)
	}
	localContext, localCancel := context.WithCancel(ctx)
	go func() {
		defer localCancel()
		_, err := io.Copy(out, readCloser)
		if err != nil && !IsStreamClosedError(err) {
			log.Warningf("Failed to complete copy: %v.", trace.DebugReport(err))
		}
	}()
	<-localContext.Done()
	// we are closing reader on local completion or higher level cancel
	// depending on what arrives first
	readCloser.Close()
	return nil
}

func (r *RunAction) getJobStatus(client *kubernetes.Clientset, ref batchv1.Job) error {
	jobs := client.BatchV1().Jobs(ref.Namespace)
	job, err := jobs.Get(ref.Name, metav1.GetOptions{})
	if err != nil {
		return trace.Wrap(err)
	}

	succeeded := job.Status.Succeeded
	active := job.Status.Active
	var complete bool
	if job.Spec.Completions == nil {
		// This type of job is complete when any pod exits with success
		if succeeded > 0 && active == 0 {
			complete = true
		}
	} else {
		// Job specifies a number of completions
		completions := *job.Spec.Completions
		if succeeded >= completions {
			complete = true
		}
	}

	if !complete {
		return trace.CompareFailed("job %v not yet complete (succeeded: %v, active: %v)",
			formatMeta(job.ObjectMeta), succeeded, active)
	}
	return nil
}

func podSelector(job batchv1.Job) labels.Set {
	var selector map[string]string
	if job.Spec.Selector != nil {
		selector = job.Spec.Selector.MatchLabels
	}
	set := make(labels.Set)
	for key, val := range selector {
		set[key] = val
	}
	return set
}

// IsStreamClosedError determines if the given error is a response/stream closed
// error
func IsStreamClosedError(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case trace.Unwrap(err) == io.EOF:
		return true
	case IsClosedResponseBodyErrorMessage(err.Error()):
		return true
	}
	return false
}

// IsClosedResponseBodyErrorMessage determines if the error message
// describes a closed response body error
func IsClosedResponseBodyErrorMessage(err string) bool {
	return strings.HasSuffix(err, "response body closed")
}
