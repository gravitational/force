# syntax = docker/dockerfile:experimental

FROM quay.io/gravitational/debian-tall:buster

ARG FORCE_ID
RUN echo "This is Force CI job ${FORCE_ID}, hello."
RUN date
RUN --mount=type=bind,target=/repo --mount=type=secret,id=logging-creds,target=/run/secrets/logging-creds.json ls /run/secrets/logging-creds.json && ls -l /repo

