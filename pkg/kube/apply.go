package kube

import (
	"context"

	"github.com/gravitational/force"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/gravitational/trace"
)

func Apply(ctx force.ExecutionContext, objects ...interface{}) error {
	a := &ApplyAction{objects: objects}
	return a.Run(ctx)
}

// ApplyAction runs kubernetes job to it's completion
type ApplyAction struct {
	objects []interface{}
}

// Run creates or updates K8s object
func (r *ApplyAction) Run(ctx force.ExecutionContext) error {
	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return trace.BadParameter("initialize Kube plugin, use kube.Setup in Setup section")
	}
	plugin, ok := pluginI.(*Plugin)
	if !ok {
		return trace.BadParameter("initialize Kube plugin, use kube.Setup in Setup section")
	}

	for _, iface := range r.objects {
		switch obj := iface.(type) {
		case batchv1.Job:
			if err := r.applyJob(ctx, plugin.client, obj); err != nil {
				return trace.Wrap(err)
			}
		case appsv1.Deployment:
			if err := r.applyDeployment(ctx, plugin.client, obj); err != nil {
				return trace.Wrap(err)
			}
		case corev1.Service:
			if err := r.applyService(ctx, plugin.client, obj); err != nil {
				return trace.Wrap(err)
			}
		default:
			return trace.BadParameter("object %T is not supported", obj)
		}
	}

	return nil
}

func (r *ApplyAction) applyJob(ctx context.Context, client *kubernetes.Clientset, j batchv1.Job) error {
	if err := checkAndSetJobDefaults(&j); err != nil {
		return trace.Wrap(err)
	}
	jobs := client.BatchV1().Jobs(j.Namespace)
	_, err := jobs.Get(j.Name, metav1.GetOptions{})
	err = ConvertError(err)
	if err != nil {
		if !trace.IsNotFound(err) {
			return trace.Wrap(err)
		}
		_, err := jobs.Create(&j)
		err = ConvertError(err)
		if !trace.IsAlreadyExists(err) {
			return err
		}
	}
	_, err = jobs.Update(&j)
	return ConvertError(err)
}

func (r *ApplyAction) applyDeployment(ctx context.Context, client *kubernetes.Clientset, d appsv1.Deployment) error {
	deployments := client.AppsV1().Deployments(d.Namespace)
	_, err := deployments.Get(d.Name, metav1.GetOptions{})
	err = ConvertError(err)
	if err != nil {
		if !trace.IsNotFound(err) {
			return trace.Wrap(err)
		}
		_, err := deployments.Create(&d)
		err = ConvertError(err)
		if !trace.IsAlreadyExists(err) {
			return err
		}
	}
	_, err = deployments.Update(&d)
	return ConvertError(err)
}

func (r *ApplyAction) applyService(ctx context.Context, client *kubernetes.Clientset, s corev1.Service) error {
	services := client.CoreV1().Services(s.Namespace)
	current, err := services.Get(s.Name, metav1.GetOptions{})
	err = ConvertError(err)
	if err != nil {
		if !trace.IsNotFound(err) {
			return trace.Wrap(err)
		}
		_, err := services.Create(&s)
		err = ConvertError(err)
		if !trace.IsAlreadyExists(err) {
			return err
		}
	}
	s.Spec.ClusterIP = current.Spec.ClusterIP
	s.ResourceVersion = current.ResourceVersion
	_, err = services.Update(&s)
	return ConvertError(err)
}
