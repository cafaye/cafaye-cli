package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientDoSetsAuthAndParsesDeprecationHeaders(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Fatalf("bad auth header: %s", got)
		}
		if got := r.Header.Get("Idempotency-Key"); got != "run-1" {
			t.Fatalf("missing idempotency key: %s", got)
		}
		w.Header().Set("X-Cafaye-Deprecated", "true")
		w.Header().Set("X-Cafaye-Replacement", "/api/new")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer s.Close()

	c := &Client{BaseURL: s.URL, Token: "tok"}
	resp, err := c.Do(http.MethodGet, "/x", nil, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Deprecation.Deprecated {
		t.Fatal("expected deprecated response")
	}
	if resp.Deprecation.Replacement != "/api/new" {
		t.Fatalf("unexpected replacement: %s", resp.Deprecation.Replacement)
	}
}

func TestClientDoParsesDeprecationBody(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"deprecation":{"message":"old route","replacement":"/api/new"}}`))
	}))
	defer s.Close()

	c := &Client{BaseURL: s.URL, Token: "tok"}
	resp, err := c.Do(http.MethodGet, "/x", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Deprecation.Deprecated || !strings.Contains(resp.Deprecation.Message, "old route") {
		t.Fatalf("expected deprecation from body, got: %+v", resp.Deprecation)
	}
}
