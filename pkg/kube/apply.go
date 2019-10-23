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

// NewApply specifies new job runners
type NewApply struct {
}

// NewApply returns functions creating kubernetes job runner action
func (n *NewApply) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(objects ...interface{}) force.Action {
		return &ApplyAction{objects: objects}
	}
}

// ApplyAction runs kubernetes job to it's completion
type ApplyAction struct {
	objects []interface{}
}

// Type returns apply action type
func (r *ApplyAction) Type() interface{} {
	return 0
}

// Eval creates or updates K8s object
func (r *ApplyAction) Eval(ctx force.ExecutionContext) (interface{}, error) {
	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return nil, trace.BadParameter("initialize Kube plugin")
	}
	plugin := pluginI.(*Plugin)

	out := make([]interface{}, len(r.objects))
	for i := 0; i < len(r.objects); i++ {
		eval, err := force.EvalFromAST(ctx, r.objects[i])
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out[i] = eval
	}

	for _, iface := range out {
		switch obj := iface.(type) {
		case batchv1.Job:
			if err := r.applyJob(ctx, plugin.client, obj); err != nil {
				return nil, trace.Wrap(err)
			}
		case appsv1.Deployment:
			if err := r.applyDeployment(ctx, plugin.client, obj); err != nil {
				return nil, trace.Wrap(err)
			}
		case corev1.Service:
			if err := r.applyService(ctx, plugin.client, obj); err != nil {
				return nil, trace.Wrap(err)
			}
		default:
			return nil, trace.BadParameter("object %T is not supported", obj)
		}
	}

	return 0, nil
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

// MarshalCode marshals the action into code representation
func (s *ApplyAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		FnName:  KeyApply,
		Args:    s.objects,
	}
	return call.MarshalCode(ctx)
}
