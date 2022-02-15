FROM golang:1.17-alpine3.14 AS gobuild-base
RUN apk add --no-cache \
	git \
	make

FROM gobuild-base AS acr-cli
WORKDIR /go/src/github.com/Azure/acr-cli
COPY . .
RUN make binaries && mv bin/acr /usr/bin/acr

FROM alpine:3.14
RUN apk --update add ca-certificates
COPY --from=acr-cli /usr/bin/acr /usr/bin/acr
ENTRYPOINT [ "/usr/bin/acr" ]
