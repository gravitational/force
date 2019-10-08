package aws

import (
	"reflect"

	"github.com/gravitational/force"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	awssession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gravitational/trace"
)

// Scope returns a new scope with all the functions and structs
// defined, this is the entrypoint into plugin as far as force is concerned
func Scope() (force.Group, error) {
	scope := force.WithLexicalScope(nil)
	err := force.ImportStructsIntoAST(scope,
		reflect.TypeOf(Config{}),
		reflect.TypeOf(S3{}),
		reflect.TypeOf(Local{}),
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	scope.AddDefinition(force.FunctionName(Copy), &force.NopScope{Func: Copy})
	scope.AddDefinition(force.FunctionName(RecursiveCopy), &force.NopScope{Func: RecursiveCopy})
	scope.AddDefinition(force.StructName(reflect.TypeOf(Setup{})), &Setup{})
	return scope, nil
}

//Namespace is a wrapper around string to namespace a variable in the context
type Namespace string

// Key is a name of the plugin variable
const Key = Namespace("aws")

const (
	KeySetup  = "Setup"
	KeyConfig = "Config"
	SchemeS3  = "s3"
)

// Config is an ssh client configuration
type Config struct {
	// Region is S3 bucket region
	Region string
}

// CheckAndSetDefaults checks and sets default values
func (cfg *Config) CheckAndSetDefaults() (*awssession.Session, error) {
	// create an AWS session using default SDK behavior, i.e. it will interpret
	// the environment and ~/.aws directory just like an AWS CLI tool would:
	sess, err := awssession.NewSessionWithOptions(awssession.Options{
		SharedConfigState: awssession.SharedConfigEnable,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// override the default environment (region + credentials) with the values
	// from the YAML file:
	if cfg.Region != "" {
		sess.Config.Region = aws.String(cfg.Region)
	}
	return sess, nil
}

// Plugin is a new logging plugin
type Plugin struct {
	cfg  Config
	sess *awssession.Session
}

// Setup creates new instances of plugins
type Setup struct {
	cfg interface{}
}

// NewInstance returns a new instance of a plugin bound to group
func (n *Setup) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(cfg interface{}) (force.Action, error) {
		return &Setup{
			cfg: cfg,
		}, nil
	}
}

// MarshalCode marshals plugin setup to code
func (n *Setup) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := force.FnCall{
		Package: string(Key),
		FnName:  KeySetup,
		Args:    []interface{}{n.cfg},
	}
	return call.MarshalCode(ctx)
}

// Run sets up logging plugin for the instance group
func (n *Setup) Run(ctx force.ExecutionContext) error {
	var cfg Config
	if err := force.EvalInto(ctx, n.cfg, &cfg); err != nil {
		return trace.Wrap(err)
	}
	sess, err := cfg.CheckAndSetDefaults()
	if err != nil {
		return trace.Wrap(err)
	}
	p := &Plugin{
		cfg:  cfg,
		sess: sess,
	}
	ctx.Process().Group().SetPlugin(Key, p)
	return nil
}

// ConvertS3Error wraps S3 error and returns trace equivalent
func ConvertS3Error(err error, args ...interface{}) error {
	if err == nil {
		return nil
	}
	if aerr, ok := err.(awserr.Error); ok {
		switch aerr.Code() {
		case s3.ErrCodeNoSuchKey, s3.ErrCodeNoSuchBucket, s3.ErrCodeNoSuchUpload, "NotFound":
			return trace.NotFound(aerr.Error(), args...)
		case s3.ErrCodeBucketAlreadyExists, s3.ErrCodeBucketAlreadyOwnedByYou:
			return trace.AlreadyExists(aerr.Error(), args...)
		default:
			return trace.BadParameter(aerr.Error(), args...)
		}
	}
	return err
}
