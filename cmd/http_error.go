package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
)

const maxErrorBodyLen = 240

func apiError(op string, status int, body []byte) error {
	msg := summarizeErrorBody(body)
	if msg == "" {
		return fmt.Errorf("%s failed: status=%d", op, status)
	}
	return fmt.Errorf("%s failed: status=%d body=%s", op, status, msg)
}

func summarizeErrorBody(body []byte) string {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return ""
	}

	if strings.HasPrefix(raw, "<!DOCTYPE html") || strings.HasPrefix(raw, "<html") {
		return "<html error response omitted>"
	}

	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err == nil {
		for _, key := range []string{"error", "message", "detail"} {
			if s, ok := parsed[key].(string); ok && strings.TrimSpace(s) != "" {
				return truncateErrorBody(strings.TrimSpace(s))
			}
		}
	}

	return truncateErrorBody(raw)
}

func truncateErrorBody(s string) string {
	if len(s) <= maxErrorBodyLen {
		return s
	}
	return strings.TrimSpace(s[:maxErrorBodyLen]) + "..."
}
