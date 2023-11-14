FROM mcr.microsoft.com/oss/go/microsoft/golang:1.19-fips-cbl-mariner2.0@sha256:a402a0c99abdf0900c5cd853f16390ee70326992c1a1d5fa5f820305ea6cc17d AS gobuild-base
RUN tdnf check-update \
    && tdnf install -y \
        git \
        make \
    && tdnf clean all

FROM gobuild-base AS acr-cli
WORKDIR /go/src/github.com/Azure/acr-cli
COPY . .
RUN make binaries && mv bin/acr /usr/bin/acr

FROM mcr.microsoft.com/cbl-mariner/base/core:2.0@sha256:4e16d123da8f90c10fe6cb7281b2f33f261b3e39cb6f1057ab85da6492eeaac7
RUN tdnf check-update \
    && tdnf --refresh install -y \
        ca-certificates-microsoft \
    && tdnf clean all
COPY --from=acr-cli /usr/bin/acr /usr/bin/acr
ENTRYPOINT [ "/usr/bin/acr" ]
