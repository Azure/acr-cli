ARG WINDOWS_IMAGE=mcr.microsoft.com/windows/servercore:1903
FROM $WINDOWS_IMAGE as base

# set the default shell as powershell.
# $ProgressPreference: https://github.com/PowerShell/PowerShell/issues/2138#issuecomment-251261324
SHELL ["powershell", "-Command", "$ErrorActionPreference = 'Stop'; $ProgressPreference = 'SilentlyContinue';"]

FROM base as gobuild-base
# ideally, this would be C:\go to match Linux a bit closer, but C:\go is the recommended install path for Go itself on Windows
ENV GOPATH C:\\gopath

# PATH isn't actually set in the Docker image, so we have to set it from within the container
RUN $newPath = ('{0}\bin;C:\go\bin;{1}' -f $env:GOPATH, $env:PATH); \
	Write-Host ('Updating PATH: {0}' -f $newPath); \
	[Environment]::SetEnvironmentVariable('PATH', $newPath, [EnvironmentVariableTarget]::Machine);

# install go lang
# ideally we should be able to use FROM golang:windowsservercore-1803. This is not done due to two reasons
# 1. The go lang for 1803 tag is not available.

ENV GOLANG_VERSION 1.24.2

RUN $url = ('https://golang.org/dl/go{0}.windows-amd64.zip' -f $env:GOLANG_VERSION); \
	Write-Host ('Downloading {0} ...' -f $url); \
	Invoke-WebRequest -Uri $url -OutFile 'go.zip'; \
	\
	Write-Host 'Expanding ...'; \
	Expand-Archive go.zip -DestinationPath C:\; \
	\
	Write-Host 'Verifying install ("go version") ...'; \
	go version; \
	\
	Write-Host 'Removing ...'; \
	Remove-Item go.zip -Force; \
	\
	Write-Host 'Complete.';

# Build the acr-cli
FROM gobuild-base as acr-cli
WORKDIR \\gopath\\src\\github.com\\Azure\\acr-cli
COPY ./ /gopath/src/github.com/Azure/acr-cli
RUN Write-Host ('Running build'); \
    go build -o acr.exe .\cmd\acr; \
	Write-Host ('Running unit tests'); \
	go test ./...

# setup the runtime environment
FROM base as runtime
COPY --from=acr-cli /gopath/src/github.com/Azure/acr-cli/acr.exe c:/acr-cli/acr.exe

RUN setx /M PATH $('c:\acr-cli;{0}' -f $env:PATH);

ENTRYPOINT [ "acr.exe" ]
CMD [ "--help" ]
