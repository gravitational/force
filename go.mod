module github.com/gravitational/force

replace github.com/hashicorp/go-immutable-radix => github.com/tonistiigi/go-immutable-radix v0.0.0-20170803185627-826af9ccf0fe

replace github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305

require (
	cloud.google.com/go v0.38.0
	github.com/containerd/console v0.0.0-20181022165439-0650fd9eeb50
	github.com/containerd/containerd v1.3.0-0.20190507210959-7c1e88399ec0
	github.com/containerd/go-runc v0.0.0-20180907222934-5a6d9f37cfa3
	github.com/cyphar/filepath-securejoin v0.2.2
	github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
	github.com/docker/go-units v0.3.3 // indirect
	github.com/fsnotify/fsnotify v1.4.7
	github.com/golang/protobuf v1.3.1 // indirect
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/google/uuid v1.0.0
	github.com/gravitational/trace v0.0.0-20190612100216-931bb2abd388
	github.com/hashicorp/golang-lru v0.5.1 // indirect
	github.com/jonboulle/clockwork v0.1.0 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/mitchellh/hashstructure v1.0.0 // indirect
	github.com/moby/buildkit v0.6.0
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runc v1.0.1-0.20190307181833-2b18fe1d885e
	github.com/opencontainers/runtime-spec v1.0.1 // indirect
	github.com/opentracing-contrib/go-stdlib v0.0.0-20180702182724-07a764486eb1 // indirect
	github.com/opentracing/opentracing-go v1.0.2 // indirect
	github.com/shurcooL/githubv4 v0.0.0-20190625031733-ee671ab25ff0
	github.com/shurcooL/graphql v0.0.0-20181231061246-d48a9a75455f // indirect
	github.com/sirupsen/logrus v1.4.1
	go.etcd.io/bbolt v1.3.2
	golang.org/x/oauth2 v0.0.0-20190402181905-9f3314589c9a
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	google.golang.org/api v0.5.0
	google.golang.org/appengine v1.5.0 // indirect
	google.golang.org/genproto v0.0.0-20190508193815-b515fa19cec8 // indirect
	google.golang.org/grpc v1.20.1
	gopkg.in/src-d/go-git.v4 v4.13.1
)
