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

func newBooksCmd(rt *cli.Runtime) *cobra.Command {
	var profile string
	cmd := &cobra.Command{Use: "books", Short: "Book resources"}
	list := &cobra.Command{
		Use:   "list",
		Short: "List books visible to current profile",
		Example: `  cafaye books list
  cafaye books list --profile noel-agent-write`,
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
	list.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
	cmd.AddCommand(list)
	cmd.AddCommand(newBooksCreateCmd(rt))
	cmd.AddCommand(newBooksUpdateCmd(rt))
	cmd.AddCommand(newBooksCoverCmd(rt))
	cmd.AddCommand(newBooksPricingCmd(rt))
	cmd.AddCommand(newBooksPublishCmd(rt))
	cmd.AddCommand(newBooksUnpublishCmd(rt))
	cmd.AddCommand(newBooksRevisionsCmd(rt))
	cmd.AddCommand(newBooksRevisionCmd(rt))
	cmd.AddCommand(newBooksSourceCmd(rt))
	cmd.AddCommand(newBooksRevisionSourceCmd(rt))
	return cmd
}

func newBooksCreateCmd(rt *cli.Runtime) *cobra.Command {
	var profile, title, subtitle, author, theme, idem string
	var everyoneAccess bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new book record",
		Example: `  cafaye books create --profile noel-agent-write --title "My Book" --author "Kaka"
  cafaye books create --profile noel-agent-write --title "Draft" --author "Kaka" --everyone-access=false`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if title == "" {
				return fmt.Errorf("missing --title\n  cafaye books create --profile <agent-profile> --title <title> --author <author>")
			}
			body := map[string]any{
				"book": map[string]any{
					"title":           title,
					"subtitle":        subtitle,
					"author":          author,
					"theme":           theme,
					"everyone_access": everyoneAccess,
				},
			}
			return runBookWrite(rt, cmd, profile, idem, "POST", "/api/books", body, "books create")
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
	cmd.Flags().StringVar(&title, "title", "", "Book title")
	cmd.Flags().StringVar(&subtitle, "subtitle", "", "Book subtitle")
	cmd.Flags().StringVar(&author, "author", "", "Book author")
	cmd.Flags().StringVar(&theme, "theme", "", "Book theme")
	cmd.Flags().BoolVar(&everyoneAccess, "everyone-access", false, "Whether everyone can access this book")
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
	cmd.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
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
	cmd.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
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
  cafaye books revisions --book-id 42 --profile noel-agent-write`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if bookID <= 0 {
				return fmt.Errorf("missing --book-id\n  cafaye books revisions --book-id <id>")
			}
			return runBookRead(rt, cmd, profile, fmt.Sprintf("/api/books/%d/revisions", bookID), "books revisions")
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
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
	cmd.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
	cmd.Flags().IntVar(&bookID, "book-id", 0, "Book ID")
	cmd.Flags().IntVar(&revisionID, "revision-id", 0, "Revision ID")
	return cmd
}

func newBooksSourceCmd(rt *cli.Runtime) *cobra.Command {
	var profile string
	var bookID int
	cmd := &cobra.Command{
		Use:     "source",
		Short:   "Show source download metadata for current book source",
		Example: `  cafaye books source --book-id 42`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if bookID <= 0 {
				return fmt.Errorf("missing --book-id\n  cafaye books source --book-id <id>")
			}
			return runBookRead(rt, cmd, profile, fmt.Sprintf("/api/books/%d/source", bookID), "books source")
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
	cmd.Flags().IntVar(&bookID, "book-id", 0, "Book ID")
	return cmd
}

func newBooksRevisionSourceCmd(rt *cli.Runtime) *cobra.Command {
	var profile string
	var bookID, revisionID int
	cmd := &cobra.Command{
		Use:     "revision-source",
		Short:   "Show source download metadata for a specific revision",
		Example: `  cafaye books revision-source --book-id 42 --revision-id 7`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if bookID <= 0 || revisionID <= 0 {
				return fmt.Errorf("missing required flags\n  cafaye books revision-source --book-id <id> --revision-id <id>")
			}
			return runBookRead(rt, cmd, profile, fmt.Sprintf("/api/books/%d/revisions/%d/source", bookID, revisionID), "books revision-source")
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
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
	cmd.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
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
	cmd.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
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
	cmd.Flags().StringVar(&profile, "profile", "", "Profile to use (defaults to active)")
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
