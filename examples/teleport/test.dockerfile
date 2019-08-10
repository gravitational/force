# This Dockerfile makes the "build box": the container used to build official
# releases of Teleport and its documentation.
ARG BUILDBOX
FROM $BUILDBOX

ARG UID
ARG GID
ARG ETCD_VER

COPY ./build.assets/pam/pam_teleport.so /lib/x86_64-linux-gnu/security
COPY ./build.assets/pam/teleport-acct-failure /etc/pam.d
COPY ./build.assets/pam/teleport-session-failure /etc/pam.d
COPY ./build.assets/pam/teleport-success /etc/pam.d
COPY . /gopath/src/github.com/gravitational/teleport

RUN (groupadd jenkins --gid=$GID -o && useradd jenkins --uid=$UID --gid=$GID --create-home --shell=/bin/sh ;\
     mkdir -p /var/lib/teleport && chown -R jenkins /var/lib/teleport /gopath/src/github.com/gravitational/teleport)

RUN (curl -L https://github.com/coreos/etcd/releases/download/${ETCD_VER}/etcd-${ETCD_VER}-linux-amd64.tar.gz | tar -xz ;\
     cp etcd-${ETCD_VER}-linux-amd64/etcd* /bin/)
