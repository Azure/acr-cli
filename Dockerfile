FROM mcr.microsoft.com/oss/go/microsoft/golang:1.18-fips-cbl-mariner2.0@sha256:7e1335398d3c33cbd6b8b17cc5b7fd63b013dffccedc5320ba58033b2a1f5a72 AS gobuild-base
RUN tdnf check-update \
    && tdnf install -y \
        git \
        make \
    && tdnf clean all

FROM gobuild-base AS acr-cli
WORKDIR /go/src/github.com/Azure/acr-cli
COPY . .
RUN make binaries && mv bin/acr /usr/bin/acr

FROM mcr.microsoft.com/cbl-mariner/base/core:2.0@sha256:ab3a6611146f66783cf55fa134c92c52186ed7258b0d63b12f3d6ed5bf6bb1e3
RUN tdnf check-update \
    && tdnf --refresh install -y \
        ca-certificates-microsoft \
    && tdnf clean all
COPY --from=acr-cli /usr/bin/acr /usr/bin/acr
ENTRYPOINT [ "/usr/bin/acr" ]
