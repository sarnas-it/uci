package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// Info — короткая запись рядом со скачанным: источник, автор, ссылка.
type Info struct {
	Type      ContentType
	SourceURL string
	Title     string
	Author    string
	SiteName  string
}

// safeName делает имя файла безопасным на всех трёх ОС:
// без запрещённых символов, зарезервированных имён Windows и с ограничением длины.
func safeName(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' ||
			r == '"' || r == '<' || r == '>' || r == '|':
			return -1
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, name)
	name = strings.Join(strings.Fields(name), " ")
	name = strings.Trim(name, " .")
	if name == "" {
		return "download"
	}
	// Зарезервированные имена Windows (CON, NUL, AUX, COM1.., LPT1..).
	upper := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	reserved := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true,
		"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true,
		"COM7": true, "COM8": true, "COM9": true, "LPT1": true, "LPT2": true, "LPT3": true,
		"LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true}
	if reserved[upper] {
		name = "_" + name
	}
	// Ограничение 255 байт на имя; режем по рунам, оставляя расширение.
	const maxBytes = 200
	if len(name) > maxBytes {
		ext := filepath.Ext(name)
		stem := name[:len(name)-len(ext)]
		for len(stem)+len(ext) > maxBytes && stem != "" {
			r := []rune(stem)
			stem = string(r[:len(r)-1])
		}
		name = strings.TrimRight(stem, " .") + ext
	}
	return name
}

// collisionFree подбирает свободное имя: name, name-2, name-3…
func collisionFree(dir, name string) string {
	candidate := filepath.Join(dir, name)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	ext := filepath.Ext(name)
	stem := name[:len(name)-len(ext)]
	for n := 2; ; n++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s-%d%s", stem, n, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// writeInfoSidecar кладёт рядом с первым файлом <имя>.info.md.
func writeInfoSidecar(files []string, info *Info) error {
	if len(files) == 0 || info == nil {
		return nil
	}
	first := files[0]
	ext := filepath.Ext(first)
	sidecar := first[:len(first)-len(ext)] + ".info.md"
	sidecar = collisionFree(filepath.Dir(first), filepath.Base(sidecar))
	var b strings.Builder
	fmt.Fprintf(&b, "- Type: %s\n", info.Type)
	fmt.Fprintf(&b, "- Source URL: %s\n", info.SourceURL)
	if info.Title != "" {
		fmt.Fprintf(&b, "- Original Title: %s\n", info.Title)
	}
	if info.Author != "" {
		fmt.Fprintf(&b, "- Author: %s\n", info.Author)
	}
	if info.SiteName != "" {
		fmt.Fprintf(&b, "- Site: %s\n", info.SiteName)
	}
	fmt.Fprintf(&b, "- Downloaded At: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Files: %d\n", len(files))
	return os.WriteFile(sidecar, []byte(b.String()), 0o644)
}
