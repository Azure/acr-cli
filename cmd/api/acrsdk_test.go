// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package api

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

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

func TestGetExpiration(t *testing.T) {
	// EmptyToken contains no authentication data
	testToken := "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJUZXN0VG9rZW4iLCJpYXQiOjE1NjM5MDk2NTIsImV4cCI6MTU2MzkxMDk4MSwiYXVkIjoiZXhhbXBsZS5henVyZWNyLmlvIiwic3ViIjoiZXhhbXBsZUBleGFtcGxlLmNvbSJ9.Ivw5oOSwMZYGKCzlogsguIIH9UDmKXIixdlgXEfo2dk" //nolint:gosec
	expectedReturn := int64(1563910981)
	exp, err := getExpiration(testToken)
	if err != nil {
		t.Fatal("Unexpected error while parsing token")
	}
	if exp != expectedReturn {
		t.Fatalf("getExpiration incorrect, got %d, expected %d", exp, expectedReturn)
	}

	testToken = "token"
	_, err = getExpiration(testToken)
	if err == nil {
		t.Fatal("Expected error while parsing token, got nil")
	}
}

// func TestRefresh(t *testing.T) {
// 	config := `{
// 		"auths": {
// 			"myregistry.azurecr.io": {
// 				"auth": "MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwOg==",
// 				"identitytoken": "abc"
// 			}
// 		}
// 	}`
// 	dir := t.TempDir()
// 	tmpFile := filepath.Join(dir, "testconfig.json")
// 	f, _ := os.OpenFile(tmpFile, 0644, fs.FileMode(os.O_CREATE))
// 	json.NewEncoder(f).Encode(config)
// 	client, err := GetAcrCLIClientWithAuth("myregistry.azurecr.io", "00000000-0000-0000-0000-000000000000", "", []string{tmpFile})
// 	client.
// }

func TestGetAcrCLIClientWithAuth(t *testing.T) {
	loginURL := "myregistry.azurecr.io"
	dockerConfigUsername := `{
		"auths": {
			"myregistry.azurecr.io": {
				"auth": "test_username",
				"identitytoken": "test_token"
			}
		}
	}`
	dockerConfigNoUsername := `{
		"auths": {
			"myregistry.azurecr.io": {
				"auth": "MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwOg==",
				"identitytoken": "test_token"
			}
		}
	}`
	// case 2: both password & username empty, read from docker config, username not empty
	// case 3: read from docker config, username "000"
	// case 1: password is empty, fail
	// case 4: empty username only
	// case 5: nonempty username and password

	tests := []struct {
		name          string
		username      string
		password      string
		configContent string
		wantErr       bool
		wantValue1    string
		wantValue2    string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tmpFile := filepath.Join(dir, "testconfig.json")
			f, _ := os.OpenFile(tmpFile, 0644, fs.FileMode(os.O_CREATE))
			json.NewEncoder(f).Encode(tt.configContent) // change this

			got, err := GetAcrCLIClientWithAuth(loginURL, tt.username, tt.password, []string{tmpFile})
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAcrCLIClientWithAuth() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// check the got's username & password
			if got.AutorestClient.user
		})
	}
}
