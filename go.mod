module github.com/gravitational/force

replace github.com/hashicorp/go-immutable-radix => github.com/tonistiigi/go-immutable-radix v0.0.0-20170803185627-826af9ccf0fe

replace github.com/jaguilar/vt100 => github.com/tonistiigi/vt100 v0.0.0-20190402012908-ad4c4a574305

require (
	cloud.google.com/go v0.38.0
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190717042225-c3de453c63f4 // indirect
	github.com/aws/aws-sdk-go v1.25.6
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/containerd/console v0.0.0-20181022165439-0650fd9eeb50
	github.com/containerd/containerd v1.3.0-0.20190507210959-7c1e88399ec0
	github.com/containerd/go-runc v0.0.0-20180907222934-5a6d9f37cfa3
	github.com/cyphar/filepath-securejoin v0.2.2
	github.com/desertbit/timer v0.0.0-20180107155436-c41aec40b27f // indirect
	github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
	github.com/docker/go-units v0.3.3 // indirect
	github.com/fsnotify/fsnotify v1.4.7
	github.com/golang/protobuf v1.3.1
	github.com/google/go-github v17.0.0+incompatible
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/gravitational/httplib v1.0.0
	github.com/gravitational/trace v1.1.10-0.20200129130229-dd5b2e8eae86
	github.com/improbable-eng/grpc-web v0.12.0
	github.com/julienschmidt/httprouter v1.3.0
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/mitchellh/hashstructure v1.0.0 // indirect
	github.com/moby/buildkit v0.6.0
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/nlopes/slack v0.6.0
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runc v1.0.1-0.20190307181833-2b18fe1d885e
	github.com/opencontainers/runtime-spec v1.0.1 // indirect
	github.com/opentracing-contrib/go-stdlib v0.0.0-20180702182724-07a764486eb1 // indirect
	github.com/opentracing/opentracing-go v1.0.2 // indirect
	github.com/rs/cors v1.7.0 // indirect
	github.com/shurcooL/githubv4 v0.0.0-20190625031733-ee671ab25ff0
	github.com/shurcooL/graphql v0.0.0-20181231061246-d48a9a75455f // indirect
	github.com/sirupsen/logrus v1.4.2
	github.com/wasmerio/go-ext-wasm v0.3.1
	go.etcd.io/bbolt v1.3.2
	golang.org/x/crypto v0.0.0-20200221231518-2aa609cf4a9d
	golang.org/x/net v0.0.0-20200222125558-5a598a2470a0
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys v0.0.0-20200116001909-b77594299b42 // indirect
	google.golang.org/api v0.10.0
	google.golang.org/genproto v0.0.0-20190508193815-b515fa19cec8
	google.golang.org/grpc v1.23.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15
	gopkg.in/src-d/go-git.v4 v4.13.1
	k8s.io/api v0.0.0-20190620084959-7cf5895f2711
	k8s.io/apimachinery v0.0.0-20190612205821-1799e75a0719
	k8s.io/client-go v0.0.0-20190620085101-78d2af792bab
)

go 1.13
