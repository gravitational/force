package kube

import (
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Scope returns a new scope with all the functions and structs
// defined, this is the entrypoint into plugin as far as force is concerned
func Scope() (force.Group, error) {
	scope := force.WithLexicalScope(nil)
	err := force.ImportStructsIntoAST(scope,
		reflect.TypeOf(Config{}),
		reflect.TypeOf(corev1.Service{}),
		reflect.TypeOf(batchv1.Job{}),
		reflect.TypeOf(appsv1.Deployment{}),
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	importedFunctions := []interface{}{
		Name,
	}
	for _, fn := range importedFunctions {
		outFn, err := force.ConvertFunctionToAST(fn)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		scope.AddDefinition(force.FunctionName(fn), outFn)
	}

	scope.AddDefinition(KeySetup, &Setup{})
	scope.AddDefinition(KeyRun, &NewRun{})
	scope.AddDefinition(KeyApply, &NewApply{})
	return scope, nil
}

var kubeNameCharset = regexp.MustCompile(`[^a-z0-9\-\.]`)

// Name converts string to a safe to use k8s resource
func Name(in string) (string, error) {
	if in == "" {
		return "", trace.BadParameter("empty resource names are not allowed")
	}
	out := kubeNameCharset.ReplaceAllString(strings.ToLower(in), "")
	if out == "" {
		return "", trace.BadParameter("empty resource names are not allowed")
	}
	return out, nil
}

// Namespace is a wrapper around string to namespace a variable
type Namespace string

const (
	// Key is a name of the github plugin variable
	Key      = Namespace("kube")
	KeySetup = "Setup"
	KeyRun   = "Run"
	KeyApply = "Apply"
)

// Config specifies kube plugin configuration
type Config struct {
	// Path is a path to kubernetes config file
	Path string
}

// CheckAndSetDefaults checks and sets defaults
func (cfg *Config) CheckAndSetDefaults(ctx force.ExecutionContext) error {
	return nil
}

// Setup creates new plugins
type Setup struct {
	cfg interface{}
}

// NewInstance returns a new kubernetes client bound to the process group
// and registers plugin within variable
func (n *Setup) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(cfg interface{}) force.Action {
		return &Setup{cfg: cfg}
	}
}

// MarshalCode marshals plugin to code representation
func (n *Setup) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		FnName:  KeySetup,
		Args:    []interface{}{n.cfg},
	}
	return call.MarshalCode(ctx)
}

func (n *Setup) Run(ctx force.ExecutionContext) error {
	var cfg Config
	if err := force.EvalInto(ctx, n.cfg, &cfg); err != nil {
		return trace.Wrap(err)
	}
	if err := cfg.CheckAndSetDefaults(ctx); err != nil {
		return trace.Wrap(err)
	}
	client, config, err := GetClient(cfg.Path)
	if err != nil {
		return trace.Wrap(err)
	}
	plugin := &Plugin{
		cfg:    cfg,
		client: client,
		config: config,
	}
	ctx.Process().Group().SetPlugin(Key, plugin)
	return nil
}

// Plugin is a new plugin
type Plugin struct {
	cfg    Config
	client *kubernetes.Clientset
	config *rest.Config
}

// ConvertError converts kubernetes client error to trace error
func ConvertError(err error) error {
	if err == nil {
		return nil
	}
	statusErr, ok := err.(*errors.StatusError)
	if !ok {
		return err
	}

	message := fmt.Sprintf("%v", err)
	if !isEmptyDetails(statusErr.ErrStatus.Details) {
		message = fmt.Sprintf("%v, details: %v", message, statusErr.ErrStatus.Details)
	}

	status := statusErr.Status()
	switch {
	case status.Code == http.StatusConflict && status.Reason == metav1.StatusReasonAlreadyExists:
		return trace.AlreadyExists(message)
	case status.Code == http.StatusNotFound:
		return trace.NotFound(message)
	case status.Code == http.StatusForbidden:
		return trace.AccessDenied(message)
	}
	return err
}

func isEmptyDetails(details *metav1.StatusDetails) bool {
	if details == nil {
		return true
	}

	if details.Name == "" && details.Group == "" && details.Kind == "" && len(details.Causes) == 0 {
		return true
	}
	return false
}
