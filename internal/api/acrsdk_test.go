// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	testToken := strings.Join([]string{
		base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"exp":1563910981}`)),
		"",
	}, ".")
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
	testAccessToken := strings.Join([]string{
		base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"exp":1563910981}`)),
		"",
	}, ".")
	testRefreshToken := "test/refresh/token"

	// create an authorization server
	as := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			t.Fatalf("unexpected request method, get %s, expect POST", r.Method)
		}
		switch path {
		case "/oauth2/token":
			if err := r.ParseForm(); err != nil { //nolint:gosec // G120: test server, no risk of memory exhaustion
				w.WriteHeader(http.StatusUnauthorized)
				t.Fatal("unable to parse form")
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
				t.Fatalf("unable to write access token: %v", err)
			}
		default:
			w.WriteHeader(http.StatusNotAcceptable)
		}
	}))
	defer as.Close()
	testLoginURL = as.URL

	// As the autorest package enforces the use of https, we have to replace the
	// transport so that the client trusts the test server.
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
			if err := os.WriteFile(configFilePath, tt.configContent, 0600); err != nil {
				t.Errorf("cannot create test config file: %v", err)
				return
			}
			got, err := GetAcrCLIClientWithAuth(tt.loginURL, tt.username, tt.password, []string{configFilePath})
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAcrCLIClientWithAuth() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
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
		})
	}
}

