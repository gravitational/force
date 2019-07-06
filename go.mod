module github.com/gravitational/force

replace github.com/hashicorp/go-immutable-radix => github.com/tonistiigi/go-immutable-radix v0.0.0-20170803185627-826af9ccf0fe

replace github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305

require (
	github.com/containerd/console v0.0.0-20181022165439-0650fd9eeb50
	github.com/containerd/containerd v1.3.0-0.20190426060238-3a3f0aac8819
	github.com/containerd/go-runc v0.0.0-20180907222934-5a6d9f37cfa3
	github.com/cyphar/filepath-securejoin v0.2.2
	github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
	github.com/docker/docker-credential-helpers v0.6.1 // indirect
	github.com/docker/go-units v0.3.3 // indirect
	github.com/fsnotify/fsnotify v1.4.7
	github.com/gogo/googleapis v1.1.0 // indirect
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/gravitational/trace v0.0.0-20190612100216-931bb2abd388
	github.com/jonboulle/clockwork v0.1.0 // indirect
	github.com/mitchellh/hashstructure v1.0.0 // indirect
	github.com/moby/buildkit v0.5.1
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runc v1.0.1-0.20190307181833-2b18fe1d885e
	github.com/opencontainers/runtime-spec v1.0.1 // indirect
	github.com/opentracing-contrib/go-stdlib v0.0.0-20180702182724-07a764486eb1 // indirect
	github.com/opentracing/opentracing-go v1.0.2 // indirect
	github.com/shurcooL/githubv4 v0.0.0-20190625031733-ee671ab25ff0
	github.com/shurcooL/graphql v0.0.0-20181231061246-d48a9a75455f // indirect
	github.com/sirupsen/logrus v1.3.0
	go.etcd.io/bbolt v1.3.2
	golang.org/x/net v0.0.0-20180925072008-f04abc6bdfa7 // indirect
	golang.org/x/oauth2 v0.0.0-20180821212333-d2e6202438be
	golang.org/x/sync v0.0.0-20180314180146-1d60e4601c6f
	golang.org/x/time v0.0.0-20180412165947-fbb02b2291d2 // indirect
	google.golang.org/genproto v0.0.0-20180924164928-221a8d4f7494 // indirect
	google.golang.org/grpc v1.15.0
)
