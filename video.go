package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// downloadVideo скачивает видео через yt-dlp в destDir, качество quality (1080p по умолчанию).
func downloadVideo(rawURL, destDir, quality string) ([]string, *Info, error) {
	ytdlp := lookTool("yt-dlp")
	if ytdlp == "" {
		return nil, nil, fmt.Errorf("не найден yt-dlp. Установка: %s", installHint("yt-dlp"))
	}
	hasFFmpeg := lookTool("ffmpeg") != ""
	format := formatSelector(quality, hasFFmpeg)
	if !hasFFmpeg {
		fmt.Fprintf(os.Stderr, "⚠ ffmpeg не найден (%s); качаю одиночный формат без склейки\n", installHint("ffmpeg"))
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, nil, err
	}
	template := filepath.Join(destDir, "%(title).150B.%(ext)s")
	args := []string{"--no-playlist", "--restrict-filenames", "-o", template, "--print-json", "-f", format, rawURL}
	cmd := exec.Command(ytdlp, args...)
	// deno мог быть найден вне PATH (например ~/.deno/bin) — отдадим его yt-dlp.
	if deno := lookTool("deno"); deno != "" {
		cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(deno)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, nil, fmt.Errorf("yt-dlp завершился с ошибкой: %w", err)
	}
	// yt-dlp --print-json печатает метаданные скачанного файла (последний JSON — итоговый).
	var meta struct {
		Title    string `json:"title"`
		Uploader string `json:"uploader"`
		Webpage  string `json:"webpage_url"`
		FileName string `json:"_filename"`
	}
	lines := bytes.Split(bytes.TrimSpace(stdout.Bytes()), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		if json.Unmarshal(lines[i], &meta) == nil && meta.FileName != "" {
			break
		}
	}
	if meta.FileName == "" {
		return nil, nil, fmt.Errorf("yt-dlp не вернул имя скачанного файла")
	}
	return []string{meta.FileName}, &Info{
		SourceURL: rawURL, Type: TypeVideo, Title: meta.Title, Author: meta.Uploader,
	}, nil
}

// formatSelector — выбор формата yt-dlp под запрошенное качество.
// Низкое исходное качество никогда не апскейлим: берём лучшее не выше лимита.
// Без ffmpeg склейка видео+аудио невозможна — берём одиночный файл (b), иначе
// yt-dlp оставляет после себя недосклеенные части.
func formatSelector(quality string, hasFFmpeg bool) string {
	limit := "1080"
	switch strings.ToLower(quality) {
	case "720p":
		limit = "720"
	case "1440p":
		limit = "1440"
	case "4k", "2160p":
		limit = "2160"
	case "highest", "best":
		if hasFFmpeg {
			return "bv+ba/best"
		}
		return "best"
	}
	if hasFFmpeg {
		return "bv[height<=" + limit + "]+ba/b[height<=" + limit + "]/best"
	}
	return "b[height<=" + limit + "]/b/best"
}
