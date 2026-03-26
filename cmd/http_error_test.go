package cmd

import (
	"strings"
	"testing"
)

func TestSummarizeErrorBodyStructuredPayload(t *testing.T) {
	body := []byte(`{
		"error":"upload_failed",
		"detail":"book.yml missing required fields",
		"validation_errors":["book.yml missing required fields: schema_version"],
		"next_steps":["Add required keys to book.yml"],
		"links":{"billing":"/home/billing"}
	}`)

	got := summarizeErrorBody(body)
	if got == "" {
		t.Fatal("expected non-empty summary")
	}
	for _, want := range []string{
		"upload_failed",
		"book.yml missing required fields",
		"validation_errors=",
		"next_steps=",
		"links=billing=/home/billing",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected summary to contain %q, got: %s", want, got)
		}
	}
}

func TestSummarizeErrorBodyHtml(t *testing.T) {
	got := summarizeErrorBody([]byte("<html><body>boom</body></html>"))
	if got != "<html error response omitted>" {
		t.Fatalf("expected html omission message, got: %s", got)
	}
}
