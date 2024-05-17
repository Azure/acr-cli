// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewTagCmd(t *testing.T) {
	rootParams := &rootParameters{}
	cmd := newTagCmd(rootParams)
	assert.NotNil(t, cmd)
	assert.Equal(t, "tag", cmd.Use)
	assert.Equal(t, newTagCmdLongMessage, cmd.Long)
}

func TestNewTagListCmd(t *testing.T) {
	rootParams := &rootParameters{}
	tagParams := tagParameters{rootParameters: rootParams}
	cmd := newTagListCmd(&tagParams)
	assert.NotNil(t, cmd)
	assert.Equal(t, "list", cmd.Use)
	assert.Equal(t, newTagListCmdLongMessage, cmd.Long)
}

func TestNewTagDeleteCmd(t *testing.T) {
	rootParams := &rootParameters{}
	tagParams := tagParameters{rootParameters: rootParams}
	cmd := newTagDeleteCmd(&tagParams)
	assert.NotNil(t, cmd)
	assert.Equal(t, "delete", cmd.Use)
	assert.Equal(t, newTagDeleteCmdLongMessage, cmd.Long)
}
