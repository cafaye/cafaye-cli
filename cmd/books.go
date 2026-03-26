package cmd

import (
	"encoding/json"
	"fmt"
	"time"

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
				return fmt.Errorf("books list failed: status=%d body=%s", resp.StatusCode, string(resp.Body))
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
	cmd.AddCommand(newBooksPricingCmd(rt))
	cmd.AddCommand(newBooksPublishCmd(rt))
	cmd.AddCommand(newBooksUnpublishCmd(rt))
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

			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			client, err := clientForProfile(rt, cfg, profile)
			if err != nil {
				return err
			}

			if idem == "" {
				idem = fmt.Sprintf("run-books-pricing-%d", time.Now().UnixNano())
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

			resp, err := client.Do("PATCH", fmt.Sprintf("/api/books/%d/pricing", bookID), body, idem)
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return fmt.Errorf("books pricing failed: status=%d body=%s", resp.StatusCode, string(resp.Body))
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
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			client, err := clientForProfile(rt, cfg, profile)
			if err != nil {
				return err
			}
			if idem == "" {
				idem = fmt.Sprintf("run-books-publish-%d", time.Now().UnixNano())
			}
			resp, err := client.Do("POST", fmt.Sprintf("/api/books/%d/publish", bookID), map[string]any{"revision_id": revisionID}, idem)
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return fmt.Errorf("books publish failed: status=%d body=%s", resp.StatusCode, string(resp.Body))
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
			cfg, err := rt.LoadConfig()
			if err != nil {
				return err
			}
			client, err := clientForProfile(rt, cfg, profile)
			if err != nil {
				return err
			}
			if idem == "" {
				idem = fmt.Sprintf("run-books-unpublish-%d", time.Now().UnixNano())
			}
			resp, err := client.Do("POST", fmt.Sprintf("/api/books/%d/unpublish", bookID), map[string]any{}, idem)
			if err != nil {
				return err
			}
			cli.PrintDeprecation(cmd.ErrOrStderr(), resp.Deprecation)
			if resp.StatusCode >= 300 {
				return fmt.Errorf("books unpublish failed: status=%d body=%s", resp.StatusCode, string(resp.Body))
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
	cmd.Flags().StringVar(&idem, "idempotency-key", "", "Stable idempotency key (auto-generated if omitted)")
	return cmd
}
