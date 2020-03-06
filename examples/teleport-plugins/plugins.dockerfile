# This Dockerfile makes the "build box": the container used to build official
# releases of Teleport and its documentation.
ARG BUILDBOX
FROM $BUILDBOX

ARG UID
ARG GID

COPY . /gopath/src/github.com/gravitational/teleport-plugins

RUN (groupadd jenkins --gid=$GID -o && useradd jenkins --uid=$UID --gid=$GID --create-home --shell=/bin/sh ;\
     chown -R jenkins /gopath/src/github.com/gravitational/teleport-plugins)
