FROM mcr.microsoft.com/oss/go/microsoft/golang:1.19-fips-cbl-mariner2.0@sha256:a2c05e5ba171cf7e540bd776fd90c800595c75704f87ab6d43d3a8d02b0be1df AS gobuild-base
RUN tdnf check-update \
    && tdnf install -y \
        git \
        make \
    && tdnf clean all

FROM gobuild-base AS acr-cli
WORKDIR /go/src/github.com/Azure/acr-cli
COPY . .
RUN make binaries && mv bin/acr /usr/bin/acr

FROM mcr.microsoft.com/cbl-mariner/base/core:2.0@sha256:4a2f15c52bf23d28272b89f86cdc515bbf6961e4aecf1f20a655d518113b5e28
RUN tdnf check-update \
    && tdnf --refresh install -y \
        ca-certificates-microsoft \
    && tdnf clean all
COPY --from=acr-cli /usr/bin/acr /usr/bin/acr
ENTRYPOINT [ "/usr/bin/acr" ]
