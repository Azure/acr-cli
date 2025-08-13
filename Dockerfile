FROM mcr.microsoft.com/oss/go/microsoft/golang:1.24.6-fips-azurelinux3.0 AS gobuild-base
RUN tdnf check-update \
    && tdnf install -y \
        git \
        make \
    && tdnf clean all

FROM gobuild-base AS acr-cli
WORKDIR /go/src/github.com/Azure/acr-cli
COPY . .
RUN make binaries && mv bin/acr /usr/bin/acr

# Manually copy essential libraries that Go FIPS binaries typically need
RUN mkdir -p /tmp/libs /tmp/linker && \
    cp /lib64/libc.so.6 /tmp/libs/ 2>/dev/null || true && \
    cp /lib64/libdl.so.2 /tmp/libs/ 2>/dev/null || true && \
    cp /lib64/libpthread.so.0 /tmp/libs/ 2>/dev/null || true && \
    cp /lib64/librt.so.1 /tmp/libs/ 2>/dev/null || true && \
    cp /lib64/libresolv.so.2 /tmp/libs/ 2>/dev/null || true && \
    cp /lib64/libssl.so* /tmp/libs/ 2>/dev/null || true && \
    cp /lib64/libcrypto.so* /tmp/libs/ 2>/dev/null || true && \
    cp /usr/lib64/libssl.so* /tmp/libs/ 2>/dev/null || true && \
    cp /usr/lib64/libcrypto.so* /tmp/libs/ 2>/dev/null || true && \
    find /lib /lib64 /usr/lib -name "ld-linux*" -exec cp {} /tmp/linker/ \; 2>/dev/null || true

FROM mcr.microsoft.com/azurelinux/distroless/minimal:3.0
# Copy the dynamic linker for the respective platform
COPY --from=acr-cli /tmp/linker/ /lib/
# Copy essential libraries
COPY --from=acr-cli /tmp/libs/ /lib64/
# Copy the binary
COPY --from=acr-cli /usr/bin/acr /usr/bin/acr
ENTRYPOINT [ "/usr/bin/acr" ]
