# Description

`Makefiles` create an easy way to build targets and projects.

`.force` files define event-driven workflows: Github to Docker builds, webhooks to Kubernetes deployments.

There is no server, `force` runs as a standalone binary or Kubernetes deployment
always processing a single `.force` file.

## Installation

Current version is `0.0.21`.

*Install locally*

```bash
$ go install github.com/gravitational/force/tool/force
```

*Docker image*

```
docker pull gcr.io/kubeadm-167321/force:0.0.21
```

*Local Docker Builds*

To use local force's ability to run builds, install
[runc 1.0.0-rc8](https://github.com/opencontainers/runc/releases/tag/v1.0.0-rc8)
