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
	"github.com/cafaye/cafaye-cli/internal/skills"
	workspacepkg "github.com/cafaye/cafaye-cli/internal/workspace"
	"github.com/spf13/cobra"
)

func newBooksCmd(rt *cli.Runtime) *cobra.Command {
	var profile string
	cmd := &cobra.Command{Use: "books", Short: "Book resources"}
	list := &cobra.Command{
		Use:   "list",
		Short: "List books visible to current context",
		Example: `  cafaye books list
  cafaye books list --context noel-agent-cafaye-com`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			client, err := clientForProfile(rt, cfg, profile)
			if err != nil {
				return err
			}
			resp, err := client.Do("GET", "/api/books", nil, "")
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return apiError("books list", resp.StatusCode, resp.Body)
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), payload)
		},
	}
	list.Flags().StringVar(&profile, "context", "", "Context to use (defaults to active)")
	cmd.AddCommand(list)
	cmd.AddCommand(newBooksCreateCmd(rt))
	cmd.AddCommand(newBooksUpdateCmd(rt))
	cmd.AddCommand(newBooksCoverCmd(rt))
	cmd.AddCommand(newBooksPricingCmd(rt))
	cmd.AddCommand(newBooksPublishCmd(rt))
	cmd.AddCommand(newBooksUnpublishCmd(rt))
	cmd.AddCommand(newBooksRevisionsCmd(rt))
	cmd.AddCommand(newBooksRevisionCmd(rt))
	return cmd
}

func newBooksCreateCmd(rt *cli.Runtime) *cobra.Command {
	var profile, title, subtitle, theme, booksDir, idem string
	var skipTemplates bool
	var everyoneAccess bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new book and scaffold local slug workspace",
		Example: `  cafaye books create --title "My Book"
  cafaye books create --title "Draft" --subtitle "Notes" --books-dir ~/Cafaye/books`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(title) == "" {
				return fmt.Errorf("missing --title\n  cafaye books create --title <title> [--subtitle <subtitle>]")
			}
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			client, err := clientForProfile(rt, cfg, profile)
			if err != nil {
				return err
			}
			if idem == "" {
				idem = fmt.Sprintf("run-%d", time.Now().UnixNano())
			}

			resp, err := client.Do("POST", "/api/books", map[string]any{
				"book": map[string]any{
					"title":           title,
					"subtitle":        subtitle,
					"theme":           theme,
					"everyone_access": everyoneAccess,
				},
			}, idem)
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return apiError("books create", resp.StatusCode, resp.Body)
			}

			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}
			book, ok := payload["book"].(map[string]any)
			if !ok {
				return fmt.Errorf("invalid books create response: missing book object")
			}
			slug, _ := book["slug"].(string)
			bookTitle, _ := book["title"].(string)
			bookSubtitle, _ := book["subtitle"].(string)
			author, _ := book["author"].(string)
			if strings.TrimSpace(slug) == "" {
				return fmt.Errorf("invalid books create response: missing book slug")
			}

			root, err := resolveWorkspaceRoot(booksDir)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(root, 0o755); err != nil {
				return err
			}
			var initRes workspacepkg.InitResult
			if skipTemplates {
				workspacePath := filepath.Join(root, slug)
				created := false
				if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
					created = true
				}
				if err := os.MkdirAll(workspacePath, 0o755); err != nil {
					return err
				}
				initRes = workspacepkg.InitResult{
					WorkspacePath: workspacePath,
					Created:       created,
					Populated:     false,
				}
			} else {
				initRes, err = workspacepkg.EnsureStarterWorkspaceForBook(root, workspacepkg.BookStarter{
					Slug:     slug,
					Title:    bookTitle,
					Subtitle: bookSubtitle,
					Author:   author,
				})
				if err != nil {
					return err
				}
			}
			skillRes, err := skills.InstallForRoot(initRes.WorkspacePath)
			if err != nil {
				return err
			}

			result := map[string]any{
				"book":              book,
				"workspace_root":    root,
				"workspace_path":    initRes.WorkspacePath,
				"workspace_created": initRes.Created,
				"templates_skipped": skipTemplates,
				"starter_populated": initRes.Populated,
				"skill_path":        skillRes.Path,
				"skill_updated":     skillRes.Updated,
				"next": map[string]any{
					"upload": fmt.Sprintf("cd %s && zip -r bundle.zip . && cafaye upload --file ./bundle.zip --idempotency-key run-%s-001", initRes.WorkspacePath, slug),
				},
			}
			return printJSON(cmd.OutOrStdout(), result)
		},
	}
	cmd.Flags().StringVar(&profile, "context", "", "Context to use (defaults to active)")
	cmd.Flags().StringVar(&title, "title", "", "Book title")
	cmd.Flags().StringVar(&subtitle, "subtitle", "", "Book subtitle")
	cmd.Flags().StringVar(&theme, "theme", "", "Book theme")
	cmd.Flags().BoolVar(&everyoneAccess, "everyone-access", false, "Whether everyone can access this book")
	cmd.Flags().BoolVar(&skipTemplates, "skip-templates", false, "Create workspace folder without starter template files")
	cmd.Flags().StringVar(&booksDir, "books-dir", "", "Workspace books directory (defaults to CAFAYE_BOOKS_DIR or ~/Cafaye/books)")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

