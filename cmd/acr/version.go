// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"fmt"

	"github.com/Azure/acr-cli/version"
	"github.com/spf13/cobra"
)

const (
	versionLongMessage = `
Prints version information
`
)

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  versionLongMessage,
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Printf(`Version: %s, Revision: %s
`, version.Version, version.Revision)
			return nil
		},
	}

	return cmd
}
