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



.PHONY: github
github:
	$(MAKE) all
	cd examples/github && force -d ci.force

.PHONY: buildbox
buildbox:
	$(MAKE) all
	cd examples/teleport/buildbox && force --setup=../../github/setup.force

.PHONY: teleport
teleport:
	$(MAKE) all
	cd examples/teleport && force teleport.force --setup=../github/setup.force


.PHONY: kube
kube:
	$(MAKE) all
	cd examples/kube && force -d kube.force --setup=./setup.force


.PHONY: vars
vars:
	$(MAKE) all
	cd examples/vars && force -d vars.force --setup=./setup.force


.PHONY: inception
inception:
	$(MAKE) all
	cd inception && force -d inception.force --setup=./setup.force


.PHONY: hello
hello:
	$(MAKE) all
	cd examples/hello && force hello.force


.PHONY: marshal
marshal:
	$(MAKE) all
	cd examples/marshal && force marshal.force


.PHONY: sloccount
sloccount:
	find . -path ./vendor -prune -o -name "*.go" -print0 | xargs -0 wc -l
