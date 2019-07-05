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


