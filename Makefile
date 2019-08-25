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
	cd examples/vars && force -d  --include=vars.force


.PHONY: inception
inception:
	$(MAKE) all
	cd inception && force -d inception.force --setup=./setup.force


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
