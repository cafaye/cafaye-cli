package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
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
	var agent string
	var baseURL string
	cmd := &cobra.Command{Use: "books", Short: "Create, update, upload, and publish books"}
	cmd.AddGroup(
		&cobra.Group{ID: "read", Title: "Read Commands"},
		&cobra.Group{ID: "write", Title: "Write Commands"},
		&cobra.Group{ID: "publish", Title: "Publish Commands"},
		&cobra.Group{ID: "upload", Title: "Upload Commands"},
	)
	list := &cobra.Command{
		Use:   "list",
		Short: "List books visible to current agent session",
		Example: `  cafaye books list
  cafaye books list --agent noel-agent`,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
	list.GroupID = "read"
	list.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	list.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	cmd.AddCommand(list)
	create := newBooksCreateCmd(rt)
	create.GroupID = "write"
	update := newBooksUpdateCmd(rt)
	update.GroupID = "write"
	cover := newBooksCoverCmd(rt)
	cover.GroupID = "write"
	pricing := newBooksPricingCmd(rt)
	pricing.GroupID = "publish"
	publish := newBooksPublishCmd(rt)
	publish.GroupID = "publish"
	unpublish := newBooksUnpublishCmd(rt)
	unpublish.GroupID = "publish"
	archive := newBooksArchiveCmd(rt)
	archive.GroupID = "write"
	unarchive := newBooksUnarchiveCmd(rt)
	unarchive.GroupID = "write"
	revisions := newBooksRevisionsCmd(rt)
	revisions.GroupID = "read"
	revision := newBooksRevisionCmd(rt)
	revision.GroupID = "read"
	upload := newUploadCmd(rt)
	upload.GroupID = "upload"
	cmd.AddCommand(create)
	cmd.AddCommand(update)
	cmd.AddCommand(cover)
	cmd.AddCommand(pricing)
	cmd.AddCommand(publish)
	cmd.AddCommand(unpublish)
	cmd.AddCommand(archive)
	cmd.AddCommand(unarchive)
	cmd.AddCommand(revisions)
	cmd.AddCommand(revision)
	cmd.AddCommand(upload)
	return cmd
}

func newBooksCreateCmd(rt *cli.Runtime) *cobra.Command {
	var agent, baseURL, title, subtitle, blurb, synopsis, theme, booksDir, idem string
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
			currSession, err := resolveAgentSession(cfg, agent, baseURL)
			if err != nil {
				return err
			}
			client, err := clientForAgentSession(rt, cfg, currSession.Name)
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
					"blurb":           blurb,
					"synopsis":        synopsis,
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
					"upload": fmt.Sprintf("cd %s && zip -r bundle.zip . && cafaye books upload --file ./bundle.zip --idempotency-key run-%s-001", initRes.WorkspacePath, slug),
				},
			}
			return printJSON(cmd.OutOrStdout(), result)
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	cmd.Flags().StringVar(&title, "title", "", "Book title")
	cmd.Flags().StringVar(&subtitle, "subtitle", "", "Book subtitle")
	cmd.Flags().StringVar(&blurb, "blurb", "", "Book blurb (short pitch)")
	cmd.Flags().StringVar(&synopsis, "synopsis", "", "Book synopsis (long summary)")
	cmd.Flags().StringVar(&theme, "theme", "", "Book theme")
	cmd.Flags().BoolVar(&everyoneAccess, "everyone-access", false, "Whether everyone can access this book")
	cmd.Flags().BoolVar(&skipTemplates, "skip-templates", false, "Create workspace folder without starter template files")
	cmd.Flags().StringVar(&booksDir, "books-dir", "", "Workspace books directory (defaults to CAFAYE_BOOKS_DIR or ~/Cafaye/books)")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

