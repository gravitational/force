# syntax = docker/dockerfile:experimental

# This Dockerfile uses an FPM image to buld Teleport packages
# FORCE_IMAGE_NAME must be set here to be 'global' so it can be referenced by the second stage
ARG FPM_CONTAINER_IMAGE
ARG FORCE_IMAGE_NAME
ARG PACKAGE_TYPE
FROM $FPM_CONTAINER_IMAGE

ARG OS
ARG ARCH
ARG RUNTIME_STANZA=""
ARG TELEPORT_TYPE
ARG TELEPORT_VERSION
ARG PACKAGE_TYPE

COPY ./xcloud/force/build-teleport-package-linux.sh /src
WORKDIR /src

# run script which calls FPM with correct parameters
RUN bash ./build-teleport-package-linux.sh \
     -t ${TELEPORT_TYPE} \
     -v ${TELEPORT_VERSION} \
     -p ${PACKAGE_TYPE} \
     -a ${ARCH} ${RUNTIME_STANZA}

# second stage using force container to upload to S3
FROM $FORCE_IMAGE_NAME
ARG TELEPORT_VERSION
ARG TARGET_S3_BUCKET
ARG PACKAGE_TYPE
# annoyingly this has to be present for now
ENV AWS_REGION=us-west-2
RUN mkdir -p build
COPY --from=0 /src/*.$PACKAGE_TYPE build/
COPY --from=0 /src/*.$PACKAGE_TYPE.sha256 build/
RUN echo 'Setup(\n\
     aws.Setup(aws.Config{\n\
          Region: ExpectEnv("AWS_REGION"),\n\
     }),\n\
)'\
> ./setup.force
RUN echo 'func(){\n\
     aws.RecursiveCopy(\n\
          aws.Local{Path: "./build/"},\n\
          aws.S3{Bucket: "'$TARGET_S3_BUCKET'", Key: "'$TELEPORT_VERSION'/"},\n\
     )\n\
}'\
> ./s3-upload.force
# you must be using the experimental Dockerfile syntax for the 'mount' stuff to work
RUN --mount=type=secret,id=aws-creds,target=/root/.aws/credentials force s3-upload.force