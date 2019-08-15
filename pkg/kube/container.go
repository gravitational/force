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
	image, err := force.EvalString(ctx, c.Image)
	if err != nil {
		return trace.Wrap(err)
	}
	if image == "" {
		return trace.BadParameter("specify Container{Image: ``}")
	}

	name, err := force.EvalString(ctx, c.Name)
	if err != nil {
		return trace.Wrap(err)
	}
	if name == "" {
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
func (c *Container) Spec(ctx force.ExecutionContext) (*corev1.Container, error) {
	name, err := c.Name.Eval(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	image, err := c.Image.Eval(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	command, err := force.EvalStringVars(ctx, c.Command)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	workingDir, err := force.EvalString(ctx, c.WorkingDir)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	pullPolicy, err := force.EvalString(ctx, c.ImagePullPolicy)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	tty, err := force.EvalBool(ctx, c.TTY)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	stdin, err := force.EvalBool(ctx, c.Stdin)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	out := &corev1.Container{
		Name:            name,
		Image:           image,
		Command:         command,
		WorkingDir:      workingDir,
		ImagePullPolicy: corev1.PullPolicy(pullPolicy),
		TTY:             tty,
		Stdin:           stdin,
	}
	for _, e := range c.Env {
		spec, err := e.Spec(ctx)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out.Env = append(out.Env, *spec)
	}
	for _, v := range c.VolumeMounts {
		spec, err := v.Spec(ctx)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out.VolumeMounts = append(out.VolumeMounts, *spec)
	}
	return out, nil
}

type EnvVar struct {
	Name  force.StringVar
	Value force.StringVar
}

func (e *EnvVar) CheckAndSetDefaults(ctx force.ExecutionContext) error {
	name, err := force.EvalString(ctx, e.Name)
	if err != nil {
		return trace.Wrap(err)
	}
	if name == "" {
		return trace.BadParameter("specify EnvVar{Name: ``}")
	}
	return nil
}

func (e *EnvVar) Spec(ctx force.ExecutionContext) (*corev1.EnvVar, error) {
	name, err := force.EvalString(ctx, e.Name)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	val, err := force.EvalString(ctx, e.Value)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &corev1.EnvVar{
		Name:  name,
		Value: val,
	}, nil
}

// VolumeMount describes a mounting of a Volume within a container.
type VolumeMount struct {
	Name      force.StringVar
	ReadOnly  bool
	MountPath force.StringVar
}

func (v *VolumeMount) CheckAndSetDefaults(ctx force.ExecutionContext) error {
	name, err := force.EvalString(ctx, v.Name)
	if err != nil {
		return trace.Wrap(err)
	}
	if name == "" {
		return trace.BadParameter("specify VolumeMount{Name: ``}")
	}
	mountPath, err := force.EvalString(ctx, v.MountPath)
	if err != nil {
		return trace.Wrap(err)
	}
	if mountPath == "" {
		return trace.BadParameter("specify VolumeMount{MountPath: ``}")
	}
	return nil
}

func (v *VolumeMount) Spec(ctx force.ExecutionContext) (*corev1.VolumeMount, error) {
	name, err := force.EvalString(ctx, v.Name)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	mountPath, err := force.EvalString(ctx, v.MountPath)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &corev1.VolumeMount{
		Name:      name,
		MountPath: mountPath,
	}, nil
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
	name, err := force.EvalString(ctx, v.Name)
	if err != nil {
		return trace.Wrap(err)
	}
	if name == "" {
		return trace.BadParameter("specify Volume{Name: ``}")
	}
	if v.EmptyDir == nil {
		return trace.BadParameter("specify EmptyDir, this is the only volume supported")
	}
	return nil
}

func (v *Volume) Spec(ctx force.ExecutionContext) (*corev1.Volume, error) {
	name, err := v.Name.Eval(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	medium, err := force.EvalString(ctx, v.EmptyDir.Medium)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium: corev1.StorageMedium(medium),
			},
		},
	}, nil
}
