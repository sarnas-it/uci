package main

import (
	"archive/tar"
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ulikunitz/xz"
)

// Автоустановка движков. Предпочтение — без sudo: статические бинари с официальных
// источников в installDir. ffmpeg на Linux/macOS ставится из пакетного менеджера,
// если он есть (там он лучше сопровождается), иначе — статическая сборка.

// installDir — куда складываем скачанные инструменты (без прав root).
func installDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(dataDir(), "bin")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

// githubAsset — прямая ссылка на файл последнего релиза GitHub.
func githubAsset(repo, asset string) string {
	return "https://github.com/" + repo + "/releases/latest/download/" + asset
}

// ytdlpAsset — standalone-бинарь yt-dlp под ОС (Python не нужен).
func ytdlpAsset(goos string) (string, error) {
	switch goos {
	case "linux":
		return "yt-dlp_linux", nil
	case "darwin":
		return "yt-dlp_macos", nil
	case "windows":
		return "yt-dlp.exe", nil
	}
	return "", fmt.Errorf("нет сборки yt-dlp под %s", goos)
}

// denoAsset — zip-архив deno под ОС/архитектуру.
func denoAsset(goos, goarch string) (string, error) {
	target := map[string]string{
		"linux/amd64":   "x86_64-unknown-linux-gnu",
		"linux/arm64":   "aarch64-unknown-linux-gnu",
		"darwin/amd64":  "x86_64-apple-darwin",
		"darwin/arm64":  "aarch64-apple-darwin",
		"windows/amd64": "x86_64-pc-windows-msvc",
	}[goos+"/"+goarch]
	if target == "" {
		return "", fmt.Errorf("нет сборки deno под %s/%s", goos, goarch)
	}
	return "deno-" + target + ".zip", nil
}

// ffmpegSource — откуда брать ffmpeg+ffprobe: "tarxz" (linux static),
// "evermeet" (macOS, два zip) или "gyan" (Windows, один zip).
func ffmpegSource(goos, goarch string) (kind string, urls []string, err error) {
	switch goos {
	case "linux":
		arch := map[string]string{"amd64": "amd64", "arm64": "arm64"}[goarch]
		if arch == "" {
			return "", nil, fmt.Errorf("нет статической сборки ffmpeg под linux/%s", goarch)
		}
		return "tarxz", []string{fmt.Sprintf("https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-%s-static.tar.xz", arch)}, nil
	case "darwin":
		return "evermeet", []string{
			"https://evermeet.cx/ffmpeg/getrelease/ffmpeg/zip",
			"https://evermeet.cx/ffmpeg/getrelease/ffprobe/zip",
		}, nil
	case "windows":
		return "gyan", []string{"https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"}, nil
	}
	return "", nil, fmt.Errorf("нет сборки ffmpeg под %s", goos)
}

// downloadFileURL скачивает url во временный файл и возвращает путь.
func downloadFileURL(url string) (string, error) {
	result, err := fetch(url)
	if err != nil {
		return "", fmt.Errorf("скачивание %s: %w", url, err)
	}
	defer result.Body.Close()
	tmp, err := os.CreateTemp("", "uci-dl-*")
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	if _, err := io.Copy(tmp, result.Body); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("скачивание %s: %w", url, err)
	}
	return tmp.Name(), nil
}

// installBinary кладёт исполняемый файл в dir под именем name с правами 0755.
func installBinary(from io.Reader, dir, name string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	tmp := path + ".part"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(out, from)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("запись %s: %v %v", path, copyErr, closeErr)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, nil
}

// exeName добавляет .exe на Windows.
func exeName(name string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(name, ".exe") {
		return name + ".exe"
	}
	return name
}

// zipEntries находит в zip файлы по базовым именам (без учёта вложенности).
func zipEntries(zipPath string, wanted ...string) (map[string]*zip.File, *zip.ReadCloser, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, nil, err
	}
	found := map[string]*zip.File{}
	for _, file := range reader.File {
		base := filepath.Base(strings.ReplaceAll(file.Name, "\\", "/"))
		for _, want := range wanted {
			if strings.EqualFold(base, want) {
				found[want] = file
			}
		}
	}
	return found, reader, nil
}

