package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRandomAPIKey(t *testing.T) {
	key, err := randomAPIKey()
	if err != nil {
		t.Fatalf("randomAPIKey returned an error: %v", err)
	}
	if !strings.HasPrefix(key, "tm_") {
		t.Fatalf("expected tm_ prefix, got %q", key)
	}
	if len(key) != 51 {
		t.Fatalf("expected 51-character key, got %d", len(key))
	}
}

func TestHashIsDeterministicAndNotPlaintext(t *testing.T) {
	first := hash("secret")
	second := hash("secret")
	if first != second {
		t.Fatal("hash should be deterministic")
	}
	if first == "secret" || len(first) != 64 {
		t.Fatalf("unexpected SHA-256 digest: %q", first)
	}
}

func TestWriteJSON(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeJSON(recorder, 201, map[string]string{"status": "created"})
	if recorder.Code != 201 {
		t.Fatalf("expected status 201, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"status":"created"`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}
