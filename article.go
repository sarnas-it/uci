package main

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/go-shiori/go-readability"
)

// downloadArticle извлекает текст статьи (readability) и сохраняет как Markdown.
func downloadArticle(rawURL, destDir string) ([]string, *Info, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, err
	}
	// Качаем сами: readability.FromURL ходит с Go-шным UA, который, например, Википедия режет.
	page, err := fetch(rawURL)
	if err != nil {
		return nil, nil, err
	}
	defer page.Body.Close()
	article, err := readability.FromReader(page.Body, u)
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось разобрать статью: %w", err)
	}
	converter := md.NewConverter("", true, nil)
	markdown, err := converter.ConvertString(article.Content)
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось преобразовать статью в Markdown: %w", err)
	}
	title := article.Title
	if title == "" {
		title = "article"
	}
	name := safeName(title) + ".md"
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, nil, err
	}
	path := collisionFree(destDir, name)
	header := "# " + article.Title + "\n\n"
	if err := os.WriteFile(path, []byte(header+markdown), 0o644); err != nil {
		return nil, nil, err
	}
	return []string{path}, &Info{
		SourceURL: rawURL, Type: TypeArticle,
		Title: article.Title, Author: article.Byline, SiteName: article.SiteName,
	}, nil
}

// downloadWebpage сохраняет страницу целиком: SingleFile + Chrome, если есть;
// иначе — один HTML-файл как его отдал сервер.
func downloadWebpage(rawURL, destDir string) ([]string, *Info, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, nil, err
	}
	singleFile := lookTool("single-file")
	chrome := lookTool("google-chrome")
	if chrome == "" {
		chrome = lookTool("chromium")
	}
	u, _ := url.Parse(rawURL)
	base := safeName(u.Hostname() + filepath.Base(u.Path))
	if base == "" {
		base = "page"
	}
	if singleFile != "" && chrome != "" {
		out := collisionFree(destDir, base+".html")
		cmd := exec.Command(singleFile, rawURL, out, "--browser-executable-path", chrome)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			if info, statErr := os.Stat(out); statErr == nil && info.Size() > 0 {
				return []string{out}, &Info{SourceURL: rawURL, Type: TypeWebpage}, nil
			}
		}
		fmt.Fprintln(os.Stderr, "⚠ SingleFile не сработал, сохраняю исходный HTML")
	} else if singleFile == "" {
		fmt.Fprintf(os.Stderr, "⚠ нет SingleFile/Chrome для полного снимка страницы (%s), сохраняю исходный HTML\n", installHint("single-file"))
	}
	// Фолбэк: HTML как отдал сервер.
	result, err := fetch(rawURL)
	if err != nil {
		return nil, nil, err
	}
	defer result.Body.Close()
	path, err := downloadFile(result.Body, destDir, base+".html")
	if err != nil {
		return nil, nil, err
	}
	return []string{path}, &Info{SourceURL: rawURL, Type: TypeWebpage}, nil
}
