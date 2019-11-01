# Force

`Force` is an event processing and infrastructure automation framework.

`Makefiles` create an easy way to build targets and projects.

`.force` scripts create event-driven workflows with multiple services
combined together: Github to Docker builds, Slack to Kubernetes Deployments.

## Status

Current version is not even alpha, use at your own risk (0.0.18).

## Documentation

Read the docs at [https://force.gravitational.com](https://force.gravitational.com)

## Goals

It should be easy and fun to define declarative event-driven workflows for infrastructure
projects.

* The tooling will be tailored to detect loops, inefficiencies in event-driven workflows.
* Should make it easy and manageable to have an even driven distributed system
running on Kubernetes or on developer's laptop.
* Should be a single binary with no external dependencies.
* Should not invent a new syntax and use `Go` syntax for everything.
* It should be trivial to build a simple CI/CD system for a small project.

## Non goals

It is not a general purpose event workflow tool, it's designed for cloud native
infrastructure projects.

## Batteries included

Force already includes out of the box plugins for:

* Local and Kubernetes-native linux Docker builds.
* Github and git integration
* AWS S3
* SSH
* Slack

Soon force will include out of the box plugins for:

* Better integration with Kubernetes.
* Other popular source control and code sharing systems - Bitbucket, Gitlab.
* Event queues - Redis, Kafka, AWS SQS.

## Design concepts for language geeks

**Rationale**

There should be a high level abstraction, a glue language to describe
modern cloud-native workloads.

Current state of the system using YAML is not good
enough, as it produces very complex systems that are very hard to troubleshoot,
debug and install.

**Originals**

Force is an interpreted mix of [Go](https://golang.org) and [Scheme](https://en.wikipedia.org/wiki/Scheme_(programming_language).

[Go](https://golang.org) makes it fun to work with concurrently running processes because it derives
it's design from the [CSP](http://www.usingcsp.com/cspbook.pdf).

[Scheme](https://en.wikipedia.org/wiki/Scheme_(programming_language) is a higher-level
functional language, and Force uses it's functional declarative style
that will work well for cloud-native workloads where state
is propagated across distributed infrastructure. Scheme's immutable and functional
approach should work well

## Status

This is work in progress and draft.

Please send your feedback to `sasha@gravitational.com`
who is working on this project as a research activity.
