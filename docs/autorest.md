# Scenario: Azure Container Registry Metadata API SDK generation
> see https://aka.ms/autorest

---
## Getting Started
Prerequisite: [Install AutoRest](https://aka.ms/autorest/install)
---
## Code Generation
User can use autorest to generate SDK based on the swagger file.
For example, enter "autorest autorest.md --output-sdk-folder=. --go" will generate golang SDK in folder "golang".

## Autorest settings
The following sections are autorest config.

### Global
These are the global settings.
``` yaml
openapi-type: metadata
license-header: MICROSOFT_MIT_NO_VERSION
namespace: acr
clear-output-folder: true
input-file: swagger.yaml
```

### Golang settings
These settings apply only when `--go` is specified on the command line.
``` yaml $(go)
output-folder: $(output-sdk-folder)
```