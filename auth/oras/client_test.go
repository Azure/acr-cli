// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package oras

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"oras.land/oras-go/v2/registry/remote/auth"
)

func TestAuthClient(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected access: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// writes uuid of the request in the response header
		uuid := r.Header.Get("x-ms-correlation-request-id")
		if uuid != "" {
			w.Header().Set("x-ms-correlation-request-id", uuid)
		}
	}))
	defer testServer.Close()

	testClient := NewClient(ClientOptions{
		Credential: Credential("testUser", "testPassword"),
	})
	req, _ := http.NewRequest(http.MethodGet, testServer.URL, nil)
	resp, err := testClient.Do(req)
	if err != nil {
		t.Fatalf("invalid response: %v", err)
	}

	// get the uuid of testClient and compare with
	// the uuid of the response header
	clientUUID := testClient.(*auth.Client).Header.Get("x-ms-correlation-request-id")
	respUUID := resp.Header.Get("x-ms-correlation-request-id")
	if clientUUID == "" || respUUID == "" || clientUUID != respUUID {
		t.Fatalf("invalid uuid: %v", err)
	}
}
