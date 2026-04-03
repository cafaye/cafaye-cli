package cmd

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type bundleSource interface {
	ReadFile(relPath string) ([]byte, error)
	ListMarkdownFiles() ([]string, error)
}

type dirBundleSource struct {
	root string
}

func (d dirBundleSource) ReadFile(relPath string) ([]byte, error) {
	clean := path.Clean(strings.TrimSpace(relPath))
	p := filepath.Join(d.root, filepath.FromSlash(clean))
	return os.ReadFile(p)
}

func (d dirBundleSource) ListMarkdownFiles() ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(d.root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			rel, relErr := filepath.Rel(d.root, p)
			if relErr != nil {
				return relErr
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	return files, err
}

type zipBundleSource struct {
	files map[string]*zip.File
}

func (z zipBundleSource) ReadFile(relPath string) ([]byte, error) {
	clean := path.Clean(strings.TrimSpace(relPath))
	f, ok := z.files[clean]
	if !ok {
		return nil, os.ErrNotExist
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (z zipBundleSource) ListMarkdownFiles() ([]string, error) {
	files := make([]string, 0, len(z.files))
	for name := range z.files {
		if strings.HasSuffix(strings.ToLower(name), ".md") {
			files = append(files, name)
		}
	}
	return files, nil
}

type bundleManifest struct {
	SchemaVersion any      `yaml:"schema_version"`
	BookUID       string   `yaml:"book_uid"`
	Title         string   `yaml:"title"`
	Author        string   `yaml:"author"`
	ReadingOrder  []string `yaml:"reading_order"`
}

type validateResult struct {
	Valid     bool     `json:"valid"`
	Source    string   `json:"source"`
	Path      string   `json:"path"`
	Errors    []string `json:"errors,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
	NextSteps []string `json:"next_steps,omitempty"`
}

func newBooksValidateCmd() *cobra.Command {
	var sourcePath string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a local source bundle directory or zip before upload",
		Example: `  cafaye books validate --path ./my-book
  cafaye books validate --path ./bundle.zip`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sourcePath = strings.TrimSpace(sourcePath)
			if sourcePath == "" {
				return fmt.Errorf("missing --path\n  cafaye books validate --path <dir|zip>")
			}
			result, err := validateBundlePath(sourcePath)
			if err != nil {
				return err
			}
			return printJSON(cmd.OutOrStdout(), result)
		},
	}
	cmd.Flags().StringVar(&sourcePath, "path", "", "Path to bundle directory or zip file")
	return cmd
}

func validateBundlePath(sourcePath string) (validateResult, error) {
	absPath, err := filepath.Abs(sourcePath)
	if err != nil {
		return validateResult{}, err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return validateResult{}, err
	}

	var (
		sourceKind string
		source     bundleSource
	)
	if info.IsDir() {
		sourceKind = "directory"
		source = dirBundleSource{root: absPath}
	} else {
		if !strings.EqualFold(filepath.Ext(absPath), ".zip") {
			return validateResult{}, fmt.Errorf("unsupported --path: expected directory or .zip file")
		}
		reader, err := zip.OpenReader(absPath)
		if err != nil {
			return validateResult{}, err
		}
		defer reader.Close()
		files := make(map[string]*zip.File, len(reader.File))
		for _, f := range reader.File {
			name := path.Clean(strings.TrimPrefix(f.Name, "./"))
			if strings.HasSuffix(name, "/") {
				continue
			}
			files[name] = f
		}
		sourceKind = "zip"
		source = zipBundleSource{files: files}
	}

	result := validateResult{Valid: true, Source: sourceKind, Path: absPath}
	errors, warnings := validateBundle(source)
	result.Errors = errors
	result.Warnings = warnings
	result.Valid = len(errors) == 0
	if !result.Valid {
		result.NextSteps = []string{
			"Fix validation errors and rerun cafaye books validate --path <dir|zip>",
			"When validation passes, upload with cafaye books upload --file <bundle.zip> --idempotency-key run-<stable-key>",
		}
	}
	return result, nil
}

func validateBundle(source bundleSource) ([]string, []string) {
	errors := make([]string, 0)
	warnings := make([]string, 0)

	bookBytes, err := source.ReadFile("book.yml")
	if err != nil {
		return append(errors, "missing required file: book.yml"), warnings
	}

	var manifest bundleManifest
	if yamlErr := yaml.Unmarshal(bookBytes, &manifest); yamlErr != nil {
		return append(errors, fmt.Sprintf("book.yml parse error: %v", yamlErr)), warnings
	}

	if manifest.SchemaVersion == nil || strings.TrimSpace(fmt.Sprint(manifest.SchemaVersion)) == "" {
		errors = append(errors, "book.yml missing required field: schema_version")
	}
	if strings.TrimSpace(manifest.BookUID) == "" {
		errors = append(errors, "book.yml missing required field: book_uid")
	}
	if strings.TrimSpace(manifest.Title) == "" {
		errors = append(errors, "book.yml missing required field: title")
	}
	if strings.TrimSpace(manifest.Author) == "" {
		errors = append(errors, "book.yml missing required field: author")
	}
	if len(manifest.ReadingOrder) == 0 {
		errors = append(errors, "book.yml missing required field: reading_order")
		return errors, warnings
	}

	readingSet := make(map[string]struct{}, len(manifest.ReadingOrder))
	for _, raw := range manifest.ReadingOrder {
		entry := path.Clean(strings.TrimPrefix(strings.TrimSpace(raw), "./"))
		if entry == "." || entry == "" {
			errors = append(errors, "book.yml reading_order contains an empty path")
			continue
		}
		readingSet[entry] = struct{}{}
		if !strings.HasSuffix(strings.ToLower(entry), ".md") {
			errors = append(errors, fmt.Sprintf("reading_order entry must reference a markdown file: %s", entry))
			continue
		}
		body, readErr := source.ReadFile(entry)
		if readErr != nil {
			errors = append(errors, fmt.Sprintf("reading_order file not found: %s", entry))
			continue
		}
		if idErr := validateFrontMatterID(entry, body); idErr != nil {
			errors = append(errors, idErr.Error())
		}
	}

	mdFiles, err := source.ListMarkdownFiles()
	if err == nil {
		extra := make([]string, 0)
		for _, f := range mdFiles {
			clean := path.Clean(strings.TrimPrefix(strings.TrimSpace(f), "./"))
			if _, ok := readingSet[clean]; !ok && clean != "book.yml" {
				extra = append(extra, clean)
			}
		}
		if len(extra) > 0 {
			warnings = append(warnings, fmt.Sprintf("markdown files not listed in reading_order will be ignored: %s", strings.Join(extra, ", ")))
		}
	}

	return errors, warnings
}

func validateFrontMatterID(relPath string, body []byte) error {
	frontMatter, err := extractFrontMatter(body)
	if err != nil {
		return fmt.Errorf("%s: %v", relPath, err)
	}
	var fields map[string]any
	if yamlErr := yaml.Unmarshal(frontMatter, &fields); yamlErr != nil {
		return fmt.Errorf("%s: invalid front matter: %v", relPath, yamlErr)
	}
	id := strings.TrimSpace(fmt.Sprint(fields["id"]))
	if id == "" || id == "<nil>" {
		return fmt.Errorf("%s: missing required front matter field: id", relPath)
	}
	return nil
}

func extractFrontMatter(body []byte) ([]byte, error) {
	lines := bytes.Split(body, []byte("\n"))
	if len(lines) == 0 {
		return nil, fmt.Errorf("missing front matter block")
	}
	first := strings.TrimSpace(string(bytes.TrimSuffix(lines[0], []byte("\r"))))
	if first != "---" {
		return nil, fmt.Errorf("missing front matter block")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(string(bytes.TrimSuffix(lines[i], []byte("\r"))))
		if line == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return nil, fmt.Errorf("unterminated front matter block")
	}
	front := bytes.Join(lines[1:end], []byte("\n"))
	return front, nil
}

