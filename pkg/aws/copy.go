package aws

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// RecursiveCopy copies files between buckets from source to destination
// using directory as a source or destination
func RecursiveCopy(src interface{}, dest interface{}) (force.Action, error) {
	return makeCopy(src, dest, true)
}

// Copy copies files between buckets from source to destination
func Copy(src interface{}, dest interface{}) (force.Action, error) {
	return makeCopy(src, dest, false)
}

// makeCopy copies files between buckets from source to destination
func makeCopy(src interface{}, dest interface{}, recursive bool) (force.Action, error) {
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
		src:       src,
		dest:      dest,
		recursive: recursive,
	}, nil
}

type CopyAction struct {
	recursive bool
	src       interface{}
	dest      interface{}
}

func (s *CopyAction) Type() interface{} {
	return 0
}

func (s *CopyAction) Eval(ctx force.ExecutionContext) (interface{}, error) {
	log := force.Log(ctx)

	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return nil, trace.NotFound("initialize AWS plugin in the setup section")
	}
	plugin := pluginI.(*Plugin)

	src, err := force.EvalFromAST(ctx, s.src)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	dest, err := force.EvalFromAST(ctx, s.dest)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	switch source := src.(type) {
	case Local:
		destination, ok := dest.(S3)
		if !ok {
			return nil, trace.BadParameter("unsupported configuration, expected S3, got %T", dest)
		}
		if err := destination.CheckAndSetDefaults(); err != nil {
			return nil, trace.Wrap(err)
		}
		if err := source.CheckAndSetDefaults(); err != nil {
			return nil, trace.Wrap(err)
		}
		fi, err := os.Stat(source.Path)
		if err != nil {
			return nil, trace.ConvertSystemError(err)
		}
		start := time.Now()
		if fi.Mode().IsDir() {
			if !s.recursive {
				return nil, trace.BadParameter("path %v is a directory, use RecursiveCopy", source.Path)
			}
			err := uploadDir(ctx, plugin.sess, source.Path, destination)
			if err != nil {
				return nil, trace.Wrap(err)
			}
		} else {
			err := uploadFile(ctx, plugin.sess, source.Path, destination)
			if err != nil {
				return nil, trace.Wrap(err)
			}
		}
		diff := time.Now().Sub(start)
		log.Infof("Uploaded %v to %v in %v.", source.Path, destination, diff)
		return 0, nil
	case S3:
		if err := source.CheckAndSetDefaults(); err != nil {
			return nil, trace.Wrap(err)
		}
		destination, ok := dest.(Local)
		if !ok {
			return nil, trace.BadParameter("unsupported configuration, expected Local, got %T", dest)
		}
		if err := destination.CheckAndSetDefaults(); err != nil {
			return nil, trace.Wrap(err)
		}
		start := time.Now()
		fi, err := os.Stat(destination.Path)
		err = trace.ConvertSystemError(err)
		if err != nil {
			if !trace.IsNotFound(err) {
				return nil, err
			}
		}
		if fi != nil && fi.Mode().IsDir() {
			if !s.recursive {
				return nil, trace.BadParameter("path %v is a directory, use RecursiveCopy", destination.Path)
			}
			err := downloadDir(ctx, plugin.sess, source, destination)
			if err != nil {
				return nil, trace.Wrap(err)
			}
		} else {
			if err := downloadFile(ctx, plugin.sess, source, destination); err != nil {
				return nil, trace.Wrap(err)
			}
		}
		diff := time.Now().Sub(start)
		log.Infof("Downloaded %v to %v in %v.", source, destination, diff)
		return 0, nil
	default:
		return nil, trace.BadParameter("unsupported type %T", src)
	}
}

func downloadDir(ctx context.Context, sess *awssession.Session, source S3, destination Local) error {
	svc := s3.New(sess)

	// Get the list of items (up to 1K for now)
	resp, err := svc.ListObjectsV2(&s3.ListObjectsV2Input{Bucket: aws.String(source.Bucket)})
	if err != nil {
		return ConvertS3Error(err)
	}

	for _, item := range resp.Contents {
		sourceFile := source
		sourceFile.Key = *item.Key
		destFile := destination
		destFile.Path = filepath.Join(destination.Path, *item.Key)
		if err := os.MkdirAll(filepath.Dir(destFile.Path), 0755); err != nil {
			return trace.ConvertSystemError(err)
		}
		if err := downloadFile(ctx, sess, sourceFile, destFile); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

func downloadFile(ctx context.Context, sess *awssession.Session, source S3, destination Local) error {
	writer, err := os.OpenFile(destination.Path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(destination.Mode))
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	defer writer.Close()
	if err := download(ctx, sess, source, writer); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func uploadDir(ctx context.Context, sess *awssession.Session, dirPath string, destination S3) error {
	var relPaths []string
	filepath.Walk(dirPath, func(path string, fi os.FileInfo, err error) error {
		if fi.Mode().IsRegular() {
			relPath, err := filepath.Rel(dirPath, path)
			if err != nil {
				return trace.Wrap(err)
			}
			relPaths = append(relPaths, relPath)
		}
		return nil
	})
	for _, relPath := range relPaths {
		destKey := destination
		destKey.Key = filepath.Join(destination.Key, relPath)
		if err := uploadFile(ctx, sess, filepath.Join(dirPath, relPath), destKey); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

func uploadFile(ctx context.Context, sess *awssession.Session, path string, destination S3) error {
	reader, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	defer reader.Close()
	if err := upload(ctx, sess, reader, destination); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// MarshalCode marshals the action into code representation
func (s *CopyAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		Args:    []interface{}{s.src, s.dest},
	}
	if s.recursive {
		call.Fn = Copy
	} else {
		call.Fn = RecursiveCopy
	}
	return call.MarshalCode(ctx)
}

// String returns a copy of the software
func (s *CopyAction) String() string {
	return fmt.Sprintf("Copy(recursive=%v)", s.recursive)
}

// upload uploads object to S3 bucket, reads the contents of the object from reader
// and returns the target S3 bucket path in case of successful upload.
func upload(ctx context.Context, session *awssession.Session, reader io.Reader, dest S3) error {
	uploader := s3manager.NewUploader(session)
	input := &s3manager.UploadInput{
		Bucket: aws.String(dest.Bucket),
		Key:    aws.String(dest.Key),
		Body:   reader,
	}
	if dest.ServerSideEncryption != "" {
		input.ServerSideEncryption = aws.String(dest.ServerSideEncryption)
	}
	_, err := uploader.UploadWithContext(ctx, input)
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
