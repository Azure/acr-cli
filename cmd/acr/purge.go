// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"fmt"
	"io"

	acrapi "github.com/Azure/libacr/golang"
	"github.com/spf13/cobra"
)

const (
	purgeLongMessage = `` // TODO
)

func newPurgeCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "purge",
		Short: "", // TODO
		Long:  purgeLongMessage,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("The default base uri is:", acrapi.DefaultBaseURI)
			return nil
		},
	}

	return cmd
}
