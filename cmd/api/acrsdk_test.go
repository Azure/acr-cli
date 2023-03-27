// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
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
	var testLoginURL string
	testTokenScope := "registry:catalog:* repository:*:*"
	testAccessToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE1NDk2NTEwMzEsInN1YiI6IjEyMzQ1Njc4OTAiLCJuYW1lIjoiSm9obiBEb2UiLCJhZG1pbiI6dHJ1ZSwiaWF0IjoxNTE2MjM5MDIyfQ.JamFfuEx54E0ZqGFx8EUe745GisoP0YCTAup-8YB2Tb4nouKfZGBbHb-YRtEqi-r8uQzihJBRi1GDxUNzDxbsLB5fe8Q8uLN8IpTHmWIMGpoD7E9W5W8-hUJiu4qG6lPZMV7e0JDsik-xUPcFjv8yJQmKl84vwDFRpH0uNhBDs_H6YnIGyso1I5ITkeSiUVpdlkeOUQ1wK-k-rAU8zYUSCj_3ke0zHQcHFXRiHsmyUMZBKNmPUTZp1CSBWx90zKGBmhgt26LHl1EmmoddRbuigReOqT_HnaTcTrrudweLPXubjHMtAFym5RDzGsIae4mkClKV6iMVweM73A2NWQNkg"
	testRefreshToken := "test/refresh/token"

	// create an authorization server
	as := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			panic("unexpected access")
		}
		switch path {
		case "/oauth2/token":
			if err := r.ParseForm(); err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				panic("failed to parse form")
			}
			if got := r.PostForm.Get("service"); got != testLoginURL {
				w.WriteHeader(http.StatusUnauthorized)
			}
			// handles refresh token requests
			if got := r.PostForm.Get("grant_type"); got != "refresh_token" {
				w.WriteHeader(http.StatusUnauthorized)
			}
			if got := r.PostForm.Get("scope"); got != testTokenScope {
				w.WriteHeader(http.StatusUnauthorized)
			}
			if got := r.PostForm.Get("refresh_token"); got != testRefreshToken {
				w.WriteHeader(http.StatusUnauthorized)
			}
			// writes back access token
			if _, err := fmt.Fprintf(w, `{"access_token":%q}`, testAccessToken); err != nil {
				panic(err)
			}
		default:
			w.WriteHeader(http.StatusNotAcceptable)
		}
	}))
	defer as.Close()
	testLoginURL = as.URL
	sender := autorest.CreateSender()
	sender.(*http.Client).Transport = as.Client().Transport

	dockerConfigWithUsernameAndPassword := []byte(`{"auths":{"myregistry.azurecr.io":{"auth":"aGVsbG86b3Jhcy10ZXN0"}}}`)
	dockerConfigNoUsername := []byte(fmt.Sprintf(`{"auths":{"%s":{"auth":"MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwOg==","identitytoken":"test/refresh/token"}}}`, testLoginURL))
	dockerConfigWithUsernameOnly := []byte(`{"auths":{"myregistry.azurecr.io":{"auth":"aGVsbG86"}}}`)

	tests := []struct {
		name             string
		loginURL         string
		username         string
		password         string
		configContent    []byte
		wantErr          bool
		useBasicAuth     bool
		wantUsername     string
		wantPassword     string
		wantAccessToken  string
		wantRefreshToken string
	}{
		{
			name:          "empty username and password, read from docker config with regular username and password",
			loginURL:      "myregistry.azurecr.io",
			username:      "",
			password:      "",
			configContent: dockerConfigWithUsernameAndPassword,
			wantErr:       false,
			useBasicAuth:  true,
			wantUsername:  "hello",
			wantPassword:  "oras-test",
		},
		{
			name:             "empty username and password, read from docker config with username 00000000-0000-0000-0000-000000000000",
			loginURL:         testLoginURL,
			username:         "",
			password:         "",
			configContent:    dockerConfigNoUsername,
			wantErr:          false,
			useBasicAuth:     false,
			wantAccessToken:  testAccessToken,
			wantRefreshToken: testRefreshToken,
		},
		{
			name:     "password is empty, fail with an error",
			loginURL: "myregistry.azurecr.io",
			username: "test user",
			password: "",
			wantErr:  true,
		},
		{
			name:          "empty username and password, read from docker config with username only, fail with an error",
			loginURL:      "myregistry.azurecr.io",
			username:      "",
			password:      "",
			configContent: dockerConfigWithUsernameOnly,
			wantErr:       true,
		},
		{
			name:             "empty username, refresh token as password",
			loginURL:         testLoginURL,
			username:         "",
			password:         testRefreshToken,
			wantErr:          false,
			useBasicAuth:     false,
			wantAccessToken:  testAccessToken,
			wantRefreshToken: testRefreshToken,
		},
		{
			name:         "non-empty username and password",
			loginURL:     "myregistry.azurecr.io",
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
			got, err := GetAcrCLIClientWithAuth(tt.loginURL, tt.username, tt.password, []string{configFilePath})
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAcrCLIClientWithAuth() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				var wantAuthorizer autorest.Authorizer
				if tt.useBasicAuth {
					wantAuthorizer = autorest.NewBasicAuthorizer(tt.wantUsername, tt.wantPassword)
				} else {
					wantAuthorizer = autorest.NewBearerAuthorizer(&adal.Token{
						AccessToken:  tt.wantAccessToken,
						RefreshToken: tt.wantRefreshToken,
					})
				}
				if !reflect.DeepEqual(got.AutorestClient.Authorizer, wantAuthorizer) {
					t.Error("incorrect AutorestClient.Authorizer")
				}
			}
		})
	}
}
