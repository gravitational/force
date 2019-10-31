# Force

`Force` is an event processing and infrastructure automation framework.z

`Makefiles` create an easy way to build targets and projects.

`G` files create an easy way to create event-driven workflows with multiple services
combined together: Github to Docker builds, Webhooks to Kubernetes Deployments.

## Goals

It should be easy and fun to define declarative event-driven workflows for infrastructure
projects.

`Force` tooling will be tailored to detect loops, inefficiencies in event-drive workflows.

`Force` should make it easy and manageable to have an even driven distributed system
running on Kubernetes or on developer's laptop.

`Force` should be a single binary with no external dependencies.

`Force` should not invent a new syntax and use `Go` syntax for everything.

It should be trivial to build a simple CI/CD system for a small project.

## Non goals

`Force` does not have a goal of becoming a turing complete interpreted language,
the simpler the better.

`Force` is not a general purpose event workflow tool, it's designed for common
backend infrastructure projects.

## Design concepts

Force is modeled (and uses it's syntax) ater Go programming language. Force
could be used as a library in Go code, or as an interpreted files using `force`.

The syntax is the same regardless of the usage mode.

[Go](https://golang.org) makes it fun to work with concurrently running processes because it derives
it's design from the [CSP](http://www.usingcsp.com/cspbook.pdf).

## Batteries included

Force already includes out of the box plugins for:

* Local linux Docker builds.
* Github and git

Soon force will include out of the box plugins for:

* Docker builds that could be run in Kubernetes.
* More integration with Kubernetes.
* Other popular source control and code sharing systems - Bitbucket, Gitlab.
* Event queues - Redis, Kafka, AWS SQS.

For everything else, folks can create plugins using GRPC plugins system.

## Documentation

Read the docs at [docs](docs/index.md)
