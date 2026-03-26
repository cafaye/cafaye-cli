package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type DeprecationNotice struct {
	Deprecated  bool   `json:"deprecated"`
	Message     string `json:"message"`
	Replacement string `json:"replacement"`
	Sunset      string `json:"sunset"`
	DocsURL     string `json:"docs_url"`
}

type Response struct {
	StatusCode  int
	Body        []byte
	Deprecation DeprecationNotice
}

type apiDeprecationBody struct {
	Deprecation *DeprecationNotice `json:"deprecation"`
}

func (c *Client) Do(method string, path string, payload any, idempotencyKey string) (Response, error) {
	if c.BaseURL == "" {
		return Response{}, fmt.Errorf("base url is required")
	}
	if c.Token == "" {
		return Response{}, fmt.Errorf("api token is required")
	}

	return c.doRequest(method, path, payload, idempotencyKey, true)
}

func (c *Client) DoPublic(method string, path string, payload any, idempotencyKey string) (Response, error) {
	if c.BaseURL == "" {
		return Response{}, fmt.Errorf("base url is required")
	}

	return c.doRequest(method, path, payload, idempotencyKey, false)
}

func (c *Client) doRequest(method string, path string, payload any, idempotencyKey string, includeAuth bool) (Response, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return Response{}, err
		}
		body = bytes.NewReader(data)
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequest(method, strings.TrimRight(c.BaseURL, "/")+path, body)
	if err != nil {
		return Response{}, err
	}
	if includeAuth {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}

	result := Response{StatusCode: resp.StatusCode, Body: respBody}
	result.Deprecation = parseDeprecation(resp, respBody)
	return result, nil
}

func parseDeprecation(resp *http.Response, body []byte) DeprecationNotice {
	n := DeprecationNotice{}
	if strings.EqualFold(resp.Header.Get("X-Cafaye-Deprecated"), "true") {
		n.Deprecated = true
	}
	n.Message = resp.Header.Get("X-Cafaye-Deprecation-Message")
	n.Replacement = resp.Header.Get("X-Cafaye-Replacement")
	n.Sunset = resp.Header.Get("X-Cafaye-Sunset")
	n.DocsURL = resp.Header.Get("X-Cafaye-Docs")

	var parsed apiDeprecationBody
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Deprecation != nil {
		if parsed.Deprecation.Deprecated {
			n.Deprecated = true
		}
		if parsed.Deprecation.Message != "" {
			n.Message = parsed.Deprecation.Message
		}
		if parsed.Deprecation.Replacement != "" {
			n.Replacement = parsed.Deprecation.Replacement
		}
		if parsed.Deprecation.Sunset != "" {
			n.Sunset = parsed.Deprecation.Sunset
		}
		if parsed.Deprecation.DocsURL != "" {
			n.DocsURL = parsed.Deprecation.DocsURL
		}
	}

	if n.Message != "" || n.Replacement != "" || n.Sunset != "" || n.DocsURL != "" {
		n.Deprecated = true
	}
	return n
}
