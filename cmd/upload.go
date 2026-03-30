package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cafaye/cafaye-cli/internal/api"
	"github.com/cafaye/cafaye-cli/internal/cli"
	"github.com/spf13/cobra"
)

func newUploadCmd(rt *cli.Runtime) *cobra.Command {
	var agent, baseURL, filePath, idem string
	var publish, dryRun, fromStdin bool

	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload a source bundle",
		Example: `  cafaye upload --agent noel-agent --file ./the-cafaye-manual.zip --idempotency-key run-123
  cafaye upload --agent noel-agent --file ./the-cafaye-manual.zip --publish --idempotency-key run-456
  cat ./the-cafaye-manual.zip | cafaye upload --agent noel-agent --stdin --publish --idempotency-key run-789`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if idem == "" {
				return fmt.Errorf("missing --idempotency-key\n  cafaye upload --file <bundle.zip> --idempotency-key <key>")
			}
			if !strings.HasPrefix(idem, "run-") && len(idem) < 8 {
				return fmt.Errorf("idempotency key should be stable and descriptive")
			}
			if fromStdin {
				tmp, err := os.CreateTemp("", "cafaye-upload-*.zip")
				if err != nil {
					return err
				}
				defer os.Remove(tmp.Name())
				if _, err := io.Copy(tmp, cmd.InOrStdin()); err != nil {
					_ = tmp.Close()
					return err
				}
				if err := tmp.Close(); err != nil {
					return err
				}
				filePath = tmp.Name()
			}
			if dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "dry_run: true")
				fmt.Fprintf(cmd.OutOrStdout(), "would_upload: %s\n", filePath)
				fmt.Fprintf(cmd.OutOrStdout(), "publish: %t\n", publish)
				return nil
			}
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			currSession, err := resolveAgentSession(cfg, agent, baseURL)
			if err != nil {
				return err
			}
			token, err := rt.Secrets.Get(currSession.TokenRef)
			if err != nil {
				return err
			}
			resp, err := uploadFile(currSession.BaseURL, token, filePath, publish, idem)
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return apiError("upload", resp.StatusCode, resp.Body)
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), payload)
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	cmd.Flags().StringVar(&filePath, "file", "", "Path to source bundle zip")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key for retry-safe uploads")
	cmd.Flags().BoolVar(&publish, "publish", false, "Publish after successful upload")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show plan without making changes")
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "Read source bundle from stdin")
	cmd.AddCommand(newUploadShowCmd(rt))
	return cmd
}

func newUploadShowCmd(rt *cli.Runtime) *cobra.Command {
	var agent string
	var baseURL string
	var uploadID int

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show upload status/details",
		Example: `  cafaye upload show --id 123
  cafaye upload show --id 123 --agent noel-agent`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if uploadID <= 0 {
				return fmt.Errorf("missing --id\n  cafaye upload show --id <upload-id>")
			}
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			currSession, err := resolveAgentSession(cfg, agent, baseURL)
			if err != nil {
				return err
			}
			client, err := clientForAgentSession(rt, cfg, currSession.Name)
			if err != nil {
				return err
			}
			resp, err := client.Do("GET", fmt.Sprintf("/api/uploads/%d", uploadID), nil, "")
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return apiError("upload show", resp.StatusCode, resp.Body)
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), payload)
		},
	}
	cmd.Flags().IntVar(&uploadID, "id", 0, "Upload ID")
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	return cmd
}

func uploadFile(baseURL string, token string, filePath string, publish bool, idem string) (api.Response, error) {
	if filePath == "" {
		return api.Response{}, fmt.Errorf("missing --file\n  cafaye upload --file <bundle.zip> --idempotency-key <key>")
	}
	f, err := os.Open(filepath.Clean(filePath))
	if err != nil {
		return api.Response{}, err
	}
	defer f.Close()

	bodyReader, bodyWriter := io.Pipe()
	mw := multipart.NewWriter(bodyWriter)
	go func() {
		defer bodyWriter.Close()
		defer mw.Close()
		fw, err := mw.CreateFormFile("source_bundle", filepath.Base(filePath))
		if err != nil {
			_ = bodyWriter.CloseWithError(err)
			return
		}
		if _, err := io.Copy(fw, f); err != nil {
			_ = bodyWriter.CloseWithError(err)
			return
		}
		_ = mw.WriteField("publish", fmt.Sprintf("%t", publish))
	}()

	req, err := http.NewRequest("POST", strings.TrimRight(baseURL, "/")+"/api/uploads", bodyReader)
	if err != nil {
		return api.Response{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", idem)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	h := &http.Client{Timeout: 2 * time.Minute}
	resp, err := h.Do(req)
	if err != nil {
		return api.Response{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return api.Response{}, err
	}
	return api.Response{StatusCode: resp.StatusCode, Body: body, Deprecation: api.DeprecationNotice{
		Deprecated:  strings.EqualFold(resp.Header.Get("X-Cafaye-Deprecated"), "true"),
		Message:     resp.Header.Get("X-Cafaye-Deprecation-Message"),
		Replacement: resp.Header.Get("X-Cafaye-Replacement"),
		Sunset:      resp.Header.Get("X-Cafaye-Sunset"),
		DocsURL:     resp.Header.Get("X-Cafaye-Docs"),
	}}, nil
}
