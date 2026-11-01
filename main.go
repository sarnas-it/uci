// uci — Universal Content Intake: одна ссылка → скачанное содержимое.
//
// Кроссплатформенный (Linux/macOS/Windows) бинарный порт ядра-загрузчика
// github.com/YIAN6557/universal-content-intake. Автомониторинг, субтитры
// и облачная часть оригинала сюда не портированы.
package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const usage = `uci — одна ссылка, и она скачивается: видео, документ, картинки, статья или страница целиком.

Использование:
  uci [флаги] <ссылка>
  uci doctor
  uci help

Флаги:
  --type video|document|image|images|article|webpage
                         принудительный тип (по умолчанию определяется сам)
  --to <папка>           куда сохранить (по умолчанию — папка загрузок)
  --quality 720p|1080p|1440p|4k|highest
                         качество видео (по умолчанию 1080p)

Зависимости-«движки»: yt-dlp (видео), ffmpeg (склейка), gallery-dl (галереи),
SingleFile + Chrome (страницы целиком). "uci doctor" проверит окружение.
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "help", "--help", "-h":
			fmt.Print(usage)
			return 0
		case "doctor":
			return doctor()
		}
	}
	flags := flag.NewFlagSet("uci", flag.ContinueOnError)
	forcedType := flags.String("type", "", "video|document|image|images|article|webpage")
	dest := flags.String("to", "", "папка назначения")
	quality := flags.String("quality", "1080p", "качество видео")
	if err := flags.Parse(flagsFirst(args)); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	rawURL := strings.TrimSpace(flags.Arg(0))
	if u, err := url.Parse(rawURL); err != nil || u.Scheme == "" || u.Host == "" {
		fmt.Fprintf(os.Stderr, "✗ это не похоже на ссылку: %q\n", rawURL)
		return 2
	}
	destDir := *dest
	if destDir == "" {
		destDir = downloadsDir()
	} else if strings.HasPrefix(destDir, "~") {
		home, _ := os.UserHomeDir()
		destDir = filepath.Join(home, strings.TrimPrefix(destDir, "~"))
	}
	if err := get(rawURL, ContentType(*forcedType), destDir, *quality); err != nil {
		fmt.Fprintf(os.Stderr, "✗ %v\n", err)
		return 1
	}
	return 0
}

// flagsFirst переставляет флаги перед позиционными аргументами:
// стандартный flag-пакет обрывает разбор на первом не-флаге,
// а пользователи пишут и `uci <url> --to dir`.
func flagsFirst(args []string) []string {
	var flagArgs, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			if !strings.Contains(arg, "=") && i+1 < len(args) {
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, arg)
	}
	return append(flagArgs, positional...)
}

// get классифицирует ссылку (с пробингом при необходимости) и скачивает.
func get(rawURL string, forced ContentType, destDir, quality string) error {
	ctype := forced
	if ctype == "" {
		known, ok := classify(rawURL)
		if !ok {
			fmt.Println("… определяю тип по ответу сервера")
			ctype = fromContentType(probeContentType(rawURL))
		} else {
			ctype = known
		}
	}
	fmt.Printf("→ тип: %s, папка: %s\n", ctype, destDir)

	var files []string
	var info *Info
	var err error
	switch ctype {
	case TypeVideo:
		files, info, err = downloadVideo(rawURL, destDir, quality)
	case TypeDocument:
		if u, _ := url.Parse(rawURL); u != nil {
			if id := gdriveID(u); id != "" {
				files, info, err = downloadGDrive(id, destDir)
				break
			}
		}
		files, info, err = downloadDirect(rawURL, destDir)
	case TypeImage:
		files, info, err = downloadImages(rawURL, destDir, false)
	case TypeImages:
		files, info, err = downloadImages(rawURL, destDir, true)
	case TypeArticle:
		files, info, err = downloadArticle(rawURL, destDir)
	case TypeWebpage:
		files, info, err = downloadWebpage(rawURL, destDir)
	default:
		return fmt.Errorf("неизвестный тип %q (варианты: video, document, image, images, article, webpage)", ctype)
	}
	if err != nil {
		return err
	}
	if sidecarErr := writeInfoSidecar(files, info); sidecarErr != nil {
		fmt.Fprintf(os.Stderr, "⚠ не записал info-файл: %v\n", sidecarErr)
	}
	for _, file := range files {
		fmt.Printf("✓ %s\n", file)
	}
	return nil
}

// doctor проверяет окружение и печатает команды установки недостающего.
func doctor() int {
	fmt.Printf("uci doctor · платформа: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("✓ папка загрузок: %s\n", downloadsDir())
	missing := 0
	tools := []struct {
		name     string
		required bool
		why      string
	}{
		{"yt-dlp", true, "скачивание видео"},
		{"ffmpeg", true, "склейка видео+аудио в 1080p и выше"},
		{"deno", false, "JS-рантайм для yt-dlp (YouTube без него режет форматы)"},
		{"gallery-dl", false, "альбомы и галереи картинок"},
		{"single-file", false, "сохранение страниц целиком"},
		{"google-chrome", false, "движок для SingleFile"},
	}
	for _, tool := range tools {
		if path := lookTool(tool.name); path != "" {
			fmt.Printf("✓ %s — %s\n", tool.name, path)
			continue
		}
		mark, label := "○", "не обязателен"
		if tool.required {
			mark, label = "✗", "нужен"
			missing++
		}
		fmt.Printf("%s %s (%s, %s) — установка: %s\n", mark, tool.name, tool.why, label, installHint(tool.name))
	}
	fmt.Println()
	if missing > 0 {
		fmt.Printf("Не хватает %d обязательных инструментов.\n", missing)
		return 1
	}
	fmt.Println("Обязательное на месте, можно качать: uci <ссылка>")
	return 0
}
