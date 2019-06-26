// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package api

import "testing"

func TestBasicAuth(t *testing.T) {
	expectedReturn := "Basic cmVnaXN0cnl1c2VyOnJlZ2lzdHJ5dXNlcnBhc3N3b3Jk"
	username := "registryuser"
	password := "registryuserpassword"
	auth := BasicAuth(username, password)

	if auth != expectedReturn {
		t.Fatalf("Basic auth of %s:%s incorrect, got %s, expected %s", username, password, auth, expectedReturn)
	}
}

func TestLoginURLWithPrefix(t *testing.T) {
	expectedReturn := "https://registry.azurecr.io"
	originalHostname := "registry.azurecr.io"
	hostname := LoginURLWithPrefix(originalHostname)

	if hostname != expectedReturn {
		t.Fatalf("GetHostname of %s incorrect, got %s, expected %s", originalHostname, hostname, expectedReturn)
	}

	originalHostname = "https://registry.azurecr.io"
	hostname = LoginURLWithPrefix(originalHostname)

	if hostname != expectedReturn {
		t.Fatalf("GetHostname of %s incorrect, got %s, expected %s", originalHostname, hostname, expectedReturn)
	}
}

func TestLoginURL(t *testing.T) {
	expectedReturn := "registry.azurecr.io"
	registryName := "registry"
	loginURL := LoginURL(registryName)

	if loginURL != expectedReturn {
		t.Fatalf("LoginURL of %s incorrect, got %s, expected %s", registryName, loginURL, expectedReturn)
	}

	expectedReturn = "registry.azurecr-test.io"
	registryName = "registry.azurecr-test.io"
	loginURL = LoginURL(registryName)

	if loginURL != expectedReturn {
		t.Fatalf("LoginURL of %s incorrect, got %s, expected %s", registryName, loginURL, expectedReturn)
	}
}
