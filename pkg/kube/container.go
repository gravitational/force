package kube

import (
	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	corev1 "k8s.io/api/core/v1"
)

// Container is a container to run
type Container struct {
	Name            force.StringVar
	Image           force.StringVar
	Command         []force.StringVar
	WorkingDir      force.StringVar
	Env             []EnvVar
	VolumeMounts    []VolumeMount
	ImagePullPolicy force.StringVar
	TTY             force.BoolVar
	Stdin           force.BoolVar
}

func (c *Container) CheckAndSetDefaults(ctx force.ExecutionContext) error {
	if c.Image == nil || c.Image.Value(ctx) == "" {
		return trace.BadParameter("specify Container{Image: ``}")
	}
	if c.Name == nil || c.Name.Value(ctx) == "" {
		return trace.BadParameter("specify Container{Name: ``}")
	}
	if c.ImagePullPolicy == nil {
		c.ImagePullPolicy = force.String(string(corev1.PullAlways))
	}
	for _, e := range c.Env {
		if err := e.CheckAndSetDefaults(ctx); err != nil {
			return trace.Wrap(err)
		}
	}
	for _, v := range c.VolumeMounts {
		if err := v.CheckAndSetDefaults(ctx); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// Spec returns kubernetes spec
func (c *Container) Spec(ctx force.ExecutionContext) corev1.Container {
	out := corev1.Container{
		Name:            c.Name.Value(ctx),
		Image:           c.Image.Value(ctx),
		Command:         force.EvalStringVars(ctx, c.Command),
		WorkingDir:      force.EvalString(ctx, c.WorkingDir),
		ImagePullPolicy: corev1.PullPolicy(force.EvalString(ctx, c.ImagePullPolicy)),
		TTY:             force.EvalBool(ctx, c.TTY),
		Stdin:           force.EvalBool(ctx, c.Stdin),
	}
	for _, e := range c.Env {
		out.Env = append(out.Env, e.Spec(ctx))
	}
	for _, v := range c.VolumeMounts {
		out.VolumeMounts = append(out.VolumeMounts, v.Spec(ctx))
	}
	return out
}

type EnvVar struct {
	Name  force.StringVar
	Value force.StringVar
}

func (e *EnvVar) CheckAndSetDefaults(ctx force.ExecutionContext) error {
	if force.EvalString(ctx, e.Name) == "" {
		return trace.BadParameter("specify EnvVar{Name: ``}")
	}
	return nil
}

func (e *EnvVar) Spec(ctx force.ExecutionContext) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  e.Name.Value(ctx),
		Value: force.EvalString(ctx, e.Value),
	}
}

// VolumeMount describes a mounting of a Volume within a container.
type VolumeMount struct {
	Name      force.StringVar
	ReadOnly  bool
	MountPath force.StringVar
}

func (v *VolumeMount) CheckAndSetDefaults(ctx force.ExecutionContext) error {
	if force.EvalString(ctx, v.Name) == "" {
		return trace.BadParameter("specify VolumeMount{Name: ``}")
	}
	if force.EvalString(ctx, v.MountPath) == "" {
		return trace.BadParameter("specify VolumeMount{MountPath: ``}")
	}
	return nil
}

func (v *VolumeMount) Spec(ctx force.ExecutionContext) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      v.Name.Value(ctx),
		MountPath: v.MountPath.Value(ctx),
	}
}

// Volume represents a named volume in a pod that may be accessed by any container in the pod.
type Volume struct {
	Name     force.StringVar
	EmptyDir *EmptyDir
}

type EmptyDir struct {
	Medium force.String
}

func (v *Volume) CheckAndSetDefaults(ctx force.ExecutionContext) error {
	if force.EvalString(ctx, v.Name) == "" {
		return trace.BadParameter("specify Volume{Name: ``}")
	}
	if v.EmptyDir == nil {
		return trace.BadParameter("specify EmptyDir, this is the only volume supported")
	}
	return nil
}

func (v *Volume) Spec(ctx force.ExecutionContext) corev1.Volume {
	return corev1.Volume{
		Name: v.Name.Value(ctx),
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium: corev1.StorageMedium(force.EvalString(ctx, v.EmptyDir.Medium)),
			},
		},
	}
}
