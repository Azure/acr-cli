// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"testing"

	"github.com/Azure/acr-cli/internal/common"
	"github.com/stretchr/testify/assert"
)

func TestNewCsscCmd(t *testing.T) {
	rootParams := &rootParameters{}
	cmd := newCsscCmd(rootParams)
	assert.NotNil(t, cmd)
	assert.Equal(t, "cssc", cmd.Use)
	assert.Equal(t, newCsscCmdLongMessage, cmd.Long)
}

func TestNewPatchFilterCmd(t *testing.T) {
	rootParams := &rootParameters{}
	csscParams := csscParameters{rootParameters: rootParams}
	cmd := newPatchFilterCmd(&csscParams)
	assert.NotNil(t, cmd)
	assert.Equal(t, "patch", cmd.Use)
	assert.Equal(t, newPatchCmdLongMessage, cmd.Long)
}

func TestGetRegistryCredsFromStore(t *testing.T) {
	rootParams := &rootParameters{}
	rootParams.configs = []string{"config1", "config2"}
	csscParams := csscParameters{rootParameters: rootParams}
	// 1. Should not get the creds from the store when creds are provided
	t.Run("CredsProvidedTest", func(t *testing.T) {
		rootParams.username = "username"
		rootParams.password = "password"
		getRegistryCredsFromStore(&csscParams, common.TestLoginURL)
		assert.Equal(t, "username", csscParams.username)
		assert.Equal(t, "password", csscParams.password)
	})
	// 2. When creds are not provided, should get the creds from the store
	t.Run("CredsNotProvidedTest", func(t *testing.T) {
		rootParams.username = ""
		rootParams.password = ""
		getRegistryCredsFromStore(&csscParams, common.TestLoginURL)
		assert.Equal(t, "", csscParams.username)
		assert.Equal(t, "", csscParams.password)
	})
}
