.PHONY: all
all:
	go install -mod=vendor github.com/gravitational/force/tool/force

.PHONY: oneshot
oneshot:
	cd examples/oneshot && force


.PHONY: vendor
vendor:
	go mod vendor

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: install-docs
install-docs:
	pip install mkdocs==1.0.4
	pip install git+https://github.com/simonrenger/markdown-include-lines.git

.PHONY: serve-docs
serve-docs:
	mkdocs serve

##

.PHONY: 1-simple
1-simple:
	$(MAKE) all
	cd examples/demo/1-simple && force

.PHONY: 2-watch
2-watch:
	$(MAKE) all
	force -d examples/demo/2-watch/G

.PHONY: 3-github
3-github:
	$(MAKE) all
	force examples/demo/3-github/G --setup=./examples/github/setup.force

.PHONY: 4-docker
4-docker:
	$(MAKE) all
	force examples/demo/4-docker/G --setup=./examples/github/setup.force
##

.PHONY: ssh
ssh: 
	$(MAKE) all
	cd examples/ssh && force -d ssh.force

.PHONY: aws
aws: 
	$(MAKE) all
	cd examples/aws && force -d aws.force

.PHONY: flows
flows:
	$(MAKE) all
	cd examples/flows && force -d ci.force

.PHONY: github
github:
	$(MAKE) all
	cd examples/github && force -d ci.force

.PHONY: github-branches
github-branches:
	$(MAKE) all
	cd examples/github-branches && force -d ci.force --setup=../github/setup.force

.PHONY: slack
slack:
	$(MAKE) all
	cd examples/slack && force -d ci.force --setup=../github/setup.force

.PHONY: buildbox
buildbox:
	$(MAKE) all
	cd examples/teleport/buildbox && force --setup=../../github/setup.force

.PHONY: teleport
teleport:
	$(MAKE) all
	cd examples/teleport && force -d teleport.force

.PHONY: teleport-reload
teleport-reload:
	$(MAKE) all
	cd examples/teleport && force -d reload.force

.PHONY: teleport-apply
teleport-apply:
	$(MAKE) all
	cd examples/teleport && force -d apply.force --setup=setup-local.force


.PHONY: kube
kube:
	$(MAKE) all
	cd examples/kube && force -d kube.force --setup=./setup.force

.PHONY: vars
vars:
	$(MAKE) all
	cd examples/vars && force -d

.PHONY: reload
reload:
	$(MAKE) all
	cd examples/reload && force -d reload.force

.PHONY: conditionals
conditionals:
	$(MAKE) all
	cd examples/conditionals && force -d conditionals.force

.PHONY: hello
hello:
	$(MAKE) all
	cd examples/hello && force -d

.PHONY: hello-lambda
hello-lambda:
	$(MAKE) all
	cd examples/hello-lambda && force -d


.PHONY: inception
inception:
	$(MAKE) all
	cd inception && force -d inception.force --setup=./setup.force

.PHONY: mkdocs
mkdocs:
	$(MAKE) all
	cd mkdocs && force -d mkdocs.force --setup=../examples/github/setup.force


.PHONY: kbuild
kbuild:
	$(MAKE) all
	cd examples/kbuild && force kbuild.force


.PHONY: marshal
marshal:
	$(MAKE) all
	cd examples/marshal && force marshal.force


.PHONY: sloccount
sloccount:
	find . -path ./vendor -prune -o -name "*.go" -print0 | xargs -0 wc -l


.PHONY: install-web
install-web:
	$(MAKE) -C web install

.PHONY: run-web
run-web: all
	force web

BUILDBOX_IMG := gcr.io/kubeadm-167321/force-grpc-buildbox:0.0.1

.PHONY: install-img
install-img:
	curl -L https://github.com/genuinetools/img/releases/download/v0.5.7/img-linux-amd64 -o $(GOPATH)/bin/img


# buildbox builds docker buildbox image used to compile binaries and generate GRPc stuff
.PHONY: grpc-buildbox
grpc-buildbox: PROTOC_VER ?= 3.11.4
grpc-buildbox: GOGO_PROTO_VER ?= v1.1.1
grpc-buildbox: PLATFORM := linux-x86_64
grpc-buildbox:
	cd build.assets/grpc && PROTOC_VER=$(PROTOC_VER) GOGO_PROTO_VER=$(GOGO_PROTO_VER) PLATFORM=$(PLATFORM) BUILDBOX_IMG=$(BUILDBOX_IMG) force -d build.force
	img save $(BUILDBOX_IMG) | docker load

# proto generates GRPC defs from service definitions
.PHONY: grpc
grpc:
	docker run -v $(shell pwd):/go/src/github.com/gravitational/force $(BUILDBOX_IMG) \
      make -C /go/src/github.com/gravitational/force grpc-in-buildbox

# proto generates GRPC stuff inside buildbox
.PHONY: grpc-in-buildbox
grpc-in-buildbox:
# standard GRPC output
	echo $$PROTO_INCLUDE
	cd proto && protoc -I=.:$$PROTO_INCLUDE \
	  --gofast_out=plugins=grpc:.\
      --plugin=protoc-gen-ts=/node_modules/.bin/protoc-gen-ts \
      --ts_out=service=grpc-web:../web/src/proto \
      --js_out=import_style=commonjs,binary:../web/src/proto \
    *.proto
