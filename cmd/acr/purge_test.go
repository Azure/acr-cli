// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package main

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/mocks"
	"github.com/Azure/acr-cli/cmd/worker"
	"github.com/stretchr/testify/assert"
)

// The following tests involve deleting tags.
// TestOneTagDeletion there is only one tag and it should be deleted, the DeleteAcrTag method should be called once.
func TestOneTagDeletion(t *testing.T) {
	loginURL := "foo.azurecr.io"
	repo := "bar"
	tagName := "latest"
	digest := "sha:abc"
	deleteEnabled := true
	lastUpdateTime := time.Now().Add(-15 * time.Minute).UTC().Format(time.RFC3339Nano) //Creation time -15minutes from current time
	OneTagResult := &acr.RepositoryTagsType{
		Registry:  &loginURL,
		ImageName: &repo,
		TagsAttributes: &[]acr.TagAttributesBase{
			{
				Name:                 &tagName,
				LastUpdateTime:       &lastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
				Digest:               &digest,
			},
		},
	}
	EmptyTagResult := &acr.RepositoryTagsType{
		Registry:       &loginURL,
		ImageName:      &repo,
		TagsAttributes: nil,
	}
	ctx := context.Background()
	assert := assert.New(t)
	mockClient := new(mocks.AcrCLIClientInterface)
	worker.StartDispatcher(&wg, mockClient, 6)
	mockClient.On("GetAcrTags", ctx, repo, "", "").Return(OneTagResult, nil).Once()
	mockClient.On("GetAcrTags", ctx, repo, "", "latest").Return(EmptyTagResult, nil).Once()
	mockClient.On("DeleteAcrTag", ctx, repo, "latest").Return(nil, nil).Once()
	deletedTags, err := PurgeTags(ctx, mockClient, loginURL, repo, "0m", "^la.*")
	assert.Equal(1, deletedTags, "Number of deleted elements should be 1")
	assert.Equal(nil, err, "Error should be nil")
	mockClient.AssertExpectations(t)
}

// TestFiveTagDeletion all tags should be deleted, 5 tags in total, separated into two GetAcrTags calls, there should be
// 5 DeleteAcrTag calls.
func TestFiveTagDeletion(t *testing.T) {
	loginURL := "foo.azurecr.io"
	repo := "bar"
	tagName := "latest"
	digest := "sha:abc"
	deleteEnabled := true
	lastUpdateTime := time.Now().Add(-15 * time.Minute).UTC().Format(time.RFC3339Nano) //Creation time -15minutes from current time
	OneTagResult := &acr.RepositoryTagsType{
		Registry:  &loginURL,
		ImageName: &repo,
		TagsAttributes: &[]acr.TagAttributesBase{
			{
				Name:                 &tagName,
				LastUpdateTime:       &lastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
				Digest:               &digest,
			},
		},
	}
	EmptyTagResult := &acr.RepositoryTagsType{
		Registry:       &loginURL,
		ImageName:      &repo,
		TagsAttributes: nil,
	}
	tagName1 := "v1"
	tagName2 := "v2"
	tagName3 := "v3"
	tagName4 := "v4"

	FourTagsResult := &acr.RepositoryTagsType{
		Registry:  &loginURL,
		ImageName: &repo,
		TagsAttributes: &[]acr.TagAttributesBase{{
			Name:                 &tagName1,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
			Digest:               &digest,
		}, {
			Name:                 &tagName2,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
			Digest:               &digest,
		}, {
			Name:                 &tagName3,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
			Digest:               &digest,
		}, {
			Name:                 &tagName4,
			LastUpdateTime:       &lastUpdateTime,
			ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
			Digest:               &digest,
		}},
	}
	ctx := context.Background()
	assert := assert.New(t)
	mockClient := new(mocks.AcrCLIClientInterface)
	worker.StartDispatcher(&wg, mockClient, 6)
	mockClient.On("GetAcrTags", ctx, repo, "", "").Return(OneTagResult, nil).Once()
	mockClient.On("GetAcrTags", ctx, repo, "", "latest").Return(FourTagsResult, nil).Once()
	mockClient.On("GetAcrTags", ctx, repo, "", "v4").Return(EmptyTagResult, nil).Once()
	mockClient.On("DeleteAcrTag", ctx, repo, "latest").Return(nil, nil).Once()
	mockClient.On("DeleteAcrTag", ctx, repo, "v1").Return(nil, nil).Once()
	mockClient.On("DeleteAcrTag", ctx, repo, "v2").Return(nil, nil).Once()
	mockClient.On("DeleteAcrTag", ctx, repo, "v3").Return(nil, nil).Once()
	mockClient.On("DeleteAcrTag", ctx, repo, "v4").Return(nil, nil).Once()
	deletedTags, err := PurgeTags(ctx, mockClient, loginURL, repo, "0m", "[\\s\\S]*")
	assert.Equal(5, deletedTags, "Number of deleted elements should be 5")
	assert.Equal(nil, err, "Error should be nil")
	mockClient.AssertExpectations(t)
}

