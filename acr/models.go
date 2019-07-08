package acr

// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See License.txt in the project root for license information.
//
// Code generated by Microsoft (R) AutoRest Code Generator.
// Changes may cause incorrect behavior and will be lost if the code is regenerated.

import (
	"github.com/Azure/go-autorest/autorest"
)

// AcrDeleteManifestMetadataBadRequestResponse ...
type AcrDeleteManifestMetadataBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrDeleteManifestMetadataNotFoundResponse ...
type AcrDeleteManifestMetadataNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrDeleteManifestMetadataUnauthorizedResponse ...
type AcrDeleteManifestMetadataUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrDeleteRepositoryBadRequestResponse ...
type AcrDeleteRepositoryBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrDeleteRepositoryMetadataBadRequestResponse ...
type AcrDeleteRepositoryMetadataBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrDeleteRepositoryMetadataNotFoundResponse ...
type AcrDeleteRepositoryMetadataNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrDeleteRepositoryMetadataUnauthorizedResponse ...
type AcrDeleteRepositoryMetadataUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrDeleteRepositoryNotFoundResponse ...
type AcrDeleteRepositoryNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrDeleteRepositoryUnauthorizedResponse ...
type AcrDeleteRepositoryUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrDeleteTagBadRequestResponse ...
type AcrDeleteTagBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrDeleteTagMetadataBadRequestResponse ...
type AcrDeleteTagMetadataBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrDeleteTagMetadataNotFoundResponse ...
type AcrDeleteTagMetadataNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrDeleteTagMetadataUnauthorizedResponse ...
type AcrDeleteTagMetadataUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrDeleteTagNotFoundResponse ...
type AcrDeleteTagNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrDeleteTagUnauthorizedResponse ...
type AcrDeleteTagUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetManifestAttributesBadRequestResponse ...
type AcrGetManifestAttributesBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetManifestAttributesNotFoundResponse ...
type AcrGetManifestAttributesNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetManifestAttributesOKResponse ...
type AcrGetManifestAttributesOKResponse struct {
	Data *ManifestAttributes `json:"data,omitempty"`
}

// AcrGetManifestAttributesUnauthorizedResponse ...
type AcrGetManifestAttributesUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetManifestMetadataBadRequestResponse ...
type AcrGetManifestMetadataBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetManifestMetadataNotFoundResponse ...
type AcrGetManifestMetadataNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetManifestMetadataUnauthorizedResponse ...
type AcrGetManifestMetadataUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetRepositoryAttributesBadRequestResponse ...
type AcrGetRepositoryAttributesBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetRepositoryAttributesNotFoundResponse ...
type AcrGetRepositoryAttributesNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetRepositoryAttributesOKResponse ...
type AcrGetRepositoryAttributesOKResponse struct {
	Data *RepositoryAttributes `json:"data,omitempty"`
}

// AcrGetRepositoryAttributesUnauthorizedResponse ...
type AcrGetRepositoryAttributesUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetRepositoryMetadataBadRequestResponse ...
type AcrGetRepositoryMetadataBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetRepositoryMetadataNotFoundResponse ...
type AcrGetRepositoryMetadataNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetRepositoryMetadataUnauthorizedResponse ...
type AcrGetRepositoryMetadataUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetTagAttributesBadRequestResponse ...
type AcrGetTagAttributesBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetTagAttributesNotFoundResponse ...
type AcrGetTagAttributesNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetTagAttributesOKResponse ...
type AcrGetTagAttributesOKResponse struct {
	Data *TagAttributes `json:"data,omitempty"`
}

// AcrGetTagAttributesUnauthorizedResponse ...
type AcrGetTagAttributesUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetTagMetadataBadRequestResponse ...
type AcrGetTagMetadataBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetTagMetadataNotFoundResponse ...
type AcrGetTagMetadataNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrGetTagMetadataUnauthorizedResponse ...
type AcrGetTagMetadataUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListManifestMetadataBadRequestResponse ...
type AcrListManifestMetadataBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListManifestMetadataNotFoundResponse ...
type AcrListManifestMetadataNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListManifestMetadataOKResponse ...
type AcrListManifestMetadataOKResponse struct {
	Data *ManifestMetadataList `json:"data,omitempty"`
}

// AcrListManifestMetadataUnauthorizedResponse ...
type AcrListManifestMetadataUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListManifestsBadRequestResponse ...
type AcrListManifestsBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListManifestsNotFoundResponse ...
type AcrListManifestsNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListManifestsOKResponse ...
type AcrListManifestsOKResponse struct {
	Data *ManifestAttributeList `json:"data,omitempty"`
}

