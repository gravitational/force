package kube

import (
	"github.com/gravitational/trace"
	corev1 "k8s.io/api/core/v1"
)

// Container is a container to run
type Container struct {
	Name            string
	Image           string
	Command         []string
	WorkingDir      string
	Env             []EnvVar
	VolumeMounts    []VolumeMount
	ImagePullPolicy string
	TTY             bool
	Stdin           bool
	SecurityContext *SecurityContext
}

func (c *Container) CheckAndSetDefaults() error {
	if c.Image == "" {
		return trace.BadParameter("specify Container{Image: ``}")
	}

	if c.Name == "" {
		return trace.BadParameter("specify Container{Name: ``}")
	}
	if c.ImagePullPolicy == "" {
		c.ImagePullPolicy = string(corev1.PullAlways)
	}
	if c.SecurityContext != nil {
		if err := c.SecurityContext.CheckAndSetDefaults(); err != nil {
			return trace.Wrap(err)
		}
	}
	for _, e := range c.Env {
		if err := e.CheckAndSetDefaults(); err != nil {
			return trace.Wrap(err)
		}
	}
	for _, v := range c.VolumeMounts {
		if err := v.CheckAndSetDefaults(); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// Spec returns kubernetes spec
func (c *Container) Spec() (*corev1.Container, error) {
	out := &corev1.Container{
		Name:            c.Name,
		Image:           c.Image,
		Command:         c.Command,
		WorkingDir:      c.WorkingDir,
		ImagePullPolicy: corev1.PullPolicy(c.ImagePullPolicy),
		TTY:             c.TTY,
		Stdin:           c.Stdin,
	}
	if c.SecurityContext != nil {
		spec, err := c.SecurityContext.Spec()
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out.SecurityContext = spec
	}
	for _, e := range c.Env {
		spec, err := e.Spec()
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out.Env = append(out.Env, *spec)
	}
	for _, v := range c.VolumeMounts {
		spec, err := v.Spec()
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out.VolumeMounts = append(out.VolumeMounts, *spec)
	}
	return out, nil
}

// SecurityContext
type SecurityContext struct {
	Privileged bool
}

func (s *SecurityContext) CheckAndSetDefaults() error {
	return nil
}

func (s *SecurityContext) Spec() (*corev1.SecurityContext, error) {
	return &corev1.SecurityContext{
		Privileged: &s.Privileged,
	}, nil
}

type EnvVar struct {
	Name  string
	Value string
}

func (e *EnvVar) CheckAndSetDefaults() error {
	if e.Name == "" {
		return trace.BadParameter("specify EnvVar{Name: ``}")
	}
	return nil
}

func (e *EnvVar) Spec() (*corev1.EnvVar, error) {
	return &corev1.EnvVar{
		Name:  e.Name,
		Value: e.Value,
	}, nil
}

// VolumeMount describes a mounting of a Volume within a container.
type VolumeMount struct {
	Name      string
	ReadOnly  bool
	MountPath string
}

func (v *VolumeMount) CheckAndSetDefaults() error {
	if v.Name == "" {
		return trace.BadParameter("specify VolumeMount{Name: ``}")
	}
	if v.MountPath == "" {
		return trace.BadParameter("specify VolumeMount{MountPath: ``}")
	}
	return nil
}

func (v *VolumeMount) Spec() (*corev1.VolumeMount, error) {
	return &corev1.VolumeMount{
		Name:      v.Name,
		MountPath: v.MountPath,
	}, nil
}

// Volume represents a named volume in a pod that may be accessed by any container in the pod.
type Volume struct {
	Name      string
	EmptyDir  *EmptyDir
	Secret    *Secret
	ConfigMap *ConfigMap
}

type EmptyDir struct {
	Medium string
}

type Secret struct {
	Name string
}

type ConfigMap struct {
	Name string
}

func (v *Volume) CheckAndSetDefaults() error {
	if v.Name == "" {
		return trace.BadParameter("specify Volume{Name: ``}")
	}
	if v.EmptyDir == nil && v.Secret == nil && v.ConfigMap == nil {
		return trace.BadParameter("specify at least one volume source")
	}
	return nil
}

func (v *Volume) Spec() (*corev1.Volume, error) {
	volume := &corev1.Volume{
		Name: v.Name,
	}
	if v.EmptyDir != nil {
		volume.VolumeSource.EmptyDir = &corev1.EmptyDirVolumeSource{
			Medium: corev1.StorageMedium(v.EmptyDir.Medium),
		}
	}
	if v.Secret != nil {
		if v.Secret.Name == "" {
			return nil, trace.BadParameter("provide &Secret{Name: ``}")
		}
		volume.VolumeSource.Secret = &corev1.SecretVolumeSource{
			SecretName: v.Secret.Name,
		}
	}
	if v.ConfigMap != nil {
		if v.ConfigMap.Name == "" {
			return nil, trace.BadParameter("provide &ConfigMap{Name: ``}")
		}
		volume.VolumeSource.ConfigMap = &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: v.ConfigMap.Name,
			},
		}
	}
	return volume, nil
}
