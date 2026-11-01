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
  uci doctor [--yes]
  uci help

Флаги:
  --type video|document|image|images|article|webpage
                         принудительный тип (по умолчанию определяется сам)
  --to <папка>           куда сохранить (по умолчанию — папка загрузок)
  --quality 720p|1080p|1440p|4k|highest
                         качество видео (по умолчанию 1080p)

Зависимости-«движки»: yt-dlp (видео), ffmpeg (склейка), gallery-dl (галереи),
SingleFile + Chrome (страницы целиком). "uci doctor" проверит окружение
и сам поставит недостающее (с --yes — без вопросов).
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
			return doctor(args[1:])
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

// toolSpec — движок, который doctor проверяет и (если install != nil) ставит сам.
type toolSpec struct {
	name     string
	required bool
	why      string
	install  func(dir string) error
}

func toolSpecs() []toolSpec {
	return []toolSpec{
		{"yt-dlp", true, "скачивание видео", installYtDlp},
		{"ffmpeg", true, "склейка видео+аудио в 1080p и выше", installFFmpeg},
		{"deno", false, "JS-рантайм для yt-dlp (YouTube без него режет форматы)", installDeno},
		{"gallery-dl", false, "альбомы и галереи картинок", installGalleryDl},
		{"single-file", false, "сохранение страниц целиком", installSingleFile},
		{"google-chrome", false, "движок для SingleFile", nil},
	}
}

// doctor проверяет окружение и с согласия пользователя ставит недостающее.
// --yes / -y — ставить всё недостающее без вопроса.
func doctor(args []string) int {
	yes := false
	for _, arg := range args {
		if arg == "--yes" || arg == "-y" {
			yes = true
		}
	}
	fmt.Printf("uci doctor · платформа: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("✓ папка загрузок: %s\n", downloadsDir())

	missingRequired := 0
	var missing []toolSpec
	for _, tool := range toolSpecs() {
		if path := lookTool(tool.name); path != "" {
			fmt.Printf("✓ %s — %s\n", tool.name, path)
			continue
		}
		mark, label := "○", "не обязателен"
		if tool.required {
			mark, label = "✗", "нужен"
			missingRequired++
		}
		fmt.Printf("%s %s (%s, %s)\n", mark, tool.name, tool.why, label)
		missing = append(missing, tool)
	}
	if len(missing) == 0 {
		fmt.Println("\nВсё на месте, можно качать: uci <ссылка>")
		return 0
	}

	if !yes {
		if !stdinIsTTY() {
			fmt.Printf("\nНе хватает инструментов, а я не в терминале — перезапустите: uci doctor --yes\n")
			return 1
		}
		fmt.Printf("\nУстановить недостающее в %s? [y/N] ", installDir())
		var answer string
		fmt.Scanln(&answer)
		yes = strings.EqualFold(strings.TrimSpace(answer), "y")
	}
	if !yes {
		fmt.Println("Установка отменена. Ручные команды:")
		for _, tool := range missing {
			fmt.Printf("  %s: %s\n", tool.name, installHint(tool.name))
		}
		return 1
	}

	dir := installDir()
	for _, tool := range missing {
		if tool.install == nil {
			fmt.Printf("○ %s — автоматически не ставится, установите вручную: %s\n", tool.name, installHint(tool.name))
			continue
		}
		fmt.Printf("→ ставлю %s…\n", tool.name)
		if err := tool.install(dir); err != nil {
			fmt.Printf("✗ %s — не получилось: %v\n  вручную: %s\n", tool.name, err, installHint(tool.name))
			continue
		}
		if path := lookTool(tool.name); path != "" {
			fmt.Printf("✓ %s — %s\n", tool.name, path)
			if tool.required {
				missingRequired--
			}
		} else {
			fmt.Printf("⚠ %s поставлен в %s, но не виден — добавьте %s в PATH\n", tool.name, dir, dir)
		}
	}
	fmt.Println()
	if missingRequired > 0 {
		fmt.Printf("Не хватает %d обязательных инструментов.\n", missingRequired)
		return 1
	}
	fmt.Println("Обязательное на месте, можно качать: uci <ссылка>")
	return 0
}

// stdinIsTTY — работаем ли мы в интерактивном терминале (а не в pipe/cron).
func stdinIsTTY() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
