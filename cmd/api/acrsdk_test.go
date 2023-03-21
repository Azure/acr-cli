// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package api

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Azure/go-autorest/autorest"
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

func TestGetAcrCLIClientWithAuth(t *testing.T) {
	loginURL := "myregistry.azurecr.io"
	dockerConfigWithUsernameAndPassword := []byte(`{"auths":{"myregistry.azurecr.io":{"auth":"aGVsbG86b3Jhcy10ZXN0"}}}`)
	// dockerConfigNoUsername := []byte(`{"auths":{"myregistry.azurecr.io":{"auth":"MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwOg==","identitytoken":"test refresh token"}}}`)
	dockerConfigWithUsernameOnly := []byte(`{"auths":{"myregistry.azurecr.io":{"auth":"aGVsbG86"}}}`)
	tests := []struct {
		name           string
		username       string
		password       string
		configContent  []byte
		wantErr        bool
		useBasicAuth   bool
		wantUsername   string
		wantPassword   string
		useBearerToken bool
	}{
		{
			name:          "empty username and password, read from docker config with regular username and password",
			username:      "",
			password:      "",
			configContent: dockerConfigWithUsernameAndPassword,
			wantErr:       false,
			useBasicAuth:  true,
			wantUsername:  "hello",
			wantPassword:  "oras-test",
		},
		// To be continue
		// {
		// 	name:          "empty username and password, read from docker config with username 00000000-0000-0000-0000-000000000000",
		// 	username:      "",
		// 	password:      "",
		// 	configContent: dockerConfigNoUsername,
		// 	wantErr:       false,
		// },
		{
			name:     "password is empty, fail with an error",
			username: "test user",
			password: "",
			wantErr:  true,
		},
		{
			name:          "empty username and password, read from docker config with username only, fail with an error",
			username:      "",
			password:      "",
			configContent: dockerConfigWithUsernameOnly,
			wantErr:       true,
		},
		// To be continue
		// {
		// 	name:         "empty username, refresh token as password",
		// 	username:     "",
		// 	password:     "test refresh token",
		// 	wantErr:      false,
		// 	useBasicAuth: false,
		// },
		{
			name:         "non-empty username and password",
			username:     "hello",
			password:     "oras-test",
			wantErr:      false,
			useBasicAuth: true,
			wantUsername: "hello",
			wantPassword: "oras-test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// create test docker config file
			configFilePath := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(configFilePath, tt.configContent, 0644); err != nil {
				t.Errorf("cannot create test config file: %v", err)
				return
			}
			got, err := GetAcrCLIClientWithAuth(loginURL, tt.username, tt.password, []string{configFilePath})
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAcrCLIClientWithAuth() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.useBasicAuth {
				wantAuthorizer := autorest.NewBasicAuthorizer(tt.wantUsername, tt.wantPassword)
				if !reflect.DeepEqual(got.AutorestClient.Authorizer, wantAuthorizer) {
					t.Error("incorrect AutorestClient.Authorizer")
				}
			}
		})
	}
}
