package aws

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gravitational/force"

	"github.com/aws/aws-sdk-go/aws"

	awssession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/gravitational/trace"
)

// Local is a local path
type Local struct {
	Path string
	Mode int
}

func (l *Local) CheckAndSetDefaults() error {
	if l.Path == "" {
		return trace.BadParameter("set Local{Path:``} parameter")
	}
	if l.Mode == 0 {
		l.Mode = 0640
	}
	return nil
}

func (l Local) String() string {
	return fmt.Sprintf("file://%v", l.Path)
}

// S3 is S3 bucket source or destination
type S3 struct {
	Bucket               string
	Key                  string
	ServerSideEncryption string
}

func (s *S3) CheckAndSetDefaults() error {
	if s.Bucket == "" {
		return trace.BadParameter("set S3{Bucket:``} parameter")
	}
	if s.Key == "" {
		return trace.BadParameter("set S3{Key:``} parameter")
	}
	return nil
}

func (s S3) String() string {
	return fmt.Sprintf("s3://%v/%v", s.Bucket, s.Key)
}

// Copy copies files between buckets from source to destination
func Copy(src interface{}, dest interface{}) (force.Action, error) {
	zeroSrc, err := force.ZeroFromAST(src)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	zeroDest, err := force.ZeroFromAST(dest)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	var sourceIsLocal bool
	var destIsLocal bool
	var sourceIsRemote bool
	var destIsRemote bool
	switch zeroSrc.(type) {
	case Local:
		sourceIsLocal = true
	case S3:
		sourceIsRemote = true
	default:
		return nil, trace.BadParameter("either S3 or Local are supported for source")
	}
	switch zeroDest.(type) {
	case Local:
		destIsLocal = true
	case S3:
		destIsRemote = true
	default:
		return nil, trace.BadParameter("either S3 or Local are supported for destination")
	}

	if sourceIsLocal && destIsLocal {
		return nil, trace.BadParameter("both source and destination can't be local")
	}

	if sourceIsRemote && destIsRemote {
		return nil, trace.BadParameter("both source and destination can't be local")
	}

	return &CopyAction{
		src:  src,
		dest: dest,
	}, nil
}

type CopyAction struct {
	src  interface{}
	dest interface{}
}

func (s *CopyAction) Run(ctx force.ExecutionContext) error {
	log := force.Log(ctx)

	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return trace.NotFound("initialize ssh plugin in the setup section")
	}
	plugin := pluginI.(*Plugin)

	src, err := force.EvalFromAST(ctx, s.src)
	if err != nil {
		return trace.Wrap(err)
	}

	dest, err := force.EvalFromAST(ctx, s.dest)
	if err != nil {
		return trace.Wrap(err)
	}

	switch source := src.(type) {
	case Local:
		destination, ok := dest.(S3)
		if !ok {
			return trace.BadParameter("unsupported configuration, expected S3, got %T", dest)
		}
		if err := destination.CheckAndSetDefaults(); err != nil {
			return trace.Wrap(err)
		}
		if err := source.CheckAndSetDefaults(); err != nil {
			return trace.Wrap(err)
		}
		reader, err := os.OpenFile(source.Path, os.O_RDONLY, 0)
		if err != nil {
			return trace.ConvertSystemError(err)
		}
		defer reader.Close()
		start := time.Now()
		if err := upload(ctx, plugin.sess, reader, destination); err != nil {
			return trace.Wrap(err)
		}
		diff := time.Now().Sub(start)
		log.Infof("Uploaded %v to %v in %v.", source, destination, diff)
		return nil
	case S3:
		if err := source.CheckAndSetDefaults(); err != nil {
			return trace.Wrap(err)
		}
		destination, ok := dest.(Local)
		if !ok {
			return trace.BadParameter("unsupported configuration, expected Local, got %T", dest)
		}
		if err := destination.CheckAndSetDefaults(); err != nil {
			return trace.Wrap(err)
		}
		writer, err := os.OpenFile(destination.Path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(destination.Mode))
		if err != nil {
			return trace.ConvertSystemError(err)
		}
		defer writer.Close()
		start := time.Now()
		if err := download(ctx, plugin.sess, source, writer); err != nil {
			return trace.Wrap(err)
		}
		diff := time.Now().Sub(start)
		log.Infof("Downloaded %v to %v in %v.", source, destination, diff)
		return nil
	default:
		return trace.BadParameter("unsupported type %T", src)
	}
}

// MarshalCode marshals the action into code representation
func (s *CopyAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		Fn:      Copy,
		Args:    []interface{}{s.src, s.dest},
	}
	return call.MarshalCode(ctx)
}

func (s *CopyAction) String() string {
	return fmt.Sprintf("Copy()")
}

// upload uploads object to S3 bucket, reads the contents of the object from reader
// and returns the target S3 bucket path in case of successful upload.
func upload(ctx context.Context, session *awssession.Session, reader io.Reader, dest S3) error {
	uploader := s3manager.NewUploader(session)
	_, err := uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket:               aws.String(dest.Bucket),
		Key:                  aws.String(dest.Key),
		Body:                 reader,
		ServerSideEncryption: aws.String(dest.ServerSideEncryption),
	})
	if err != nil {
		return ConvertS3Error(err)
	}
	return nil
}

// download downloads recorded session from S3 bucket and writes the results
// into writer return trace.NotFound error is object is not found.
func download(ctx context.Context, session *awssession.Session, src S3, writer io.WriterAt) error {
	downloader := s3manager.NewDownloader(session)

	written, err := downloader.DownloadWithContext(ctx, writer, &s3.GetObjectInput{
		Bucket: aws.String(src.Bucket),
		Key:    aws.String(src.Key),
	})
	if err != nil {
		return ConvertS3Error(err)
	}
	if written == 0 {
		return trace.NotFound("bucket key is not found")
	}
	return nil
}
