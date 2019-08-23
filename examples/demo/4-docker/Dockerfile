# This Dockerfile makes the "build box": the container used to build official
# releases of Teleport and its documentation.
FROM golang:1.12.9 as builder

ENV GO111MODULE="on"
COPY . /go/src/github.com/gravitational/force
WORKDIR /go/src/github.com/gravitational/force
RUN go build -o force -mod=vendor github.com/gravitational/force/tool/force

FROM ubuntu:18.04
COPY --from=builder /go/src/github.com/gravitational/force/force /usr/bin/force
