// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package tag

import (
	"context"
	"fmt"

	"github.com/Azure/acr-cli/acr"
	"github.com/Azure/acr-cli/cmd/api"
	"github.com/pkg/errors"
)

// ListTags will do the http requests and return the digest of all the tags in the selected repository.
func ListTags(ctx context.Context, acrClient api.AcrCLIClientInterface, repoName string) ([]acr.TagAttributesBase, error) {

	lastTag := ""
	resultTags, err := acrClient.GetAcrTags(ctx, repoName, "", lastTag)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list tags")
	}

	var tagList []acr.TagAttributesBase
	tagList = append(tagList, *resultTags.TagsAttributes...)

	// A for loop is used because the GetAcrTags method returns by default only 100 tags and their attributes.
	for resultTags != nil && resultTags.TagsAttributes != nil {
		tags := *resultTags.TagsAttributes

		// Since the GetAcrTags supports pagination when supplied with the last digest that was returned the last tag name
		// digest is saved, the tag array contains at least one element because if it was empty the API would return
		// a nil pointer instead of a pointer to a length 0 array.
		lastTag = *tags[len(tags)-1].Name
		resultTags, err = acrClient.GetAcrTags(ctx, repoName, "", lastTag)
		if err != nil {
			return nil, err
		}
		if resultTags != nil && resultTags.TagsAttributes != nil {
			tagList = append(tagList, *resultTags.TagsAttributes...)
		}
	}

	return tagList, nil
}

// DeleteTags receives an array of tags digest and deletes them using the supplied acrClient.
func DeleteTags(ctx context.Context, acrClient api.AcrCLIClientInterface, loginURL string, repoName string, args []string) error {
	for i := 0; i < len(args); i++ {
		_, err := acrClient.DeleteAcrTag(ctx, repoName, args[i])
		if err != nil {
			// If there is an error (this includes not found and not allowed operations) the deletion of the tags is stopped and an error is returned.
			return errors.Wrap(err, "failed to delete tags")
		}
		fmt.Printf("%s/%s:%s\n", loginURL, repoName, args[i])
	}
	return nil
}
