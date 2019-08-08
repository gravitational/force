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
	cd examples/github && force ci.force

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
	cd examples/kube && force kube.force --setup=./setup.force