func newBooksUpdateCmd(rt *cli.Runtime) *cobra.Command {
	var agent, baseURL, title, subtitle, blurb, synopsis, author, theme, languageCode, tagsCSV, primaryTag, idem string
	var categoryID int
	var bookSlug, bookRef string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update book metadata",
		Example: `  cafaye books update --book-slug the-cafaye-manual --title "Updated Title"
  cafaye books update --book-slug the-cafaye-manual --subtitle "New subtitle" --blurb "Short pitch" --synopsis "Long summary" --theme amber
  cafaye books update --book-slug the-cafaye-manual --language-code en --category-id 2
  cafaye books update --book-slug the-cafaye-manual --tags "cafaye manual,publishing" --primary-tag "cafaye manual"`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bookIdentifier, err := resolveBookIdentifier(bookSlug, bookRef)
			if err != nil {
				return err
			}
			book := map[string]any{}
			tagsBook := map[string]any{}
			if title != "" {
				book["title"] = title
			}
			if subtitle != "" {
				book["subtitle"] = subtitle
			}
			if blurb != "" {
				book["blurb"] = blurb
			}
			if synopsis != "" {
				book["synopsis"] = synopsis
			}
			if author != "" {
				book["author"] = author
			}
			if theme != "" {
				book["theme"] = theme
			}
			if strings.TrimSpace(languageCode) != "" {
				book["language_code"] = strings.TrimSpace(languageCode)
			}
			if categoryID > 0 {
				book["category_id"] = categoryID
			}
			if strings.TrimSpace(tagsCSV) != "" {
				parts := strings.Split(tagsCSV, ",")
				tagNames := make([]string, 0, len(parts))
				for _, part := range parts {
					trimmed := strings.TrimSpace(part)
					if trimmed != "" {
						tagNames = append(tagNames, trimmed)
					}
				}
				if len(tagNames) == 0 {
					return fmt.Errorf("no valid tags parsed from --tags")
				}
				tagsBook["tag_names"] = tagNames
			}
			if strings.TrimSpace(primaryTag) != "" {
				tagsBook["primary_tag"] = strings.TrimSpace(primaryTag)
			}
			if len(book) == 0 && len(tagsBook) == 0 {
				return fmt.Errorf("no updates provided\n  cafaye books update --book-slug <slug> --title <title> | --tags \"tag1,tag2\"")
			}

			escapedSlug := url.PathEscape(bookIdentifier)

			if len(book) > 0 {
				if err := runBookWrite(rt, cmd, agent, baseURL, deriveScopedIdempotencyKey(idem, "book"), "PATCH", fmt.Sprintf("/api/books/%s", escapedSlug), map[string]any{"book": book}, "books update"); err != nil {
					return err
				}
			}

			if len(tagsBook) > 0 {
				return runBookWrite(rt, cmd, agent, baseURL, deriveScopedIdempotencyKey(idem, "tags"), "PATCH", fmt.Sprintf("/api/books/%s/tags", escapedSlug), map[string]any{"book": tagsBook}, "books update tags")
			}

			return nil
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	cmd.Flags().StringVar(&bookSlug, "book-slug", "", "Book slug")
	cmd.Flags().StringVar(&bookRef, "book-ref", "", "Book reference ID (book_...)")
	cmd.Flags().StringVar(&title, "title", "", "Book title")
	cmd.Flags().StringVar(&subtitle, "subtitle", "", "Book subtitle")
	cmd.Flags().StringVar(&blurb, "blurb", "", "Book blurb (short pitch)")
	cmd.Flags().StringVar(&synopsis, "synopsis", "", "Book synopsis (long summary)")
	cmd.Flags().StringVar(&author, "author", "", "Book author")
	cmd.Flags().StringVar(&theme, "theme", "", "Book theme")
	cmd.Flags().StringVar(&languageCode, "language-code", "", "Language code (e.g. en, sw)")
	cmd.Flags().IntVar(&categoryID, "category-id", 0, "Category ID")
	cmd.Flags().StringVar(&tagsCSV, "tags", "", "Comma-separated tags")
	cmd.Flags().StringVar(&primaryTag, "primary-tag", "", "Primary tag (must match an updated or existing tag)")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

func deriveScopedIdempotencyKey(base string, suffix string) string {
	trimmedBase := strings.TrimSpace(base)
	trimmedSuffix := strings.TrimSpace(suffix)
	if trimmedBase == "" || trimmedSuffix == "" {
		return trimmedBase
	}
	return fmt.Sprintf("%s-%s", trimmedBase, trimmedSuffix)
}

func newBooksCoverCmd(rt *cli.Runtime) *cobra.Command {
	var agent, baseURL, filePath, idem string
	var bookSlug, bookRef string
	var remove bool

	cmd := &cobra.Command{
		Use:   "cover",
		Short: "Upload or remove a book cover",
		Example: `  cafaye books cover --book-slug the-cafaye-manual --file ./cover.webp
  cafaye books cover --book-slug the-cafaye-manual --remove`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bookIdentifier, err := resolveBookIdentifier(bookSlug, bookRef)
			if err != nil {
				return err
			}
			if !remove && filePath == "" {
				return fmt.Errorf("missing --file or --remove\n  cafaye books cover --book-slug <slug> --file <path> | --remove")
			}
			if remove && filePath != "" {
				return fmt.Errorf("choose one of --file or --remove")
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
			resp, err := uploadBookCover(currSession.BaseURL, token, bookIdentifier, filePath, remove, idem)
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
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	cmd.Flags().StringVar(&bookSlug, "book-slug", "", "Book slug")
	cmd.Flags().StringVar(&bookRef, "book-ref", "", "Book reference ID (book_...)")
	cmd.Flags().StringVar(&filePath, "file", "", "Cover image path")
	cmd.Flags().BoolVar(&remove, "remove", false, "Remove current cover")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

func newBooksRevisionsCmd(rt *cli.Runtime) *cobra.Command {
	var agent, baseURL string
	var bookSlug, bookRef string
	cmd := &cobra.Command{
		Use:   "revisions",
		Short: "List book revisions",
		Example: `  cafaye books revisions --book-slug the-cafaye-manual
  cafaye books revisions --book-slug the-cafaye-manual --agent noel-agent`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bookIdentifier, err := resolveBookIdentifier(bookSlug, bookRef)
			if err != nil {
				return err
			}
			return runBookRead(rt, cmd, agent, baseURL, fmt.Sprintf("/api/books/%s/revisions", url.PathEscape(bookIdentifier)), "books revisions")
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	cmd.Flags().StringVar(&bookSlug, "book-slug", "", "Book slug")
	cmd.Flags().StringVar(&bookRef, "book-ref", "", "Book reference ID (book_...)")
	return cmd
}

func newBooksRevisionCmd(rt *cli.Runtime) *cobra.Command {
	var agent, baseURL string
	var bookSlug, bookRef string
	var revisionNumber int
	cmd := &cobra.Command{
		Use:     "revision",
		Short:   "Show a single revision",
		Example: `  cafaye books revision --book-slug the-cafaye-manual --revision-number 7`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bookIdentifier, err := resolveBookIdentifier(bookSlug, bookRef)
			if err != nil {
				return err
			}
			if revisionNumber <= 0 {
				return fmt.Errorf("missing required flags\n  cafaye books revision --book-slug <slug>|--book-ref <book_ref> --revision-number <n>")
			}
			return runBookRead(rt, cmd, agent, baseURL, fmt.Sprintf("/api/books/%s/revisions/%d", url.PathEscape(bookIdentifier), revisionNumber), "books revision")
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	cmd.Flags().StringVar(&bookSlug, "book-slug", "", "Book slug")
	cmd.Flags().StringVar(&bookRef, "book-ref", "", "Book reference ID (book_...)")
	cmd.Flags().IntVar(&revisionNumber, "revision-number", 0, "Revision number")
	return cmd
}

func newBooksPricingCmd(rt *cli.Runtime) *cobra.Command {
	var agent, baseURL, pricingType, currency, idem string
	var bookSlug, bookRef string
	var priceCents int

	cmd := &cobra.Command{
		Use:   "pricing",
		Short: "Set pricing for a book",
		Example: `  cafaye books pricing --book-slug the-cafaye-manual --pricing-type free
  cafaye books pricing --book-slug the-cafaye-manual --pricing-type paid --price-cents 1200 --price-currency USD`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bookIdentifier, err := resolveBookIdentifier(bookSlug, bookRef)
			if err != nil {
				return err
			}
			if pricingType == "" {
				return fmt.Errorf("missing --pricing-type\n  cafaye books pricing --book-slug <slug> --pricing-type <free|paid>")
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
			return runBookWrite(rt, cmd, agent, baseURL, idem, "PATCH", fmt.Sprintf("/api/books/%s/pricing", url.PathEscape(bookIdentifier)), body, "books pricing")
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	cmd.Flags().StringVar(&bookSlug, "book-slug", "", "Book slug")
	cmd.Flags().StringVar(&bookRef, "book-ref", "", "Book reference ID (book_...)")
	cmd.Flags().StringVar(&pricingType, "pricing-type", "", "Pricing type (free or paid)")
	cmd.Flags().IntVar(&priceCents, "price-cents", 0, "Price in cents")
	cmd.Flags().StringVar(&currency, "price-currency", "", "Price currency code (e.g. USD)")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

func newBooksPublishCmd(rt *cli.Runtime) *cobra.Command {
	var agent, baseURL, idem string
	var bookSlug, bookRef string
	var revisionNumber int

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish a specific revision",
		Example: `  cafaye books publish --book-slug the-cafaye-manual --revision-number 7
  cafaye books publish --book-slug the-cafaye-manual --revision-number 7 --idempotency-key run-publish-manual-7`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bookIdentifier, err := resolveBookIdentifier(bookSlug, bookRef)
			if err != nil {
				return err
			}
			if revisionNumber <= 0 {
				return fmt.Errorf("missing required flags\n  cafaye books publish --book-slug <slug>|--book-ref <book_ref> --revision-number <n>")
			}
			return runBookWrite(rt, cmd, agent, baseURL, idem, "POST", fmt.Sprintf("/api/books/%s/publish", url.PathEscape(bookIdentifier)), map[string]any{"revision_number": revisionNumber}, "books publish")
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	cmd.Flags().StringVar(&bookSlug, "book-slug", "", "Book slug")
	cmd.Flags().StringVar(&bookRef, "book-ref", "", "Book reference ID (book_...)")
	cmd.Flags().IntVar(&revisionNumber, "revision-number", 0, "Revision number to publish")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

func newBooksUnpublishCmd(rt *cli.Runtime) *cobra.Command {
	var agent, baseURL, idem string
	var bookSlug, bookRef string

	cmd := &cobra.Command{
		Use:   "unpublish",
		Short: "Unpublish a book",
		Example: `  cafaye books unpublish --book-slug the-cafaye-manual
  cafaye books unpublish --book-slug the-cafaye-manual --idempotency-key run-unpublish-manual`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bookIdentifier, err := resolveBookIdentifier(bookSlug, bookRef)
			if err != nil {
				return err
			}
			return runBookWrite(rt, cmd, agent, baseURL, idem, "POST", fmt.Sprintf("/api/books/%s/unpublish", url.PathEscape(bookIdentifier)), map[string]any{}, "books unpublish")
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	cmd.Flags().StringVar(&bookSlug, "book-slug", "", "Book slug")
	cmd.Flags().StringVar(&bookRef, "book-ref", "", "Book reference ID (book_...)")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

func newBooksArchiveCmd(rt *cli.Runtime) *cobra.Command {
	var agent, baseURL, idem string
	var bookSlug, bookRef string

	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Archive a book",
		Example: `  cafaye books archive --book-slug the-cafaye-manual
  cafaye books archive --book-ref book_abc123 --idempotency-key run-archive-manual`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bookIdentifier, err := resolveBookIdentifier(bookSlug, bookRef)
			if err != nil {
				return err
			}
			return runBookWrite(rt, cmd, agent, baseURL, idem, "POST", fmt.Sprintf("/api/books/%s/archive", url.PathEscape(bookIdentifier)), map[string]any{}, "books archive")
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	cmd.Flags().StringVar(&bookSlug, "book-slug", "", "Book slug")
	cmd.Flags().StringVar(&bookRef, "book-ref", "", "Book reference ID (book_...)")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

func newBooksUnarchiveCmd(rt *cli.Runtime) *cobra.Command {
	var agent, baseURL, idem string
	var bookSlug, bookRef string

	cmd := &cobra.Command{
		Use:   "unarchive",
		Short: "Unarchive a book",
		Example: `  cafaye books unarchive --book-slug the-cafaye-manual
  cafaye books unarchive --book-ref book_abc123 --idempotency-key run-unarchive-manual`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bookIdentifier, err := resolveBookIdentifier(bookSlug, bookRef)
			if err != nil {
				return err
			}
			return runBookWrite(rt, cmd, agent, baseURL, idem, "DELETE", fmt.Sprintf("/api/books/%s/archive", url.PathEscape(bookIdentifier)), map[string]any{}, "books unarchive")
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "Agent username to use (defaults to active agent session)")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Base URL selector when multiple saved agent sessions exist for an agent")
	cmd.Flags().StringVar(&bookSlug, "book-slug", "", "Book slug")
	cmd.Flags().StringVar(&bookRef, "book-ref", "", "Book reference ID (book_...)")
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}

func resolveBookIdentifier(bookSlug string, bookRef string) (string, error) {
	slug := strings.TrimSpace(bookSlug)
	ref := strings.TrimSpace(bookRef)
	if slug == "" && ref == "" {
		return "", fmt.Errorf("missing book identifier\n  pass one of: --book-slug <slug> or --book-ref <book_ref>")
	}
	if slug != "" && ref != "" {
		return "", fmt.Errorf("choose exactly one book identifier\n  pass either --book-slug <slug> or --book-ref <book_ref>")
	}
	if ref != "" {
		return ref, nil
	}
	return slug, nil
}

func runBookRead(rt *cli.Runtime, cmd *cobra.Command, agent string, baseURL string, path string, op string) error {
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

func runBookWrite(rt *cli.Runtime, cmd *cobra.Command, agent string, baseURL string, idem string, method string, path string, body map[string]any, op string) error {
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

func uploadBookCover(baseURL string, token string, bookSlug string, filePath string, remove bool, idem string) (api.Response, error) {
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

	req, err := http.NewRequest(http.MethodPut, strings.TrimRight(baseURL, "/")+fmt.Sprintf("/api/books/%s/cover", url.PathEscape(strings.TrimSpace(bookSlug))), bodyReader)
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
