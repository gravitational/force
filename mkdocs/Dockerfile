# syntax = docker/dockerfile:experimental
ARG PY_VER
ARG NGINX_VER
FROM python:${PY_VER}-alpine as builder
ARG MKDOCS_VER
COPY . /go/src/github.com/gravitational/force
WORKDIR /go/src/github.com/gravitational/force
RUN apk update && apk upgrade &&apk add git
RUN echo "mkdocs ver is ${MKDOCS_VER}"
RUN pip install mkdocs==${MKDOCS_VER}
RUN pip install git+https://github.com/simonrenger/markdown-include-lines.git
RUN mkdocs build

FROM nginx:${NGINX_VER}-alpine
COPY --from=builder /go/src/github.com/gravitational/force/site /usr/share/nginx/html
