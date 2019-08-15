# syntax = docker/dockerfile:experimental
ARG GO_VER
FROM golang:${GO_VER} as builder
ENV GO111MODULE="on"
ARG RUNC_VER
COPY . /go/src/github.com/gravitational/force
WORKDIR /go/src/github.com/gravitational/force
RUN go build -o force -mod=vendor github.com/gravitational/force/tool/force
ADD https://github.com/opencontainers/runc/releases/download/v${RUNC_VER}/runc.amd64 /usr/bin/runc
RUN chmod +x /usr/bin/runc

FROM ubuntu:18.04
COPY --from=builder /go/src/github.com/gravitational/force/force /usr/bin/force
COPY --from=builder /usr/bin/runc /usr/bin/runc
RUN apt-get update && apt-get install -y ca-certificates && update-ca-certificates && apt-get -y autoclean && apt-get -y clean



