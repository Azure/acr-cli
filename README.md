
# Azure Container Registry CLI

| Linux Build | Windows Build | Go Report |
|----|----|----|
|[![Build Status](https://dev.azure.com/azurecontainerregistry/acr-cli/_apis/build/status/acr-cli_linux?branchName=main)](https://dev.azure.com/azurecontainerregistry/acr-cli/_build/latest?definitionId=16&branchName=main)|[![Build Status](https://dev.azure.com/azurecontainerregistry/acr-cli/_apis/build/status/acr-cli_windows?branchName=main)](https://dev.azure.com/azurecontainerregistry/acr-cli/_build/latest?definitionId=17&branchName=main)|[![Go Report Card](https://goreportcard.com/badge/github.com/Azure/acr-cli)](https://goreportcard.com/report/github.com/Azure/acr-cli)|

This repository contains the source code for CLI components for Azure Container Registry.
The CLI consists of a new way to interact with Container Registries, the currently supported commands include
* Tag: to view all the tags of a repository and individually untag them.
* Manifest: to view the manifest of a given repository and delete them if necessary.
* Purge: to be able to delete all tags that are older than a certain date and that match a regex specified filter.

## Getting Started

Before running the ACR-CLI project make sure the following prerequisites are installed.

### Prerequisites

* [Go](https://golang.org/dl/) version greater than 1.11 (any version that has go mod support)
* [Docker](https://docs.docker.com/install/) installed (for running this project as a container image, not needed for local development)
* [Azure CLI](https://github.com/Azure/azure-cli) installed (only for running this project as a Task)
* An [Azure Container Registry](https://azure.microsoft.com/en-us/services/container-registry/)
* [Autorest](https://github.com/Azure/autorest.go) installed (if there are going to be modifications on the ACR SDK)

### Installation

For just building the application binaries, execute the following commands:

Linux (at repository root):

```sh
make binaries
```

Windows (inside /cmd/acr folder):

```sh
go build ./...
```

If using Docker:

```sh
docker build -t acr .
```

### Optional

For regenerating the ACR SDK for Go run (inside the docs folder):

```sh
autorest autorest.md --output-sdk-folder=../acr --go
```

For updating the vendor folder run (at repository root):

```sh
make vendor
```

## Usage

The following are examples of commands that the CLI currently supports.

### Login Command

If you are currently logged into an Azure Container Registry the program should be able to read your stored credentials, if not you can do:

```sh
acr login <registry name>
```

This login will also work with the [Docker CLI](https://github.com/docker/cli).

### Tag Command

To list all the tags inside a repository

```sh
acr tag list -r <Registry Name> --repository <Repository Name>
```

To delete a single tag from a repository

```sh
acr tag delete -r <Registry Name> --repository <Repository Name> <Tag Names>
```

### Manifest Command

To list all the manifests inside a repository

```sh
acr manifest list -r <Registry Name> --repository <Repository Name>
```

To delete a single manifest from a repository (and all the tags that are linked to it)

```sh
acr manifest delete -r <Registry Name> --repository <Repository Name> <Manifest digests>
```

### Purge Command

To delete all the tags that are older than a certain duration:

```sh
acr purge \
    --registry <Registry Name> \
    --filter <Repository Filter/Name>:<Regex Filter> \
    --ago <Go Style Duration>
```

#### Filter flag

The filter flag is used to specify the repository and a regex filter, if a tag is older than the duration specified by the ago flag and matches the regex filter then it is untagged, for example:

Examples of filters

| Intention                                                                        | Flag                                  |
|----------------------------------------------------------------------------------|---------------------------------------|
| Untag all tags that begin with hello in app repository                           | --filter `"app:^hello.*"`             |
| Untag tags that end with world in app repository                                 | --filter `"app:\w*world\b"`           |
| Untag tags that include hello-world in their name in app repository              | --filter `"app:hello-world"`          |
| Untag all tags that are older than the duration in repositories ending in /cache | --filter `".*/cache:.*"`              |
| Untag all tags that are older than the duration in app repository                | --filter `"app:.*"`                   |
| Untag all tags that are older than the duration in all repositories              | --filter `".*:.*"`                    |

#### Ago flag

The ago flag can be used to change the default expiration time of a tag, for example, the following command would purge all tags that are older than 30 days:

```sh
acr purge \
    --registry <Registry Name> \
    --filter <Repository Filter/Name>:<Regex Filter> \
    --ago 30d
```

The following table further explains the functionality of this flag.

| Intention                                                                     | Flag        |
|-------------------------------------------------------------------------------|-------------|
| To delete all images that were last modified before yesterday                 | --ago 1d    |
| To delete all images that were last modified before 10 minutes ago            | --ago 10m   |
| To delete all images that were last modified before 1 hour and 15 minutes ago | --ago 1h15m |

The duration should be of the form \[integer\]d\[string\] where the first integer specifies the number of days and the string is in a go style duration (can be omitted)

### Optional purge flags

#### Untagged flag

To delete all the manifests that do not have any tags linked to them, the `--untagged` flag should be set.

```sh
acr purge \
    --registry <Registry Name> \
    --filter <Repository Filter/Name>:<Regex Filter> \
    --ago 30d \
    --untagged
```

#### Keep flag

To keep the latest x number of to-be-deleted tags, the `--keep` flag should be set.

```sh
acr purge \
    --registry <Registry Name> \
    --filter <Repository Filter/Name>:<Regex Filter> \
    --ago 30d \
    --keep 3
```

#### Dry run flag

To know which tags and manifests would be deleted the `dry-run` flag can be set, nothing will be deleted and the output would be the same as if the purge command was executed normally.
An example of this would be:

```sh
acr purge \
    --registry <Registry Name> \
    --filter <Repository Filter/Name>:<Regex Filter> \
    --ago 30d \
    --dry-run
```

#### Concurrency flag
To control the number of concurrent purge tasks, the `--concurrency` flag should be set, the allowed range is [1, 32]. A default value will be used if `--concurrency` is not specified.
```sh
acr purge \
    --registry <Registry Name> \
    --filter <Repository Filter/Name>:<Regex Filter> \
    --ago 30d \
    --concurrency 4
```

#### Repository page size flag
To control the number of repositories fetched in a single page, the `--repository-page-size` flag should be set. A default value of 100 will be used if `--repository-page-size` is not specified.
This is useful when the number of artifacts in the registry is very large and listing too many repositories at once can timeout.
```sh
acr purge \
    --registry <Registry Name> \
    --filter <Repository Filter/Name>:<Regex Filter> \
    --ago 30d \
    --repository-page-size 10
```

#### Include-locked flag
To delete locked manifests and tags (where deleteEnabled or writeEnabled is false), the `--include-locked` flag should be set. This will unlock them before deletion.

**Warning:** The `--include-locked` flag will unlock and delete images that have been locked for protection. Use this flag with caution as it bypasses the image lock mechanism. For more information about image locking, see [Lock a container image in an Azure container registry](https://learn.microsoft.com/en-us/azure/container-registry/container-registry-image-lock).

```sh
acr purge \
    --registry <Registry Name> \
    --filter <Repository Filter/Name>:<Regex Filter> \
    --ago 30d \
    --include-locked
```

### Integration with ACR Tasks

To run a locally built version of the ACR-CLI using ACR Tasks follow these steps:
1. Build the docker image and push to an Azure Container Registry
Either build and push manually:

```sh
docker build -t <Registry Name>/acr:latest .
docker push <Registry Name>/acr:latest
```

Or using [ACR Build](https://docs.microsoft.com/en-us/azure/container-registry/container-registry-tutorial-quick-task)

```sh
az acr build -t acr:latest .
```

2. Run it inside an ACR task (authentication is obtained through the task itself) by executing

```sh
az acr run \
    --registry <Registry Name> \
    --cmd "{{ .Run.Registry }}/acr:latest <ACR-CLI command>" \
    /dev/null
```

For example to run the tag list command

```sh
az acr run \
    --registry <Registry Name> \
    --cmd "{{ .Run.Registry }}/acr:latest tag list -r {{ .Run.Registry }}
            --filter <Repository Filter/Name>:<Regex Filter>" \
    /dev/null
```

OR.
Schedule a periodically repeating task using [ACR Scheduled Tasks](https://docs.microsoft.com/en-us/azure/container-registry/container-registry-tasks-scheduled)

```sh
az acr task create \
    --name purgeTask \
    --registry <Registry Name> \
    --cmd "{{ .Run.Registry }}/acr:latest <ACR-CLI command>" \
    --context /dev/null \
    --schedule <CRON expression>
```

For example to have a task that executes every day and purges tags older than 7 days one can execute:

```sh
az acr task create \
    --name purgeTask \
    --registry <Registry Name> \
    --cmd "{{ .Run.Registry }}/acr:latest purge -r {{ .Run.Registry }}
            --filter <Repository Filter/Name>:<Regex Filter> --ago 7d" \
    --context /dev/null \
    --schedule "0 0 * * *"
```

## Contributing

This project welcomes contributions and suggestions.  Most contributions require you to agree to a
Contributor License Agreement (CLA) declaring that you have the right to, and actually do, grant us
the rights to use your contribution. For details, visit https://cla.microsoft.com.

When you submit a pull request, a CLA-bot will automatically determine whether you need to provide
a CLA and decorate the PR appropriately (e.g., label, comment). Simply follow the instructions
provided by the bot. You will only need to do this once across all repos using our CLA.

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/).
For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or
contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.
