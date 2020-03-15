package main

import (
	"fmt"

	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/kube"
	"github.com/gravitational/force/pkg/runner"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// This example demonstrates a watcher set up to track
// any new commits on any branch in Github under path github
// and trigger the action
func main() {
	runner.Setup(
		kube.Setup(kube.Config{
			// Path is a path to kubeconfig,
			// which is optional, if not set,
			// force assumes that it is running inside kubernetes
			Path: runner.ExitWithoutEnv("KUBE_CREDENTIALS"),
		})).
		// specify process name
		Name("kube-apply").
		// Run those actions
		RunFunc(func(ctx force.ExecutionContext) error {
			job := batchv1.Job{}
			job.Name = fmt.Sprintf("hello-first-%v", ctx.ID())
			job.Namespace = "default"

			// set up a job container
			container := corev1.Container{
				Image:           "busybox",
				Name:            "busybox",
				Command:         []string{"/bin/sh", "-c", `echo "hello, first $GOCACHE" && sleep 10;`},
				SecurityContext: &corev1.SecurityContext{Privileged: kube.BoolPtr(true)},
				Env:             []corev1.EnvVar{{Name: "GOCACHE", Value: "/mnt/gocache"}},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "gocache", MountPath: "/mnt/gocache"},
				},
				Resources: corev1.ResourceRequirements{
					Requests: map[corev1.ResourceName]resource.Quantity{"cpu": kube.MustParseQuantity("300m")},
					Limits:   map[corev1.ResourceName]resource.Quantity{"cpu": kube.MustParseQuantity("400m")},
				},
			}
			job.Spec.Template.Spec.Containers = []corev1.Container{container}
			job.Spec.Template.Spec.Volumes = []corev1.Volume{{
				Name:         "gocache",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			}}

			return kube.Run(ctx, job)
		})
}
