package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// downloadImages — одиночная картинка скачивается напрямую,
// альбомы и галереи — через gallery-dl (в отдельную подпапку).
func downloadImages(rawURL, destDir string, gallery bool) ([]string, *Info, error) {
	if !gallery {
		files, info, err := downloadDirect(rawURL, destDir)
		if info != nil {
			info.Type = TypeImage
		}
		return files, info, err
	}
	gdl := lookTool("gallery-dl")
	if gdl == "" {
		return nil, nil, fmt.Errorf("не найден gallery-dl. Установка: %s", installHint("gallery-dl"))
	}
	before := map[string]bool{}
	filepath.WalkDir(destDir, func(path string, _ os.DirEntry, err error) error {
		if err == nil {
			before[path] = true
		}
		return nil
	})
	cmd := exec.Command(gdl, "-d", destDir, rawURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, nil, fmt.Errorf("gallery-dl завершился с ошибкой: %w", err)
	}
	var created []string
	filepath.WalkDir(destDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !before[path] && !strings.HasSuffix(path, ".info.md") {
			created = append(created, path)
		}
		return nil
	})
	if len(created) == 0 {
		return nil, nil, fmt.Errorf("gallery-dl ничего не скачал")
	}
	return created, &Info{SourceURL: rawURL, Type: TypeImages}, nil
}
