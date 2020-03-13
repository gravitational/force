package main

import (
	"io/ioutil"

	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/aws"
	"github.com/gravitational/force/pkg/runner"
)

func main() {
	runner.Setup(
		aws.Setup(aws.Config{}),
	).RunFunc(func(ctx force.ExecutionContext) error {
		log := force.Log(ctx)
		log.Infof("Uploading the file aws.go")

		// Upload file from local location with KMS encryption
		err := aws.Copy(ctx, aws.Local{Path: "aws.go"}, aws.S3{
			Bucket:               "demo.gravitational.io",
			Key:                  "aws.go",
			ServerSideEncryption: "aws:kms",
		})
		if err != nil {
			return err
		}

		// Download file from bucket
		log.Infof("Downloading file")
		err = aws.Copy(
			ctx,
			aws.S3{Bucket: "demo.gravitational.io", Key: "aws.go"},
			aws.Local{Path: "/tmp/aws.go"})
		if err != nil {
			return err
		}

		log.Infof("Uploading to dir")
		err = aws.RecursiveCopy(ctx, aws.Local{Path: "."}, aws.S3{Bucket: "demo.gravitational.io", Key: "/"})
		if err != nil {
			return err
		}

		// Download the bucket into temp dir
		log.Infof("Downloading to dir")
		tempDir, err := ioutil.TempDir("", "")
		if err != nil {
			return err
		}
		return aws.RecursiveCopy(ctx, aws.S3{Bucket: "demo.gravitational.io", Key: "/tmp"}, aws.Local{Path: tempDir})
	})
}
