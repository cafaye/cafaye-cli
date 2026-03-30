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

func TestSummarizeErrorBodyDedupesRepeatedMessages(t *testing.T) {
	body := []byte(`{
		"error":"validation_failed",
		"detail":"same message",
		"validation_errors":["same message","same message"]
	}`)

	got := summarizeErrorBody(body)
	if strings.Count(got, "same message") != 2 {
		// One in detail and one in labeled validation_errors is acceptable,
		// but duplicates inside validation_errors should be collapsed.
		t.Fatalf("expected deduped repeated values, got: %s", got)
	}
	if strings.Contains(got, "same message; same message") {
		t.Fatalf("expected duplicate validation_errors to collapse, got: %s", got)
	}
}

func TestSummarizeErrorBodyIncludesAgentHint(t *testing.T) {
	body := []byte(`{"error":"agent_required","detail":"agent principal is required"}`)

	got := summarizeErrorBody(body)
	if !strings.Contains(got, "hint=use a claimed agent session token") {
		t.Fatalf("expected agent hint in summary, got: %s", got)
	}
}

func TestSummarizeErrorBodyIncludesDetailsList(t *testing.T) {
	body := []byte(`{"error":"invalid_agent","details":["Username has already been taken"]}`)

	got := summarizeErrorBody(body)
	if !strings.Contains(got, "invalid_agent") {
		t.Fatalf("expected error code, got: %s", got)
	}
	if !strings.Contains(got, "details=Username has already been taken") {
		t.Fatalf("expected details in summary, got: %s", got)
	}
}
