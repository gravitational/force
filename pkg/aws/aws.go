package aws

import (
	"github.com/gravitational/force"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	awssession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gravitational/trace"
)

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

// Setup returns a function that sets up build plugin
func Setup(cfg Config) force.SetupFunc {
	return func(group force.Group) error {
		sess, err := cfg.CheckAndSetDefaults()
		if err != nil {
			return trace.Wrap(err)
		}
		p := &Plugin{
			cfg:  cfg,
			sess: sess,
		}
		group.SetPlugin(Key, p)
		return nil
	}
}

// Plugin is a new logging plugin
type Plugin struct {
	cfg  Config
	sess *awssession.Session
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
