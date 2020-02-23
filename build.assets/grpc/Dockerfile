FROM golang:1.12.4-stretch as gobuild

ARG PROTOC_VER
ARG GOGO_PROTO_VER
ARG PLATFORM

ENV TARBALL protoc-${PROTOC_VER}-${PLATFORM}.zip

RUN apt-get update && apt-get install unzip

RUN curl -L -o /tmp/${TARBALL} https://github.com/google/protobuf/releases/download/v${PROTOC_VER}/${TARBALL}
RUN cd /tmp && unzip /tmp/protoc-${PROTOC_VER}-linux-x86_64.zip -d /usr/local && rm /tmp/${TARBALL}

RUN go get -u github.com/gogo/protobuf/proto github.com/gogo/protobuf/protoc-gen-gogo github.com/gogo/protobuf/gogoproto golang.org/x/tools/cmd/goimports
RUN cd ${GOPATH}/src/github.com/gogo/protobuf && git reset --hard ${GOGO_PROTO_VER} && make install
RUN which protoc-gen-gofast


FROM node:12.16.1
ENV GOPATH /go
ENV PATH $PATH:/go/bin
ENV GOGOPROTO_ROOT ${GOPATH}/src/github.com/gogo/protobuf
ENV PROTO_INCLUDE "/usr/local/include":"${GOPATH}/src":"${GOPATH}/src/github.com/gogo/protobuf/protobuf":"${GOGOPROTO_ROOT}":"${GOGOPROTO_ROOT}/protobuf":"${GOPATH}/src/github.com/gravitational/force"
COPY --from=gobuild /go/src/ /go/src
COPY --from=gobuild /go/bin/ /go/bin
COPY --from=gobuild /usr/local/bin/protoc /usr/local/bin/protoc
RUN yarn add --dev @types/google-protobuf@^3.7.2 ts-protoc-gen@^0.12.0
