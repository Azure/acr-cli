
# Azure Container Registry CLI

| Linux Build | Windows Build | Go Report |
|----|----|----|
|[![Build Status](https://dev.azure.com/azurecontainerregistry/acr-cli/_apis/build/status/acr-cli_linux?branchName=main)](https://dev.azure.com/azurecontainerregistry/acr-cli/_build/latest?definitionId=16&branchName=main)|[![Build Status](https://dev.azure.com/azurecontainerregistry/acr-cli/_apis/build/status/acr-cli_windows?branchName=main)](https://dev.azure.com/azurecontainerregistry/acr-cli/_build/latest?definitionId=17&branchName=main)|[![Go Report Card](https://goreportcard.com/badge/github.com/Azure/acr-cli)](https://goreportcard.com/report/github.com/Azure/acr-cli)|

> [!IMPORTANT]
> The `acr purge` feature in this repository is now in maintenance mode. While we won’t be adding new features, we will continue to accept pull requests, address critical issues, and incorporate feedback. Development efforts are shifting toward a server-side purge solution planned in Azure Container Registry for 2026, which will eventually replace this tool.

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

| Intention                                                                           | Flag                                  |
|-------------------------------------------------------------------------------------|---------------------------------------|
| Untag all tags that begin with hello in app repository                              | --filter `"app:^hello.*"`             |
| Untag tags that end with world in app repository                                    | --filter `"app:\w*world\b"`           |
| Untag tags that include hello-world in their name in app repository                 | --filter `"app:hello-world"`          |
| Untag all tags in repositories ending in /cache                                     | --filter `".*/cache:.*"`              |
| Untag all tags in app repository                                                    | --filter `"app:.*"`                   |
| Untag all tags in all repositories                                                  | --filter `".*:.*"`                    |
| Clean only untagged manifests in app repository (with --untagged-only)             | --filter `"app:.*"`                   |

##### Literal vs. regex repository names

When the repository portion of a `--filter` is a plain name with no regex metacharacters (e.g. `my-repo:.*`), the CLI treats it as a **literal** repository name and targets that repository directly — **without listing the full catalog**. The catalog API is only called when at least one filter contains a regex pattern in the repository portion (e.g. `.*:.*` or `.*/cache:.*`).

This has two practical benefits:

- **Large registries:** Registries with a very large number of repositories can avoid the slow or impractical catalog listing step entirely by using literal repository names in their filters.
- **Restricted permissions (ABAC):** In ABAC-enabled registries where a user only has access to specific repositories and lacks catalog listing permissions, literal filters allow `acr purge` to work without requiring the `Container Registry Repository Catalog Lister` role.

For example, a user who only has access to `team-a/app` can run:

```sh
acr purge \
    --registry <Registry Name> \
    --filter "team-a/app:.*" \
    --ago 30d
```

No catalog listing permission is needed because `team-a/app` is a literal name. Multiple literal filters can be combined to target several specific repositories:

```sh
acr purge \
    --registry <Registry Name> \
    --filter "team-a/app:.*" \
    --filter "team-a/cache:.*" \
    --ago 30d
```


#### Ago flag

The `--ago` flag sets the age cutoff for deletion. It is required when deleting tags and optional with `--untagged-only`. For example, the following command purges all matching tags older than 30 days:

```sh
acr purge \
    --registry <Registry Name> \
    --filter <Repository Filter/Name>:<Regex Filter> \
    --ago 30d
```

You can pair `--ago` with the `--untagged` flag to apply the same age threshold to manifest cleanup, ensuring that only manifests
older than the specified cutoff are removed:

```sh
acr purge \
    --registry <Registry Name> \
    --filter <Repository Filter/Name>:<Regex Filter> \
    --ago 7d \
    --untagged
```

The following table further explains the functionality of this flag.

| Intention                                                                     | Flag        |
|-------------------------------------------------------------------------------|-------------|
| To delete all images that were last modified before yesterday                 | --ago 1d    |
| To delete all images that were last modified before 10 minutes ago            | --ago 10m   |
| To delete all images that were last modified before 1 hour and 15 minutes ago | --ago 1h15m |

The duration should be of the form \[integer\]d\[string\] where the first integer specifies the number of days and the string is in a go style duration (can be omitted). The maximum ago duration is set to 150 years but that will effectively clean nothing up as no images should be that old.

### Optional purge flags

#### Untagged flag

To delete all the manifests that do not have any tags linked to them, the `--untagged` flag should be set. Tag deletion runs first, followed by deletion of eligible manifests that have no tags. Manifest cleanup respects the same `--ago` cutoff used for tag deletion, so recently-untagged images newer than the configured age threshold are preserved. If `--keep` is specified, it is applied independently to tags and untagged manifests.

```sh
acr purge \
    --registry <Registry Name> \
    --filter <Repository Filter/Name>:<Regex Filter> \
    --ago 30d \
    --untagged
```

#### Untagged-only flag

To delete ONLY untagged manifests without deleting any tags, the `--untagged-only` flag should be set. The `--ago`, `--keep`, and `--filter` flags are optional in this mode. When specified, `--ago` limits deletion to untagged manifests older than the configured duration, and `--keep` preserves the specified number of most recently updated manifests among those eligible for deletion. If `--ago` is omitted, all existing untagged manifests are eligible for deletion. When `--filter` is specified, only its repository portion is used; the tag regex portion is ignored.

```sh
# Delete untagged manifests in all repositories
acr purge \
    --registry <Registry Name> \
    --untagged-only

# Delete untagged manifests in specific repositories matching a filter
acr purge \
    --registry <Registry Name> \
    --filter <Repository Filter/Name>:<Regex Filter> \
    --untagged-only

# Delete untagged manifests older than 30 days, keeping the 5 most recent
# manifests that are eligible for deletion
acr purge \
    --registry <Registry Name> \
    --untagged-only \
    --ago 30d \
    --keep 5
```

Note: The `--untagged` and `--untagged-only` flags are mutually exclusive.

#### Keep flag

To keep the latest x number of items that would otherwise be deleted, the `--keep` flag should be set. The count is applied per repository. By default, it preserves tags. When `--untagged` is also set, `--keep` is applied independently to tags and untagged manifests, preserving up to the specified number of each in every repository. With `--untagged-only`, it applies only to untagged manifests.

```sh
acr purge \
    --registry <Registry Name> \
    --filter <Repository Filter/Name>:<Regex Filter> \
    --ago 30d \
    --keep 3
```

#### Dry run flag

To know which tags and manifests would be deleted, the `--dry-run` flag can be set. Nothing will be deleted, and the output will show what would happen if the purge command were executed normally.
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

#### ABAC (Attribute-Based Access Control) registries

Registries with ABAC enabled use repository-scoped permissions instead of registry-wide roles. When using `acr purge` with an ABAC registry, keep the following in mind:

**Required permissions:**
- **Catalog listing:** Required only when the `--filter` contains a regex pattern in the repository portion (e.g., `.*:.*`). If all filters use literal repository names (e.g., `my-repo:.*`), catalog listing is **not** required — see [Literal vs. regex repository names](#literal-vs-regex-repository-names) above.
- **Repository access:** The user needs the `Container Registry Repository Contributor` role for deletes, which can be scoped to specific repositories using ABAC conditions.

**Partial access behavior:**

If a broad `--filter` matches repositories that the user does not have permission to purge, the command will stop at the first unauthorized repository and report:
- Which repository failed due to insufficient permissions
- Which repositories were already successfully purged
- Which repositories were not yet processed

To avoid this, use a more specific `--filter` to target only repositories you have access to.

**Batch size (environment variable):**

ABAC registries process repositories in batches, where each batch shares a single token scope. Token refresh happens dynamically when API calls detect token expiration. The batch size can be configured via the `ABAC_BATCH_SIZE` environment variable (default: 10).


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
