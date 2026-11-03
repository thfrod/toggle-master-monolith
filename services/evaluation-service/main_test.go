package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoJSONDecodesSuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"matches": true})
	}))
	defer server.Close()

	var result targetResult
	err := doJSON(server.Client(), http.MethodPost, server.URL, map[string]string{"subject_id": "user-1"}, &result)
	if err != nil {
		t.Fatalf("doJSON returned an error: %v", err)
	}
	if !result.Matches {
		t.Fatal("expected matches=true")
	}
}

func TestDoJSONReturnsErrorForUpstreamFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	var result targetResult
	if err := doJSON(server.Client(), http.MethodGet, server.URL, nil, &result); err == nil {
		t.Fatal("expected upstream error")
	}
}

func TestEnvUsesFallbackWhenUnset(t *testing.T) {
	t.Setenv("TOGGLEMASTER_TEST_ENV", "")
	if got := env("TOGGLEMASTER_TEST_ENV", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
}
