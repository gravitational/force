# Building Kubernetes Native CI/CD Pipeline with Force.

[Force](https://force.gravitational.co) is an experimental tool
built by Gravitational to simplify Kubernetes-native and cloud native CI/CD
pipelines.

It is inspired by the simplicity of Makefiles, does not need a server to run
and consists of one binary that can run as a single instance on developers computer
or as a scalable distributed deployement in 1000 node Kubernetes cluster.


## Running test pipeline in Kubernetes

Let's outline the steps of the pipeline that runs in kubernetes:

* The script watches pull requests to the github branches.
* Not-approved contributors will wait for approval from some of the admins before running.
* Builds and publishes container with the test build.
* Run a test job and track it's status to publish back to the pull request.
* Sends logs to a central storage.



