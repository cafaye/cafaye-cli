package workspace

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const defaultWorkspaceName = "starter-book"

//go:embed assets/starter/**
var starterFS embed.FS

type InitResult struct {
	WorkspacePath string
	Created       bool
	Populated     bool
}

type BookStarter struct {
	Slug     string
	Title    string
	Subtitle string
	Author   string
}

func EnsureStarterWorkspace(root string, name string) (InitResult, error) {
	workspaceName := strings.TrimSpace(name)
	if workspaceName == "" {
		workspaceName = defaultWorkspaceName
	}
	workspacePath := filepath.Join(root, workspaceName)

	created := false
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		created = true
	}
	if err := os.MkdirAll(workspacePath, 0o755); err != nil {
		return InitResult{}, err
	}

	populated := false
	err := fs.WalkDir(starterFS, "assets/starter", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "assets/starter" {
			return nil
		}

		rel := strings.TrimPrefix(path, "assets/starter/")
		target := filepath.Join(workspacePath, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		content, err := starterFS.ReadFile(path)
		if err != nil {
			return err
		}
		if prev, err := os.ReadFile(target); err == nil {
			if string(prev) == string(content) {
				return nil
			}
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return err
		}
		populated = true
		return nil
	})
	if err != nil {
		return InitResult{}, fmt.Errorf("failed to materialize starter workspace: %w", err)
	}

	return InitResult{WorkspacePath: workspacePath, Created: created, Populated: populated}, nil
}

func EnsureStarterWorkspaceForBook(root string, starter BookStarter) (InitResult, error) {
	slug := sanitizeSlug(starter.Slug)
	if slug == "" {
		return InitResult{}, fmt.Errorf("book slug is required")
	}
	res, err := EnsureStarterWorkspace(root, slug)
	if err != nil {
		return InitResult{}, err
	}
	bookYML := filepath.Join(res.WorkspacePath, "book.yml")
	content := renderBookYML(starter)
	prev, err := os.ReadFile(bookYML)
	if err != nil || string(prev) != content {
		if writeErr := os.WriteFile(bookYML, []byte(content), 0o644); writeErr != nil {
			return InitResult{}, writeErr
		}
		res.Populated = true
	}
	return res, nil
}

func sanitizeSlug(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	re := regexp.MustCompile(`[^a-z0-9-]+`)
	s = re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func renderBookYML(starter BookStarter) string {
	title := strings.TrimSpace(starter.Title)
	if title == "" {
		title = "Untitled Book"
	}
	author := strings.TrimSpace(starter.Author)
	if author == "" {
		author = "Cafaye Agent"
	}
	subtitleBlock := ""
	if strings.TrimSpace(starter.Subtitle) != "" {
		subtitleBlock = fmt.Sprintf("subtitle: %s\n", yamlQuote(starter.Subtitle))
	}
	return fmt.Sprintf(`schema_version: 1
book_uid: %s
title: %s
%sauthor: %s
language: en
description: Starter source bundle for new Cafaye books.
blurb: A short back-cover style pitch to help readers decide quickly.
synopsis: A longer summary explaining what the book covers and who it is for.
category: General
tags:
  - starter
theme: blue
reading_order:
  - content/001-start-here.md
`, yamlQuote(starter.Slug), yamlQuote(title), subtitleBlock, yamlQuote(author))
}

func yamlQuote(v string) string {
	s := strings.ReplaceAll(strings.TrimSpace(v), `"`, `\"`)
	return `"` + s + `"`
}
