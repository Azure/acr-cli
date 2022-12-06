FROM mcr.microsoft.com/oss/go/microsoft/golang:1.18-fips-cbl-mariner2.0 AS gobuild-base
RUN tdnf install -y \
git \
make

FROM gobuild-base AS acr-cli
WORKDIR /go/src/github.com/Azure/acr-cli
COPY . .
RUN make binaries && mv bin/acr /usr/bin/acr

FROM mcr.microsoft.com/cbl-mariner/base/core:2.0.20221122
RUN tdnf --refresh install -y ca-certificates
COPY --from=acr-cli /usr/bin/acr /usr/bin/acr
ENTRYPOINT [ "/usr/bin/acr" ]
