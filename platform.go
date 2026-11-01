package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Платформенные различия — только здесь; остальной код вызывает эти функции,
// а не проверяет runtime.GOOS сам.

// downloadsDir — папка «Загрузки» пользователя.
// На Linux читаем XDG_DOWNLOAD_DIR (локаль может переименовать её в «Загрузки»),
// на macOS/Windows это всегда ~/Downloads.
func downloadsDir() string {
	if runtime.GOOS == "linux" {
		if dir := xdgUserDir("DOWNLOAD"); dir != "" {
			return dir
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads")
}

func xdgUserDir(name string) string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configHome = filepath.Join(home, ".config")
	}
	file, err := os.Open(filepath.Join(configHome, "user-dirs.dirs"))
	if err != nil {
		return ""
	}
	defer file.Close()
	prefix := "XDG_" + name + "_DIR=\""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if rest, ok := strings.CutPrefix(line, prefix); ok {
			value := strings.TrimSuffix(rest, "\"")
			if home, err := os.UserHomeDir(); err == nil {
				value = strings.Replace(value, "$HOME", home, 1)
			}
			return value
		}
	}
	return ""
}

// dataDir — куда uci складывает своё состояние (не настройки).
func dataDir() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(base, "Universal Content Intake")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Universal Content Intake")
	default:
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			base = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(base, "Universal Content Intake")
	}
}

// lookTool ищет программу в PATH и в типичных для платформы местах,
// куда ставятся CLI-инструменты (фоновый запуск может иметь короткий PATH).
func lookTool(name string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	home, _ := os.UserHomeDir()
	var extra []string
	switch runtime.GOOS {
	case "windows":
		extra = []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WinGet", "Links")}
		name += ".exe"
	case "darwin":
		extra = []string{"/opt/homebrew/bin", "/usr/local/bin", filepath.Join(home, ".deno", "bin")}
	default:
		extra = []string{filepath.Join(home, ".local", "bin"), filepath.Join(home, ".deno", "bin"), "/usr/local/bin", "/snap/bin"}
	}
	for _, dir := range extra {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// installHint — команда установки инструмента под текущую платформу.
func installHint(tool string) string {
	switch runtime.GOOS {
	case "windows":
		ids := map[string]string{
			"yt-dlp":      "winget install --id yt-dlp.yt-dlp -e",
			"ffmpeg":      "winget install --id Gyan.FFmpeg -e",
			"deno":        "winget install --id DenoLand.Deno -e",
			"gallery-dl":  "pip install gallery-dl",
			"single-file": "npm install -g single-file-cli",
			"chrome":      "winget install --id Google.Chrome -e",
		}
		return orDefault(ids, tool, "см. https://github.com/"+tool)
	case "darwin":
		return "brew install " + tool
	default:
		pkgs := map[string]string{
			"yt-dlp":      "pip install --user yt-dlp  # или sudo apt install yt-dlp",
			"ffmpeg":      "sudo apt install ffmpeg",
			"deno":        "curl -fsSL https://deno.land/install.sh | sh",
			"gallery-dl":  "pip install --user gallery-dl",
			"single-file": "npm install -g single-file-cli",
			"chrome":      "sudo apt install chromium  # или google-chrome с сайта Google",
		}
		return orDefault(pkgs, tool, "sudo apt install "+tool)
	}
}

func orDefault(m map[string]string, key, fallback string) string {
	if value, ok := m[key]; ok {
		return value
	}
	return fallback
}
