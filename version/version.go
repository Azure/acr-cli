// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

// Package version provides version information for the ACR CLI.
package version

var (
	// Version holds the semantic version. Filled in at linking time.
	Version = ""

	// Revision is filled with the VCS revision. Filled in at linking time.
	Revision = ""
)

// FullVersion generates a string that contains the version and revision
// information, if any is available.
func FullVersion() string {
	if Revision == "" {
		return Version
	}
	return Version + "+" + Revision
}