func newBooksUpdateCmd(rt *cli.Runtime) *cobra.Command {
	var profile, title, subtitle, author, theme, idem string
	var bookID int
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update book metadata",
		Example: `  cafaye books update --book-id 42 --title "Updated Title"
  cafaye books update --book-id 42 --subtitle "New subtitle" --theme amber`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if bookID <= 0 {
				return fmt.Errorf("missing --book-id\n  cafaye books update --book-id <id> [flags]")
			}
			book := map[string]any{}
			if title != "" {
				book["title"] = title
			}
			if subtitle != "" {
				book["subtitle"] = subtitle
			}
			if author != "" {
				book["author"] = author
			}
			if theme != "" {
				book["theme"] = theme
			}
			if len(book) == 0 {
				return fmt.Errorf("no updates provided\n  cafaye books update --book-id <id> --title <title>")
			}
			return runBookWrite(rt, cmd, profile, idem, "PATCH", fmt.Sprintf("/api/books/%d", bookID), map[string]any{"book": book}, "books update")
		},
	}
	cmd.Flags().StringVar(&profile, "context", "", "Context to use (defaults to active)")
	cmd.Flags().IntVar(&bookID, "book-id", 0, "Book ID")
	cmd.Flags().StringVar(&title, "title", "", "Book title")
	cmd.Flags().StringVar(&subtitle, "subtitle", "", "Book subtitle")
	cmd.Flags().StringVar(&author, "author", "", "Book author")
	cmd.Flags().StringVar(&theme, "theme", "", "Book theme")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

func newBooksCoverCmd(rt *cli.Runtime) *cobra.Command {
	var profile, filePath, idem string
	var bookID int
	var remove bool

	cmd := &cobra.Command{
		Use:   "cover",
		Short: "Upload or remove a book cover",
		Example: `  cafaye books cover --book-id 42 --file ./cover.webp
  cafaye books cover --book-id 42 --remove`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if bookID <= 0 {
				return fmt.Errorf("missing --book-id\n  cafaye books cover --book-id <id> --file <path> | --remove")
			}
			if !remove && filePath == "" {
				return fmt.Errorf("missing --file or --remove\n  cafaye books cover --book-id <id> --file <path> | --remove")
			}
			if remove && filePath != "" {
				return fmt.Errorf("choose one of --file or --remove")
			}
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			p, err := rt.ActiveProfile(cfg, profile)
			if err != nil {
				return err
			}
			token, err := rt.Secrets.Get(p.TokenRef)
			if err != nil {
				return err
			}
			resp, err := uploadBookCover(p.BaseURL, token, bookID, filePath, remove, idem)
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return apiError("books cover", resp.StatusCode, resp.Body)
			}
			var payload map[string]any
			if err := json.Unmarshal(resp.Body, &payload); err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), payload)
		},
	}
	cmd.Flags().StringVar(&profile, "context", "", "Context to use (defaults to active)")
	cmd.Flags().IntVar(&bookID, "book-id", 0, "Book ID")
	cmd.Flags().StringVar(&filePath, "file", "", "Cover image path")
	cmd.Flags().BoolVar(&remove, "remove", false, "Remove current cover")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

