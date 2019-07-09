// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package docker

import "github.com/docker/docker/registry"

func resolveHostname(hostname string) string {
	switch hostname {
	case registry.IndexHostname, registry.IndexName, registry.DefaultV2Registry.Host:
		return registry.IndexServer
	}
	return hostname
}
