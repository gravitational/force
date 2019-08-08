FROM debian:stretch

ARG GO_RUNTIME
ADD locale.gen /etc/locale.gen

ENV LANGUAGE="en_US.UTF-8" \
    DEBIAN_FRONTEND="noninteractive" \
    LANG="en_US.UTF-8" \
    LC_ALL="en_US.UTF-8" \
    LC_CTYPE="en_US.UTF-8"

RUN apt-get update -y --fix-missing; \
    apt-get -q -y upgrade; \
    apt-get install -q -y apt-utils locales; \
    locale-gen; \
	locale-gen en_US.UTF-8 ;\
	dpkg-reconfigure locales

RUN apt-get install -q -y \
            libsqlite3-0 \
            curl \
            make \
            git \
            libc6-dev \
            libpam-dev \
            gcc \
            tar \
            gzip \
            zip \
            libc6-dev-i386 \
            net-tools \
            tree

ADD profile /etc/profile

# Install Go.
RUN mkdir -p /opt && cd /opt && curl https://storage.googleapis.com/golang/$GO_RUNTIME.linux-amd64.tar.gz | tar xz;\
    mkdir -p /gopath/src/github.com/gravitational/teleport;\
    chmod a+w /gopath;\
    chmod a+w /var/lib;\
    chmod a-w /

ENV GOPATH="/gopath" \
    GOROOT="/opt/go" \
    PATH="$PATH:/opt/go/bin:/gopath/bin:/gopath/src/github.com/gravitational/teleport/build"

RUN apt-get -y autoclean; apt-get -y clean
