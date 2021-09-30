module github.com/Azure/acr-cli

go 1.12

require (
	github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78 // indirect
	github.com/Azure/go-autorest/autorest v0.11.21
	github.com/Azure/go-autorest/autorest/adal v0.9.16
	github.com/Azure/go-autorest/tracing v0.6.0
	github.com/containerd/containerd v1.4.10 // indirect
	github.com/docker/cli v0.0.0-20190506213505-d88565df0c2d
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v1.14.0-0.20190131205458-8a43b7bb99cd
	github.com/docker/docker-credential-helpers v0.6.1 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-metrics v0.0.0-20181218153428-b84716841b82 // indirect
	github.com/docker/go-units v0.3.3 // indirect
	github.com/docker/libtrust v0.0.0-20160708172513-aabc10ec26b7 // indirect
	github.com/gogo/protobuf v1.2.0 // indirect
	github.com/golang-jwt/jwt/v4 v4.1.0
	github.com/golang/protobuf v1.3.1 // indirect
	github.com/gorilla/context v1.1.1 // indirect
	github.com/gorilla/mux v1.6.2 // indirect
	github.com/morikuni/aec v0.0.0-20170113033406-39771216ff4c // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v0.9.3-0.20190127221311-3c4408c8b829 // indirect
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.5
	github.com/stretchr/testify v1.2.2
	google.golang.org/genproto v0.0.0-20190307195333-5fe7a883aa19 // indirect
	google.golang.org/grpc v1.19.1 // indirect
	gotest.tools v2.2.0+incompatible // indirect
	gotest.tools/v3 v3.0.2 // indirect
)

replace github.com/docker/docker => github.com/docker/docker v1.4.2-0.20200227233006-38f52c9fec82
