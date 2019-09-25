# syntax = docker/dockerfile:experimental

# This Dockerfile uses a buildbox image to build release versions of Teleport
# FORCE_IMAGE_NAME must be set here to be 'global' so it can be referenced by the second stage
ARG BUILDBOX
ARG FORCE_IMAGE_NAME
ARG ARTEFACT_EXTENSION
FROM $BUILDBOX

ARG UID
ARG GID
ARG OS
ARG ARCH
ARG RUNTIME=""
ARG FIPS=""
ARG ARTEFACT_EXTENSION
ARG WINDOWS_EXTRA_BUILD_STANZA="echo"

COPY ./build.assets/pam/pam_teleport.so /lib/x86_64-linux-gnu/security
COPY ./build.assets/pam/teleport-acct-failure /etc/pam.d
COPY ./build.assets/pam/teleport-session-failure /etc/pam.d
COPY ./build.assets/pam/teleport-success /etc/pam.d
COPY . /gopath/src/github.com/gravitational/teleport

RUN (groupadd jenkins --gid=$GID -o && useradd jenkins --uid=$UID --gid=$GID --create-home --shell=/bin/sh ;\
     mkdir -p /var/lib/teleport && chown -R jenkins /var/lib/teleport /gopath/src/github.com/gravitational/teleport)

WORKDIR /gopath/src/github.com/gravitational/teleport

RUN make release -e OS=${OS} ARCH=${ARCH} RUNTIME=${RUNTIME} FIPS=${FIPS}
RUN ${WINDOWS_EXTRA_BUILD_STANZA}

# calculate SHA256 hash and write to file
RUN echo '#!/bin/bash\n\
find . -type f -name "teleport-*'${ARTEFACT_EXTENSION}'" | while read FILE ;\n\
do FILENAME=$(echo ${FILE} | sed "s|^\./||") ;\n\
sha256sum ${FILENAME} > ${FILENAME}.sha256 ;\n\
echo ${FILENAME}.sha256 ;\n\
done'\
> ./sha256.sh
RUN bash ./sha256.sh && ls -l .

WORKDIR /gopath/src/github.com/gravitational/teleport/e
# this script renames the enterprise tarball as it doesn't get named automatically by the build process
RUN echo '#!/bin/bash\n\
find . -type f -name "teleport-*.tar.gz" | while read FILE ;\n\
do NEWFILE=$(echo ${FILE} | sed -e "s/teleport/teleport-ent/") ;\n\
mv ${FILE} ${NEWFILE} ;\n\
done'\
> ./ent-rename.sh
RUN bash ./ent-rename.sh && rm -f ./ent-rename.sh && ls -l .

# copy and run SHA256 script to output hash for enterprise version, then tidy up
RUN cp ../sha256.sh . && rm -f ../sha256.sh && bash ./sha256.sh && rm -f ./sha256.sh && ls -l .

# second stage using force container to upload to S3
FROM $FORCE_IMAGE_NAME
ARG TELEPORT_VERSION
ARG TARGET_S3_BUCKET
ARG ARTEFACT_EXTENSION
# annoyingly this has to be present for now
ENV AWS_REGION=us-west-2
RUN mkdir -p build
COPY --from=0 /gopath/src/github.com/gravitational/teleport/*$ARTEFACT_EXTENSION build/
COPY --from=0 /gopath/src/github.com/gravitational/teleport/*$ARTEFACT_EXTENSION.sha256 build/
COPY --from=0 /gopath/src/github.com/gravitational/teleport/e/*$ARTEFACT_EXTENSION build/
COPY --from=0 /gopath/src/github.com/gravitational/teleport/e/*$ARTEFACT_EXTENSION.sha256 build/
RUN echo 'Setup(\n\
     aws.Setup(aws.Config{\n\
          Region: ExpectEnv("AWS_REGION"),\n\
     }),\n\
)'\
> ./setup.force
RUN echo 'func(){\n\
     Include("../setup.force")\n\
     aws.RecursiveCopy(\n\
          aws.Local{Path: "../build/"},\n\
          aws.S3{Bucket: "'$TARGET_S3_BUCKET'", Key: "'$TELEPORT_VERSION'/"},\n\
     )\n\
}'\
> ./s3-upload.force
WORKDIR build
# you must be using the experimental Dockerfile syntax for the 'mount' stuff to work
RUN --mount=type=secret,id=aws-creds,target=/root/.aws/credentials force ../s3-upload.force
