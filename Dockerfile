FROM golang:1.12.5-alpine AS gobuild-base
RUN apk add --no-cache \
	git \
	make

FROM gobuild-base AS acr-cli
WORKDIR /go/src/github.com/Azure/acr-cli
COPY . .
RUN make binaries && mv bin/acr /usr/bin/acr

FROM alpine:3.9.4
RUN apk --update add ca-certificates
COPY --from=acr-cli /usr/bin/acr /usr/bin/acr
ENTRYPOINT [ "acr" ]
