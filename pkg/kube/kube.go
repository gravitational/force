package kube

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var kubeNameCharset = regexp.MustCompile(`[^a-z0-9\-\.]`)

// BoolPtr returns bool pointer from bool
func BoolPtr(v bool) *bool {
	return &v
}

// Int32Ptr returns a pointer to int32
func Int32Ptr(i int32) *int32 {
	return &i
}

// Int64Ptr returns a pointer to int64
func Int64Ptr(i int64) *int64 {
	return &i
}

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

// MustParseQuantity parses resource quantity,
// panics on incorrect format
func MustParseQuantity(qs string) resource.Quantity {
	q, err := resource.ParseQuantity(qs)
	if err != nil {
		panic(err)
	}
	return q
}

// Namespace is a wrapper around string to namespace a variable
type Namespace string

const (
	// Key is a name of the github plugin variable
	Key = Namespace("kube")
)

// Config specifies kube plugin configuration
type Config struct {
	// Path is a path to kubernetes config file
	Path string
}

// CheckAndSetDefaults checks and sets defaults
func (cfg *Config) CheckAndSetDefaults() error {
	return nil
}

// Setup returns a function that sets up github plugin
func Setup(cfg Config) force.SetupFunc {
	return func(group force.Group) error {
		if err := cfg.CheckAndSetDefaults(); err != nil {
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
		group.SetPlugin(Key, plugin)
		return nil
	}
}

// Plugin is a kubernetes plugin
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