// AcrListManifestsUnauthorizedResponse ...
type AcrListManifestsUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListRepositoriesBadRequestResponse ...
type AcrListRepositoriesBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListRepositoriesOKResponse ...
type AcrListRepositoriesOKResponse struct {
	Data *[]string `json:"data,omitempty"`
}

// AcrListRepositoriesUnauthorizedResponse ...
type AcrListRepositoriesUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListRepositoryMetadataBadRequestResponse ...
type AcrListRepositoryMetadataBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListRepositoryMetadataNotFoundResponse ...
type AcrListRepositoryMetadataNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListRepositoryMetadataOKResponse ...
type AcrListRepositoryMetadataOKResponse struct {
	Data *RepositoryMetadata `json:"data,omitempty"`
}

// AcrListRepositoryMetadataUnauthorizedResponse ...
type AcrListRepositoryMetadataUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListTagMetadataBadRequestResponse ...
type AcrListTagMetadataBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListTagMetadataNotFoundResponse ...
type AcrListTagMetadataNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListTagMetadataOKResponse ...
type AcrListTagMetadataOKResponse struct {
	Data *TagMetadataList `json:"data,omitempty"`
}

// AcrListTagMetadataUnauthorizedResponse ...
type AcrListTagMetadataUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListTagsBadRequestResponse ...
type AcrListTagsBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListTagsNotFoundResponse ...
type AcrListTagsNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrListTagsOKResponse ...
type AcrListTagsOKResponse struct {
	Data *TagAttributeList `json:"data,omitempty"`
}

// AcrListTagsUnauthorizedResponse ...
type AcrListTagsUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateManifestAttributesBadRequestResponse ...
type AcrUpdateManifestAttributesBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateManifestAttributesNotFoundResponse ...
type AcrUpdateManifestAttributesNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateManifestAttributesUnauthorizedResponse ...
type AcrUpdateManifestAttributesUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateManifestMetadataBadRequestResponse ...
type AcrUpdateManifestMetadataBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateManifestMetadataNotFoundResponse ...
type AcrUpdateManifestMetadataNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateManifestMetadataUnauthorizedResponse ...
type AcrUpdateManifestMetadataUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateRepositoryAttributesBadRequestResponse ...
type AcrUpdateRepositoryAttributesBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateRepositoryAttributesNotFoundResponse ...
type AcrUpdateRepositoryAttributesNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateRepositoryAttributesUnauthorizedResponse ...
type AcrUpdateRepositoryAttributesUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateRepositoryMetadataBadRequestResponse ...
type AcrUpdateRepositoryMetadataBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateRepositoryMetadataNotFoundResponse ...
type AcrUpdateRepositoryMetadataNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateRepositoryMetadataUnauthorizedResponse ...
type AcrUpdateRepositoryMetadataUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateTagAttributesBadRequestResponse ...
type AcrUpdateTagAttributesBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateTagAttributesNotFoundResponse ...
type AcrUpdateTagAttributesNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateTagAttributesUnauthorizedResponse ...
type AcrUpdateTagAttributesUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateTagMetadataBadRequestResponse ...
type AcrUpdateTagMetadataBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateTagMetadataNotFoundResponse ...
type AcrUpdateTagMetadataNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// AcrUpdateTagMetadataUnauthorizedResponse ...
type AcrUpdateTagMetadataUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// CancelBlobUploadBadRequestResponse ...
type CancelBlobUploadBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// CancelBlobUploadNotFoundResponse ...
type CancelBlobUploadNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// CancelBlobUploadUnauthorizedResponse ...
type CancelBlobUploadUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// CheckDockerRegistryV2SupportUnauthorizedResponse ...
type CheckDockerRegistryV2SupportUnauthorizedResponse struct {
	autorest.Response `json:"-"`
	Data              *Error `json:"data,omitempty"`
}

// DeleteManifestBadRequestResponse ...
type DeleteManifestBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// DeleteManifestNotFoundResponse ...
type DeleteManifestNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// DeleteManifestUnauthorizedResponse ...
type DeleteManifestUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// EndBlobUploadBadRequestResponse ...
type EndBlobUploadBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// EndBlobUploadNotFoundResponse ...
type EndBlobUploadNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// EndBlobUploadUnauthorizedResponse ...
type EndBlobUploadUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// Error ...
type Error struct {
	Errors *[]ErrorErrorsItem `json:"errors,omitempty"`
}

// ErrorErrorsItem ...
type ErrorErrorsItem struct {
	Code    *string `json:"code,omitempty"`
	Message *string `json:"message,omitempty"`
	Detail  *string `json:"detail,omitempty"`
}

