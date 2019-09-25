# This Dockerfile uses a buildbox image to build release versions of Teleport followed by a Docker container
ARG BUILDBOX
FROM $BUILDBOX

ARG UID
ARG GID
ARG OS
ARG ARCH
ARG RUNTIME=""
ARG FIPS=""

COPY ./build.assets/pam/pam_teleport.so /lib/x86_64-linux-gnu/security
COPY ./build.assets/pam/teleport-acct-failure /etc/pam.d
COPY ./build.assets/pam/teleport-session-failure /etc/pam.d
COPY ./build.assets/pam/teleport-success /etc/pam.d
COPY . /gopath/src/github.com/gravitational/teleport

RUN (groupadd jenkins --gid=$GID -o && useradd jenkins --uid=$UID --gid=$GID --create-home --shell=/bin/sh ;\
     mkdir -p /var/lib/teleport && chown -R jenkins /var/lib/teleport /gopath/src/github.com/gravitational/teleport)

WORKDIR /gopath/src/github.com/gravitational/teleport

RUN make release -e OS=${OS} ARCH=${ARCH} RUNTIME=${RUNTIME} FIPS=${FIPS}

# second stage builds actual container with teleport binaries in
FROM ubuntu:18.04
ARG GO_BUILD_PATH

# Install dumb-init and ca-certificates. The dumb-init package is to ensure
# signals and orphaned processes are are handled correctly. The ca-certificate
# package is installed because the base Ubuntu image does not come with any
# certificate authorities.
#
# Note that /var/lib/apt/lists/* is cleaned up in the same RUN command as
# "apt-get update" to reduce the size of the image.
RUN apt-get update && apt-get upgrade -y && \
    apt-get install --no-install-recommends -y \
    dumb-init \
    ca-certificates \
    && update-ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN echo "$GO_BUILD_PATH/teleport"

# Copy "teleport", "tctl", and "tsh" binaries from the previous stage
COPY --from=0 $GO_BUILD_PATH/teleport /usr/local/bin/teleport
COPY --from=0 $GO_BUILD_PATH/tctl /usr/local/bin/tctl
COPY --from=0 $GO_BUILD_PATH/tsh /usr/local/bin/tsh

# By setting this entry point, we expose make target as command.
ENTRYPOINT ["/usr/bin/dumb-init", "teleport", "start", "-c", "/etc/teleport/teleport.yaml"]