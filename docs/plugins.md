# Plugins

`Force` supports AWS, Slack and Docker out of the box. To load and configure the plugin,
use the `setup.force` file with `Setup` directive:

```go
Setup(
    // place plugin setup calls here
)
```

## Logging

**Setting it up**

The `log.Setup` function loads log plugin and sets up up various outputs.
The example below configures stdout output and Google's stackdriver output,
the only two supported at the moment:

{go * ./docs/snippets/log/setup.force}

**Logging**

Once loaded, logging plugin's function `Infof` could be used in the scripts:

{go * ./docs/snippets/log/log.force}

```
$ force log.force
INFO [PLANET-1]  Hello, stackdriver! id:e211e690 proc:planet-1
```

The Stackdriver logs are added with label.id `id:e211e690`, where ID is a unique
short ID associated with every single process execution triggered by event.

Stackdriver's logs for this process could be accessed via link using `labels.id` as a filter:

`https://console.cloud.google.com/logs/viewer?advancedFilter=labels.id%3De211e690&interval=P1D`

## SSH

**Setting it up**

SSH plugin enables `ssh.Copy`, `ssh.Session` and `ssh.Command` functions.
To set it up, use the `ssh.Setup` function:

{go * ./docs/snippets/ssh/setup.force}

**Parallel copying and executing commands**

Once setup, use the `ssh.Copy` to copy files between machines, `ssh.Command` to run
individual commands, and `ssh.Session` to initiate parallel SSH sessions:

{go * ./docs/snippets/ssh/ssh.force}


## Github and Git

**Setting it up**

The plugins `git` and `github` provide access to raw git repositories and
high level GitHub functionality, accordingly.

The `github` plugin requires [access token](https://help.github.com/en/github/authenticating-to-github/creating-a-personal-access-token-for-the-command-line).
The `git` plugin could use SSH agent to authenticate, or explicitly provided ssh public keys or certificates.

{go * ./docs/snippets/github/setup.force}

**Watching PRs**

Here is an example of how to watch the pull request to master branch
and trigger the build process triggering the git clone action:

{go * ./docs/snippets/github/ci.force}

## Docker Image Builder

**Setting it up**

Force can build and publish Docker images using `builder` plugin

{go * ./docs/snippets/builder/setup.force}

To run `Docker` builds locally, only `Linux` is supported, additionally, 
`runc` tool `1.0.0-rc.8` or greater is required:

[runc 1.0.0-rc8](https://github.com/opencontainers/runc/releases/tag/v1.0.0-rc8)

**Dockerfile**

{bash * ./docs/snippets/builder/Dockerfile}

Notice the ` # syntax = docker/dockerfile:experimental` stanza in the begining
of this Dockerfile, that allows to mount secrets using:

```bash
RUN --mount=type=bind,target=/repo --mount=type=secret,id=logging-creds,target=/run/secrets/logging-creds.json ls /run/secrets/logging-creds.json && ls -l /repo
```

You can find this and other examples in the `buildkit` [docs](https://github.com/moby/buildkit/blob/master/frontend/dockerfile/docs/experimental.md).

**Running the build**

Here is an example of the build and push combined together:

{go * ./docs/snippets/builder/build.force}

**Managing images with img tool**

All images built by `builder` plugin are integrated with [img](https://github.com/genuinetools/img)
tool that could be installed [here](https://github.com/genuinetools/img#binaries).

```bash
$ img ls
gcr.io/project/example:latest						4.916MiB
```

**CI pipeline process with docker and github**

The example below combines git, github and Docker to form a simple CI pipeline
process:

{go * ./docs/snippets/ci/ci.force}


## Kubernetes

**Setting it up**

{go * ./docs/snippets/kube/setup.force}

**Applying resources**

Function `kube.Apply` works similar to [kubectl apply](https://kubernetes.io/docs/reference/generated/kubectl/kubectl-commands#apply).

As you see, the snippets below use the `_` automatic type inferrence to avoid
typing config structs of kubernetes objects:

{go * ./docs/snippets/kube/apply.force}

**Running jobs**

The function `kube.Run` starts kubernetes jobs, collects and forwards logs from the pods,
verifies the pod statuses, while waiting until the job runs to completion or fails.

{go * ./docs/snippets/kube/job.force}

**Running Docker Builds**

In this example, we will create a function that takes a `Dockerfile` as a parameter and
run a kubernetes job that will build and publish the image.

Here is our complete function `RunBuildJob`:

{go * ./docs/snippets/buildkube/job.force}

Let's create a configmap with the Dockerfile in `docs/snippets` folder:

```
$ kubectl create configmap example-docker --from-file=./buildkube/Dockerfile
```

And let's call this function to build

{go * ./docs/snippets/buildkube/build.force}

It should be easy to expand this function to accept environment variable
arguments to be more useful.

## AWS

The family of `aws` plugin functions `Copy` and `RecursiveCopy` helps to
publish or download files from S3:

**Setting it up**

{go * ./docs/snippets/aws/setup.force}

**Working with S3**

{go * ./docs/snippets/aws/aws.force}

## Slack

The family of `aws` plugin functions helps to publish or download files from
S3:

**Setting it up**

{go * ./docs/snippets/slack/setup.force}

**Working with S3**

{go * ./docs/snippets/slack/slack.force}

The resulting slack bot could be used as following:

```bash
@force publish release with version 0.0.1
```

```bash
@force publish release with version 0.0.1, with flags build-rpm
```

Here's the generated help:

```bash
@force help
Here are the supported commands:
* help to print this help message
* help <command> to get help on individual command
* publish release Publishes a release. Use flags to control the publish process. For example, to build and publish all non-Windows builds:
publish release with flags build-linux-amd64, build-darwin-amd64, build-linux-arm, publish-s3, publish-image.
To build and publish only macOS binaries:
publish teleport with version, flags build-rpm.
```
