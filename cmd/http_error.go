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
		return summarizeStructuredError(parsed)
	}

	return truncateErrorBody(raw)
}

func summarizeStructuredError(parsed map[string]any) string {
	parts := make([]string, 0, 6)
	seen := map[string]struct{}{}
	errorCode, _ := parsed["error"].(string)
	for _, key := range []string{"error", "message", "detail"} {
		if s, ok := parsed[key].(string); ok && strings.TrimSpace(s) != "" {
			val := strings.TrimSpace(s)
			if _, exists := seen[val]; exists {
				continue
			}
			seen[val] = struct{}{}
			parts = append(parts, val)
		}
	}

	if vals, ok := parsed["validation_errors"].([]any); ok {
		items := collectStringList(vals)
		if len(items) > 0 {
			deduped := uniqueStrings(items)
			label := "validation_errors=" + strings.Join(deduped, "; ")
			if _, exists := seen[label]; !exists {
				seen[label] = struct{}{}
				parts = append(parts, label)
			}
		}
	}

	if vals, ok := parsed["details"].([]any); ok {
		items := collectStringList(vals)
		if len(items) > 0 {
			deduped := uniqueStrings(items)
			label := "details=" + strings.Join(deduped, "; ")
			if _, exists := seen[label]; !exists {
				seen[label] = struct{}{}
				parts = append(parts, label)
			}
		}
	}

	if vals, ok := parsed["next_steps"].([]any); ok {
		items := collectStringList(vals)
		if len(items) > 0 {
			deduped := uniqueStrings(items)
			label := "next_steps=" + strings.Join(deduped, " | ")
			if _, exists := seen[label]; !exists {
				seen[label] = struct{}{}
				parts = append(parts, label)
			}
		}
	}

	if links, ok := parsed["links"].(map[string]any); ok {
		linkParts := make([]string, 0, len(links))
		for k, v := range links {
			s, vok := v.(string)
			if !vok || strings.TrimSpace(s) == "" {
				continue
			}
			linkParts = append(linkParts, fmt.Sprintf("%s=%s", strings.TrimSpace(k), strings.TrimSpace(s)))
		}
		if len(linkParts) > 0 {
			label := "links=" + strings.Join(linkParts, ", ")
			if _, exists := seen[label]; !exists {
				seen[label] = struct{}{}
				parts = append(parts, label)
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}

	if hint := errorHint(errorCode); hint != "" {
		parts = append(parts, "hint="+hint)
	}

	return truncateErrorBody(strings.Join(parts, " | "))
}

func errorHint(code string) string {
	switch strings.TrimSpace(code) {
	case "agent_required":
		return "use a claimed agent profile/token for create or upload operations"
	default:
		return ""
	}
}

func collectStringList(vals []any) []string {
	items := make([]string, 0, len(vals))
	for _, raw := range vals {
		s, ok := raw.(string)
		if !ok || strings.TrimSpace(s) == "" {
			continue
		}
		items = append(items, strings.TrimSpace(s))
	}
	return items
}

func uniqueStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, raw := range in {
		val := strings.TrimSpace(raw)
		if val == "" {
			continue
		}
		if _, exists := seen[val]; exists {
			continue
		}
		seen[val] = struct{}{}
		out = append(out, val)
	}
	return out
}

func truncateErrorBody(s string) string {
	if len(s) <= maxErrorBodyLen {
		return s
	}
	return strings.TrimSpace(s[:maxErrorBodyLen]) + "..."
}
