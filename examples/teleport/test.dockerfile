# This Dockerfile makes the "build box": the container used to build official
# releases of Teleport and its documentation.
ARG BUILDBOX
FROM $BUILDBOX

COPY ./build.assets/pam/pam_teleport.so /lib/x86_64-linux-gnu/security
COPY ./build.assets/pam/teleport-acct-failure /etc/pam.d
COPY ./build.assets/pam/teleport-session-failure /etc/pam.d
COPY ./build.assets/pam/teleport-success /etc/pam.d
COPY . /gopath/src/github.com/gravitational/teleport