func newBooksRevisionsCmd(rt *cli.Runtime) *cobra.Command {
	var profile string
	var bookID int
	cmd := &cobra.Command{
		Use:   "revisions",
		Short: "List book revisions",
		Example: `  cafaye books revisions --book-id 42
  cafaye books revisions --book-id 42 --context noel-agent-cafaye-com`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if bookID <= 0 {
				return fmt.Errorf("missing --book-id\n  cafaye books revisions --book-id <id>")
			}
			return runBookRead(rt, cmd, profile, fmt.Sprintf("/api/books/%d/revisions", bookID), "books revisions")
		},
	}
	cmd.Flags().StringVar(&profile, "context", "", "Context to use (defaults to active)")
	cmd.Flags().IntVar(&bookID, "book-id", 0, "Book ID")
	return cmd
}

func newBooksRevisionCmd(rt *cli.Runtime) *cobra.Command {
	var profile string
	var bookID, revisionID int
	cmd := &cobra.Command{
		Use:     "revision",
		Short:   "Show a single revision",
		Example: `  cafaye books revision --book-id 42 --revision-id 7`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if bookID <= 0 || revisionID <= 0 {
				return fmt.Errorf("missing required flags\n  cafaye books revision --book-id <id> --revision-id <id>")
			}
			return runBookRead(rt, cmd, profile, fmt.Sprintf("/api/books/%d/revisions/%d", bookID, revisionID), "books revision")
		},
	}
	cmd.Flags().StringVar(&profile, "context", "", "Context to use (defaults to active)")
	cmd.Flags().IntVar(&bookID, "book-id", 0, "Book ID")
	cmd.Flags().IntVar(&revisionID, "revision-id", 0, "Revision ID")
	return cmd
}

func newBooksPricingCmd(rt *cli.Runtime) *cobra.Command {
	var profile, pricingType, currency, idem string
	var bookID, priceCents int

	cmd := &cobra.Command{
		Use:   "pricing",
		Short: "Set pricing for a book",
		Example: `  cafaye books pricing --book-id 42 --pricing-type free
  cafaye books pricing --book-id 42 --pricing-type paid --price-cents 1200 --price-currency USD`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if bookID <= 0 {
				return fmt.Errorf("missing --book-id\n  cafaye books pricing --book-id <id> --pricing-type <free|paid>")
			}
			if pricingType == "" {
				return fmt.Errorf("missing --pricing-type\n  cafaye books pricing --book-id <id> --pricing-type <free|paid>")
			}

			body := map[string]any{
				"book": map[string]any{
					"pricing_type": pricingType,
				},
			}
			if priceCents > 0 {
				body["book"].(map[string]any)["price_cents"] = priceCents
			}
			if currency != "" {
				body["book"].(map[string]any)["price_currency"] = currency
			}
			return runBookWrite(rt, cmd, profile, idem, "PATCH", fmt.Sprintf("/api/books/%d/pricing", bookID), body, "books pricing")
		},
	}
	cmd.Flags().StringVar(&profile, "context", "", "Context to use (defaults to active)")
	cmd.Flags().IntVar(&bookID, "book-id", 0, "Book ID")
	cmd.Flags().StringVar(&pricingType, "pricing-type", "", "Pricing type (free or paid)")
	cmd.Flags().IntVar(&priceCents, "price-cents", 0, "Price in cents")
	cmd.Flags().StringVar(&currency, "price-currency", "", "Price currency code (e.g. USD)")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

