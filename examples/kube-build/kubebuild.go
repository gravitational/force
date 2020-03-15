package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/builder"
	"github.com/gravitational/force/pkg/kube"
	"github.com/gravitational/force/pkg/runner"

	_ "github.com/gravitational/force/internal/unshare"
	"github.com/gravitational/trace"
	"gopkg.in/alecthomas/kingpin.v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func main() {
	runner.Reexec()
	ctx := runner.SetupSignalHandlers()

	var debug bool
	app := kingpin.New("force", "Force is simple CI/CD tool")
	app.Flag("debug", "Turn on debugging level").Short('d').BoolVar(&debug)

	buildC := app.Command("build", "Runs a build job in kubernetes")
	runC := app.Command("run", "Runs a kubernetes build job").Default()

	cmd, err := app.Parse(os.Args[1:])
	runner.ExitIf(err)

	switch cmd {
	case buildC.FullCommand():
		buildInKubernetes(ctx)
	case runC.FullCommand():
		runBuildJob(ctx, "gcr.io/kubeadm-167321/kubebuild:latest", []string{"-d", "build"})
	}
}

// buildInKubernetes runs a build in kubernetes
func buildInKubernetes(ctx context.Context) {
	runner.SetupInCLI(ctx,
		builder.Setup(builder.Config{
			Server:     "gcr.io",
			Username:   "_json_key",
			SecretFile: "/var/secrets/google/force-creds.json",
		})).
		RunFunc(func(ctx force.ExecutionContext) error {
			image := "gcr.io/kubeadm-167321/hello:0.0.1"
			err := builder.Build(ctx, builder.Image{
				Dockerfile: "/mnt/test-dockerfile/Dockerfile",
				Tag:        image,
			})
			if err != nil {
				return trace.Wrap(err)
			}
			return trace.Wrap(builder.Push(ctx, builder.Image{Tag: image}))
		})
}

// runBuildJob runs
func runBuildJob(ctx context.Context, imageName string, args []string) {
	runner.SetupInCLI(ctx,
		kube.Setup(kube.Config{
			Path: runner.ExitWithoutEnv("KUBE_CREDENTIALS"),
		})).
		// specify process name
		Name("kube-build").
		// Run those actions
		RunFunc(func(ctx force.ExecutionContext) error {
			job := batchv1.Job{}
			job.Name = fmt.Sprintf("kbuild-%v", ctx.ID())
			job.Namespace = "default"

			// set up a job container
			container := corev1.Container{
				Image:   imageName,
				Name:    "build",
				Command: append([]string{"force"}, args...),
				SecurityContext: &corev1.SecurityContext{
					Privileged: kube.BoolPtr(true),
				},
				VolumeMounts: []corev1.VolumeMount{
					// tmp is for temporary directory, just in case
					{Name: "tmp", MountPath: "/tmp"},
					// cache is for container build cache
					{Name: "cache", MountPath: "/root/.local"},
					// creds is for google creds
					{Name: "creds", MountPath: "/var/secrets/google"},
					// test-dockerfile is a script with a dockerfile
					{Name: "test-dockerfile", MountPath: "/mnt/test-dockerfile"},
				},
				Resources: corev1.ResourceRequirements{
					Requests: map[corev1.ResourceName]resource.Quantity{"cpu": kube.MustParseQuantity("1000m")},
				},
			}
			job.Spec.Template.Spec.Containers = []corev1.Container{container}
			job.Spec.Template.Spec.Volumes = []corev1.Volume{
				{
					Name:         "cache",
					VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				},
				{
					Name:         "tmp",
					VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				},
				{
					Name:         "test-dockerfile",
					VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "test-dockerfile"}}},
				},
				{
					Name:         "creds",
					VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "creds"}},
				},
			}

			return kube.Run(ctx, job)
		})
}
