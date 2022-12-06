FROM mcr.microsoft.com/oss/go/microsoft/golang:1.18-fips-cbl-mariner2.0 AS gobuild-base
RUN tdnf check-update \
    && tdnf install -y \
        git \
        make \
    && tdnf clean all

FROM gobuild-base AS acr-cli
WORKDIR /go/src/github.com/Azure/acr-cli
COPY . .
RUN make binaries && mv bin/acr /usr/bin/acr

FROM mcr.microsoft.com/cbl-mariner/base/core:2.0
RUN tdnf check-update \
    && tdnf --refresh install -y \
        ca-certificates-microsoft \
    && tdnf clean all
COPY --from=acr-cli /usr/bin/acr /usr/bin/acr
ENTRYPOINT [ "/usr/bin/acr" ]