// GetBlobBadRequestResponse ...
type GetBlobBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// GetBlobNotFoundResponse ...
type GetBlobNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// GetBlobUnauthorizedResponse ...
type GetBlobUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// GetBlobUploadStatusBadRequestResponse ...
type GetBlobUploadStatusBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// GetBlobUploadStatusNotFoundResponse ...
type GetBlobUploadStatusNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// GetBlobUploadStatusUnauthorizedResponse ...
type GetBlobUploadStatusUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// GetManifestBadRequestResponse ...
type GetManifestBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// GetManifestNotFoundResponse ...
type GetManifestNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// GetManifestOKResponse ...
type GetManifestOKResponse struct {
	Data *Layers `json:"data,omitempty"`
}

// GetManifestUnauthorizedResponse ...
type GetManifestUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// Layers ...
type Layers struct {
	Name      *string               `json:"name,omitempty"`
	Tag       *string               `json:"tag,omitempty"`
	FsLayers  *[]LayersFsLayersItem `json:"fsLayers,omitempty"`
	History   *string               `json:"history,omitempty"`
	Signature *string               `json:"signature,omitempty"`
}

// LayersFsLayersItem ...
type LayersFsLayersItem struct {
	BlobSum *string `json:"blobSum,omitempty"`
}

// ListRepositoriesBadRequestResponse ...
type ListRepositoriesBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// ListRepositoriesOKResponse ...
type ListRepositoriesOKResponse struct {
	Data *[]string `json:"data,omitempty"`
}

// ListRepositoriesUnauthorizedResponse ...
type ListRepositoriesUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// ListTagsNotFoundResponse ...
type ListTagsNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// ListTagsOKResponse ...
type ListTagsOKResponse struct {
	Name *string   `json:"name,omitempty"`
	Tags *[]string `json:"tags,omitempty"`
}

// ListTagsUnauthorizedResponse ...
type ListTagsUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// ManifestAttributeList ...
type ManifestAttributeList struct {
	Registry  *string                   `json:"registry,omitempty"`
	ImageName *string                   `json:"imageName,omitempty"`
	Manifests *[]ManifestAttributesBase `json:"manifests,omitempty"`
}

// ManifestAttributes ...
type ManifestAttributes struct {
	Registry  *string                     `json:"registry,omitempty"`
	ImageName *string                     `json:"imageName,omitempty"`
	Manifest  *ManifestAttributesManifest `json:"manifest,omitempty"`
}

// ManifestAttributesBase ...
type ManifestAttributesBase struct {
	Digest               *string                                     `json:"digest,omitempty"`
	CreatedTime          *string                                     `json:"createdTime,omitempty"`
	LastUpdateTime       *string                                     `json:"lastUpdateTime,omitempty"`
	Architecture         *string                                     `json:"architecture,omitempty"`
	Os                   *string                                     `json:"os,omitempty"`
	MediaType            *string                                     `json:"mediaType,omitempty"`
	Tags                 *[]string                                   `json:"tags,omitempty"`
	ChangeableAttributes *ManifestAttributesBaseChangeableAttributes `json:"changeableAttributes,omitempty"`
}

// ManifestAttributesBaseChangeableAttributes ...
type ManifestAttributesBaseChangeableAttributes struct {
	DeleteEnabled     *bool   `json:"deleteEnabled,omitempty"`
	WriteEnabled      *bool   `json:"writeEnabled,omitempty"`
	ListEnabled       *bool   `json:"listEnabled,omitempty"`
	ReadEnabled       *bool   `json:"readEnabled,omitempty"`
	QuarantineState   *string `json:"quarantineState,omitempty"`
	QuarantineDetails *string `json:"quarantineDetails,omitempty"`
}

// ManifestAttributesManifest ...
type ManifestAttributesManifest struct {
	References           *[]ManifestAttributesManifestReferencesItem `json:"references,omitempty"`
	QuarantineTag        *string                                     `json:"quarantineTag,omitempty"`
	Digest               *string                                     `json:"digest,omitempty"`
	CreatedTime          *string                                     `json:"createdTime,omitempty"`
	LastUpdateTime       *string                                     `json:"lastUpdateTime,omitempty"`
	Architecture         *string                                     `json:"architecture,omitempty"`
	Os                   *string                                     `json:"os,omitempty"`
	MediaType            *string                                     `json:"mediaType,omitempty"`
	Tags                 *[]string                                   `json:"tags,omitempty"`
	ChangeableAttributes *ManifestAttributesBaseChangeableAttributes `json:"changeableAttributes,omitempty"`
}

// ManifestAttributesManifestReferencesItem ...
type ManifestAttributesManifestReferencesItem struct {
	Digest       *string `json:"digest,omitempty"`
	Architecture *string `json:"architecture,omitempty"`
	Os           *string `json:"os,omitempty"`
}

// ManifestMetadataList ...
type ManifestMetadataList struct {
	Registry  *string   `json:"registry,omitempty"`
	ImageName *string   `json:"imageName,omitempty"`
	Digest    *string   `json:"digest,omitempty"`
	Metadata  *[]string `json:"metadata,omitempty"`
}