// TestDeleteTagError if an error (other than a 404 error) occurs during delete, an error should be returned.
func TestDeleteTagError(t *testing.T) {
	loginURL := "foo.azurecr.io"
	repo := "bar"
	tagName := "latest"
	digest := "sha:abc"
	deleteEnabled := true
	lastUpdateTime := time.Now().Add(-15 * time.Minute).UTC().Format(time.RFC3339Nano) //Creation time -15minutes from current time
	OneTagResult := &acr.RepositoryTagsType{
		Registry:  &loginURL,
		ImageName: &repo,
		TagsAttributes: &[]acr.TagAttributesBase{
			{
				Name:                 &tagName,
				LastUpdateTime:       &lastUpdateTime,
				ChangeableAttributes: &acr.ChangeableAttributes{DeleteEnabled: &deleteEnabled},
				Digest:               &digest,
			},
		},
	}
	ctx := context.Background()
	assert := assert.New(t)
	mockClient := new(mocks.AcrCLIClientInterface)
	worker.StartDispatcher(&wg, mockClient, 6)
	mockClient.On("GetAcrTags", ctx, repo, "", "").Return(OneTagResult, nil).Once()
	mockClient.On("DeleteAcrTag", ctx, repo, "latest").Return(nil, errors.New("error during delete")).Once()
	deletedTags, err := PurgeTags(ctx, mockClient, loginURL, repo, "0m", "^la.*")
	assert.Equal(-1, deletedTags, "Number of deleted elements should be -1")
	assert.NotEqual(nil, err, "Error should not be nil")
	mockClient.AssertExpectations(t)
}

func TestGetRepositoryAndTagRegex(t *testing.T) {
	// First test normal functionality
	t.Run("NormalFunctionalityTest", func(t *testing.T) {
		assert := assert.New(t)
		testString := "foo:bar"
		repository, filter, err := GetRepositoryAndTagRegex(testString)
		assert.Equal("foo", repository)
		assert.Equal("bar", filter)
		assert.Equal(nil, err, "Error should be nil")
	})
	// Second test no colon
	t.Run("NoColonTest", func(t *testing.T) {
		assert := assert.New(t)
		testString := "foo"
		repository, filter, err := GetRepositoryAndTagRegex(testString)
		assert.Equal("", repository)
		assert.Equal("", filter)
		assert.NotEqual(nil, err, "Error should not be nil")
	})
	// Third test more than one colon
	t.Run("TwoColonsTest", func(t *testing.T) {
		assert := assert.New(t)
		testString := "foo:bar:zzz"
		repository, filter, err := GetRepositoryAndTagRegex(testString)
		assert.Equal("", repository)
		assert.Equal("", filter)
		assert.NotEqual(nil, err, "Error should not be nil")
	})
}

func TestParseDuration(t *testing.T) {
	tables := []struct {
		durationString string
		duration       time.Duration
		err            error
	}{
		{"15m", -15 * time.Minute, nil},
		{"1d1h3m", -25*time.Hour - 3*time.Minute, nil},
		{"3d", -3 * 24 * time.Hour, nil},
		{"", 0, io.EOF},
		{"15p", 0, errors.New("time: unknown unit p in duration 15p")},
		{"15", 0 * time.Minute, errors.New("time: missing unit in duration 15")},
	}
	assert := assert.New(t)
	for _, table := range tables {
		durationResult, errorResult := ParseDuration(table.durationString)
		assert.Equal(table.duration, durationResult)
		assert.Equal(table.err, errorResult)
	}
}