// installFromZip достаёт wanted-файлы из zip-архива url в dir.
func installFromZip(url, dir string, wanted ...string) error {
	archive, err := downloadFileURL(url)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	found, reader, err := zipEntries(archive, wanted...)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, want := range wanted {
		entry, ok := found[want]
		if !ok {
			return fmt.Errorf("в архиве %s нет %s", url, want)
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		_, copyErr := installBinary(source, dir, want)
		source.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

// installFromTarXZ достаёт wanted-файлы из tar.xz-архива url в dir.
func installFromTarXZ(url, dir string, wanted ...string) error {
	archive, err := downloadFileURL(url)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	xzReader, err := xz.NewReader(file)
	if err != nil {
		return fmt.Errorf("распаковка xz: %w", err)
	}
	remaining := map[string]bool{}
	for _, want := range wanted {
		remaining[want] = true
	}
	tarReader := tar.NewReader(xzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		base := filepath.Base(header.Name)
		if remaining[base] && header.Typeflag == tar.TypeReg {
			if _, err := installBinary(tarReader, dir, base); err != nil {
				return err
			}
			delete(remaining, base)
		}
	}
	if len(remaining) > 0 {
		missing := []string{}
		for name := range remaining {
			missing = append(missing, name)
		}
		return fmt.Errorf("в архиве %s не нашлось: %s", url, strings.Join(missing, ", "))
	}
	return nil
}

// installYtDlp — standalone-бинарь с GitHub-релизов.
func installYtDlp(dir string) error {
	asset, err := ytdlpAsset(runtime.GOOS)
	if err != nil {
		return err
	}
	tmp, err := downloadFileURL(githubAsset("yt-dlp/yt-dlp", asset))
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	file, err := os.Open(tmp)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = installBinary(file, dir, exeName("yt-dlp"))
	return err
}

// installDeno — zip с GitHub-релизов.
func installDeno(dir string) error {
	asset, err := denoAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	return installFromZip(githubAsset("denoland/deno", asset), dir, exeName("deno"))
}

// installFFmpeg — пакетный менеджер, если есть; иначе статическая сборка.
func installFFmpeg(dir string) error {
	if pm := packageManager(); pm != nil {
		fmt.Printf("  ставлю через %s…\n", pm.name)
		cmd := exec.Command(pm.command[0], pm.command[1:]...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err == nil {
			return nil
		}
		fmt.Println("  пакетный менеджер не справился, качаю статическую сборку")
	}
	kind, urls, err := ffmpegSource(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	switch kind {
	case "tarxz":
		return installFromTarXZ(urls[0], dir, "ffmpeg", "ffprobe")
	case "gyan":
		return installFromZip(urls[0], dir, "ffmpeg.exe", "ffprobe.exe")
	case "evermeet":
		for i, name := range []string{"ffmpeg", "ffprobe"} {
			if err := installFromZip(urls[i], dir, name); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("неизвестный источник ffmpeg: %s", kind)
}

type pmSpec struct {
	name    string
	command []string
}

// packageManager — доступный пакетный менеджер для ffmpeg.
func packageManager() *pmSpec {
	candidates := []pmSpec{
		{"apt", []string{"sudo", "apt-get", "install", "-y", "ffmpeg"}},
		{"brew", []string{"brew", "install", "ffmpeg"}},
		{"dnf", []string{"sudo", "dnf", "install", "-y", "ffmpeg"}},
		{"pacman", []string{"sudo", "pacman", "-S", "--noconfirm", "ffmpeg"}},
		{"winget", []string{"winget", "install", "--id", "Gyan.FFmpeg", "-e"}},
		{"choco", []string{"choco", "install", "ffmpeg", "-y"}},
	}
	for _, candidate := range candidates {
		if _, err := exec.LookPath(candidate.command[0]); err == nil {
			return &candidate
		}
	}
	return nil
}

// installGalleryDl — через pip (standalone-сборок под все ОС нет).
func installGalleryDl(dir string) error {
	python := lookTool("python3")
	if python == "" {
		python = lookTool("python")
	}
	if python == "" {
		return fmt.Errorf("gallery-dl ставится через pip, а Python не найден — установите Python 3 или поставьте gallery-dl вручную")
	}
	pipArgs := []string{"-m", "pip", "install", "--user", "-U", "gallery-dl"}
	if runtime.GOOS == "linux" {
		pipArgs = append(pipArgs, "--break-system-packages") // PEP 668: user-site, систему не ломает
	}
	cmd := exec.Command(python, pipArgs...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// installSingleFile — через npm.
func installSingleFile(dir string) error {
	npm := lookTool("npm")
	if npm == "" {
		return fmt.Errorf("single-file ставится через npm, а Node.js не найден — установите Node.js LTS (https://nodejs.org) и повторите")
	}
	args := []string{"install", "-g", "single-file-cli"}
	if runtime.GOOS != "windows" {
		args = append(args, "--prefix", filepath.Join(filepath.Dir(dir), ""))
		// prefix = ~/.local → бинарь попадёт в ~/.local/bin
	}
	cmd := exec.Command(npm, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