func newBooksPublishCmd(rt *cli.Runtime) *cobra.Command {
	var profile, idem string
	var bookID, revisionID int

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish a specific revision",
		Example: `  cafaye books publish --book-id 42 --revision-id 7
  cafaye books publish --book-id 42 --revision-id 7 --idempotency-key run-publish-42-7`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if bookID <= 0 || revisionID <= 0 {
				return fmt.Errorf("missing required flags\n  cafaye books publish --book-id <id> --revision-id <id>")
			}
			return runBookWrite(rt, cmd, profile, idem, "POST", fmt.Sprintf("/api/books/%d/publish", bookID), map[string]any{"revision_id": revisionID}, "books publish")
		},
	}
	cmd.Flags().StringVar(&profile, "context", "", "Context to use (defaults to active)")
	cmd.Flags().IntVar(&bookID, "book-id", 0, "Book ID")
	cmd.Flags().IntVar(&revisionID, "revision-id", 0, "Revision ID to publish")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

func newBooksUnpublishCmd(rt *cli.Runtime) *cobra.Command {
	var profile, idem string
	var bookID int

	cmd := &cobra.Command{
		Use:   "unpublish",
		Short: "Unpublish a book",
		Example: `  cafaye books unpublish --book-id 42
  cafaye books unpublish --book-id 42 --idempotency-key run-unpublish-42`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if bookID <= 0 {
				return fmt.Errorf("missing --book-id\n  cafaye books unpublish --book-id <id>")
			}
			return runBookWrite(rt, cmd, profile, idem, "POST", fmt.Sprintf("/api/books/%d/unpublish", bookID), map[string]any{}, "books unpublish")
		},
	}
	cmd.Flags().StringVar(&profile, "context", "", "Context to use (defaults to active)")
	cmd.Flags().IntVar(&bookID, "book-id", 0, "Book ID")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

func runBookRead(rt *cli.Runtime, cmd *cobra.Command, profile string, path string, op string) error {
	cfg, err := rt.LoadConfig()
	if err != nil {
		return err
	}
	client, err := clientForProfile(rt, cfg, profile)
	if err != nil {
		return err
	}
	resp, err := client.Do("GET", path, nil, "")
	if err != nil {
		return err
	}
	cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
	if resp.StatusCode >= 300 {
		return apiError(op, resp.StatusCode, resp.Body)
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return err
	}
	return printJSON(cmd.OutOrStdout(), payload)
}

func runBookWrite(rt *cli.Runtime, cmd *cobra.Command, profile string, idem string, method string, path string, body map[string]any, op string) error {
	cfg, err := rt.LoadConfig()
	if err != nil {
		return err
	}
	client, err := clientForProfile(rt, cfg, profile)
	if err != nil {
		return err
	}
	if idem == "" {
		idem = fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	resp, err := client.Do(method, path, body, idem)
	if err != nil {
		return err
	}
	cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
	if resp.StatusCode >= 300 {
		return apiError(op, resp.StatusCode, resp.Body)
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return err
	}
	return printJSON(cmd.OutOrStdout(), payload)
}

func uploadBookCover(baseURL string, token string, bookID int, filePath string, remove bool, idem string) (api.Response, error) {
	if idem == "" {
		idem = fmt.Sprintf("run-books-cover-%d", time.Now().UnixNano())
	}

	bodyReader, bodyWriter := io.Pipe()
	mw := multipart.NewWriter(bodyWriter)
	go func() {
		defer bodyWriter.Close()
		defer mw.Close()

		if remove {
			_ = mw.WriteField("remove_cover", "true")
			return
		}

		f, err := os.Open(filepath.Clean(filePath))
		if err != nil {
			_ = bodyWriter.CloseWithError(err)
			return
		}
		defer f.Close()

		fw, err := mw.CreateFormFile("cover", filepath.Base(filePath))
		if err != nil {
			_ = bodyWriter.CloseWithError(err)
			return
		}
		if _, err := io.Copy(fw, f); err != nil {
			_ = bodyWriter.CloseWithError(err)
			return
		}
	}()

	req, err := http.NewRequest(http.MethodPut, strings.TrimRight(baseURL, "/")+fmt.Sprintf("/api/books/%d/cover", bookID), bodyReader)
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