// RepositoryAttributes ...
type RepositoryAttributes struct {
	Registry             *string                                   `json:"registry,omitempty"`
	ImageName            *string                                   `json:"imageName,omitempty"`
	CreatedTime          *string                                   `json:"createdTime,omitempty"`
	LastUpdateTime       *string                                   `json:"lastUpdateTime,omitempty"`
	ManifestCount        *float64                                  `json:"manifestCount,omitempty"`
	TagCount             *float64                                  `json:"tagCount,omitempty"`
	ChangeableAttributes *RepositoryAttributesChangeableAttributes `json:"changeableAttributes,omitempty"`
}

// RepositoryAttributesChangeableAttributes ...
type RepositoryAttributesChangeableAttributes struct {
	DeleteEnabled *bool `json:"deleteEnabled,omitempty"`
	WriteEnabled  *bool `json:"writeEnabled,omitempty"`
	ListEnabled   *bool `json:"listEnabled,omitempty"`
	ReadEnabled   *bool `json:"readEnabled,omitempty"`
}

// RepositoryMetadata ...
type RepositoryMetadata struct {
	Registry  *string   `json:"registry,omitempty"`
	ImageName *string   `json:"imageName,omitempty"`
	Metadata  *[]string `json:"metadata,omitempty"`
}

// SetObject ...
type SetObject struct {
	autorest.Response `json:"-"`
	Value             interface{} `json:"value,omitempty"`
}

// StartBlobUploadBadRequestResponse ...
type StartBlobUploadBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// StartBlobUploadUnauthorizedResponse ...
type StartBlobUploadUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// TagAttributeList ...
type TagAttributeList struct {
	Registry  *string              `json:"registry,omitempty"`
	ImageName *string              `json:"imageName,omitempty"`
	Tags      *[]TagAttributesBase `json:"tags,omitempty"`
}

// TagAttributes ...
type TagAttributes struct {
	Registry  *string           `json:"registry,omitempty"`
	ImageName *string           `json:"imageName,omitempty"`
	Tag       *TagAttributesTag `json:"tag,omitempty"`
}

// TagAttributesBase ...
type TagAttributesBase struct {
	Name                 *string                                `json:"name,omitempty"`
	Digest               *string                                `json:"digest,omitempty"`
	CreatedTime          *string                                `json:"createdTime,omitempty"`
	LastUpdateTime       *string                                `json:"lastUpdateTime,omitempty"`
	Signed               *bool                                  `json:"signed,omitempty"`
	QuarantineState      *string                                `json:"quarantineState,omitempty"`
	ChangeableAttributes *TagAttributesBaseChangeableAttributes `json:"changeableAttributes,omitempty"`
}

// TagAttributesBaseChangeableAttributes ...
type TagAttributesBaseChangeableAttributes struct {
	DeleteEnabled *bool `json:"deleteEnabled,omitempty"`
	WriteEnabled  *bool `json:"writeEnabled,omitempty"`
	ListEnabled   *bool `json:"listEnabled,omitempty"`
	ReadEnabled   *bool `json:"readEnabled,omitempty"`
}

// TagAttributesTag ...
type TagAttributesTag struct {
	SignatureRecord      *string                                `json:"signatureRecord,omitempty"`
	Name                 *string                                `json:"name,omitempty"`
	Digest               *string                                `json:"digest,omitempty"`
	CreatedTime          *string                                `json:"createdTime,omitempty"`
	LastUpdateTime       *string                                `json:"lastUpdateTime,omitempty"`
	Signed               *bool                                  `json:"signed,omitempty"`
	QuarantineState      *string                                `json:"quarantineState,omitempty"`
	ChangeableAttributes *TagAttributesBaseChangeableAttributes `json:"changeableAttributes,omitempty"`
}

// TagMetadataList ...
type TagMetadataList struct {
	Registry  *string   `json:"registry,omitempty"`
	ImageName *string   `json:"imageName,omitempty"`
	TagName   *string   `json:"tagName,omitempty"`
	Metadata  *[]string `json:"metadata,omitempty"`
}

// UploadBlobContentBadRequestResponse ...
type UploadBlobContentBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// UploadBlobContentNotFoundResponse ...
type UploadBlobContentNotFoundResponse struct {
	Data *Error `json:"data,omitempty"`
}

// UploadBlobContentUnauthorizedResponse ...
type UploadBlobContentUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}

// UploadManifestBadRequestResponse ...
type UploadManifestBadRequestResponse struct {
	Data *Error `json:"data,omitempty"`
}

// UploadManifestUnauthorizedResponse ...
type UploadManifestUnauthorizedResponse struct {
	Data *Error `json:"data,omitempty"`
}