// TestHasAadIdentityClaim tests the ABAC detection function
func TestHasAadIdentityClaim(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected bool
	}{
		{
			name: "token with aad_identity claim - ABAC enabled",
			// JWT with {"aad_identity": "user@example.com"} in payload
			token: strings.Join([]string{
				base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`)),
				base64.RawURLEncoding.EncodeToString([]byte(`{"exp":1563910981,"aad_identity":"user@example.com"}`)),
				"",
			}, "."),
			expected: true,
		},
		{
			name: "token without aad_identity claim - non-ABAC",
			// JWT without aad_identity
			token: strings.Join([]string{
				base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`)),
				base64.RawURLEncoding.EncodeToString([]byte(`{"exp":1563910981}`)),
				"",
			}, "."),
			expected: false,
		},
		{ //nolint:gosec // G101: not a real credential, test data
			name:     "invalid token",
			token:    "not-a-valid-jwt",
			expected: false,
		},
		{
			name:     "empty token",
			token:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasAadIdentityClaim(tt.token)
			if result != tt.expected {
				t.Errorf("hasAadIdentityClaim() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestAcrCLIClientIsAbac tests the IsAbac method
func TestAcrCLIClientIsAbac(t *testing.T) {
	tests := []struct {
		name     string
		isAbac   bool
		expected bool
	}{
		{
			name:     "ABAC enabled client",
			isAbac:   true,
			expected: true,
		},
		{
			name:     "non-ABAC client",
			isAbac:   false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := AcrCLIClient{
				isAbac: tt.isAbac,
			}
			result := client.IsAbac()
			if result != tt.expected {
				t.Errorf("IsAbac() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestRefreshAcrCLIClientTokenAbac tests the ABAC-aware token refresh path.
// This ensures that when SDK methods (GetAcrTags, DeleteAcrTag, etc.) detect token expiry,
// the refresh uses repository-scoped tokens for ABAC registries instead of wildcard scope.
func TestRefreshAcrCLIClientTokenAbac(t *testing.T) {
	testAccessToken := strings.Join([]string{
		base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"exp":9999999999}`)), // Far future expiry
		"",
	}, ".")
	testRefreshToken := "test/refresh/token"

	tests := []struct {
		name                string
		isAbac              bool
		currentRepositories []string
		repoName            string
		expectedScopePrefix string // What the scope should start with or contain
		shouldContainRepo   string // Specific repo that must be in scope
		wantErr             bool
	}{
		{
			name:                "ABAC with currentRepositories and repoName - includes both",
			isAbac:              true,
			currentRepositories: []string{"repo1", "repo2"},
			repoName:            "repo3",
			expectedScopePrefix: "repository:",
			shouldContainRepo:   "repo3",
			wantErr:             false,
		},
		{
			name:                "ABAC with only repoName - uses repoName for scope",
			isAbac:              true,
			currentRepositories: []string{},
			repoName:            "my-repo",
			expectedScopePrefix: "repository:my-repo:",
			shouldContainRepo:   "my-repo",
			wantErr:             false,
		},
		{
			name:                "ABAC with repoName already in currentRepositories - no duplicate",
			isAbac:              true,
			currentRepositories: []string{"repo1", "repo2"},
			repoName:            "repo1",
			expectedScopePrefix: "repository:",
			shouldContainRepo:   "repo1",
			wantErr:             false,
		},
		{
			name:                "ABAC with no repos and no repoName - returns error",
			isAbac:              true,
			currentRepositories: []string{},
			repoName:            "",
			wantErr:             true,
		},
		{
			name:                "Non-ABAC registry - uses wildcard scope",
			isAbac:              false,
			currentRepositories: []string{},
			repoName:            "any-repo",
			expectedScopePrefix: "repository:*:*",
			wantErr:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedScope string

			// Create a test server that captures the scope parameter
			as := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if err := r.ParseForm(); err != nil { //nolint:gosec // G120: test server, no risk of memory exhaustion
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				capturedScope = r.PostForm.Get("scope")
				// Return a valid access token
				fmt.Fprintf(w, `{"access_token":%q}`, testAccessToken)
			}))
			defer as.Close()

			// Create client with test configuration
			client := newAcrCLIClient(as.URL)
			client.isAbac = tt.isAbac
			client.currentRepositories = tt.currentRepositories
			client.token = &adal.Token{
				AccessToken:  testAccessToken,
				RefreshToken: testRefreshToken,
			}
			// Replace transport to trust test server
			client.AutorestClient.Sender = as.Client()

			// Call refreshAcrCLIClientToken
			err := refreshAcrCLIClientToken(context.Background(), &client, tt.repoName)

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("refreshAcrCLIClientToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Verify the scope was correct
			if tt.expectedScopePrefix != "" && !strings.Contains(capturedScope, tt.expectedScopePrefix) {
				t.Errorf("Expected scope to contain %q, got %q", tt.expectedScopePrefix, capturedScope)
			}

			if tt.shouldContainRepo != "" && !strings.Contains(capturedScope, tt.shouldContainRepo) {
				t.Errorf("Expected scope to contain repo %q, got %q", tt.shouldContainRepo, capturedScope)
			}

			// For ABAC, verify we're NOT using wildcard
			if tt.isAbac && strings.Contains(capturedScope, "repository:*:*") {
				t.Errorf("ABAC refresh should NOT use wildcard scope, got %q", capturedScope)
			}
		})
	}
}

// TestBuildAbacScope tests that buildAbacScope correctly maps repository names
// to token scope strings and handles the "catalog" sentinel value.
func TestBuildAbacScope(t *testing.T) {
	tests := []struct {
		name         string
		repositories []string
		wantContains []string
		wantExcludes []string
	}{
		{
			name:         "single repository",
			repositories: []string{"my-repo"},
			wantContains: []string{"repository:my-repo:pull,delete,metadata_read,metadata_write"},
		},
		{
			name:         "multiple repositories",
			repositories: []string{"repo1", "repo2"},
			wantContains: []string{
				"repository:repo1:pull,delete,metadata_read,metadata_write",
				"repository:repo2:pull,delete,metadata_read,metadata_write",
			},
		},
		{
			name:         "catalog sentinel maps to registry scope",
			repositories: []string{"catalog"},
			wantContains: []string{"registry:catalog:*"},
			wantExcludes: []string{"repository:catalog:"},
		},
		{
			name:         "catalog mixed with regular repos",
			repositories: []string{"catalog", "my-repo"},
			wantContains: []string{
				"registry:catalog:*",
				"repository:my-repo:pull,delete,metadata_read,metadata_write",
			},
			wantExcludes: []string{"repository:catalog:"},
		},
		{
			name:         "empty list returns empty string",
			repositories: []string{},
			wantContains: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildAbacScope(tt.repositories)

			if len(tt.repositories) == 0 {
				if got != "" {
					t.Errorf("buildAbacScope([]) = %q, want empty string", got)
				}
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("buildAbacScope() = %q, want it to contain %q", got, want)
				}
			}
			for _, exclude := range tt.wantExcludes {
				if strings.Contains(got, exclude) {
					t.Errorf("buildAbacScope() = %q, should NOT contain %q", got, exclude)
				}
			}
		})
	}
}
