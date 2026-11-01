package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"
)

func TestYtdlpAsset(t *testing.T) {
	want := map[string]string{"linux": "yt-dlp_linux", "darwin": "yt-dlp_macos", "windows": "yt-dlp.exe"}
	for goos, asset := range want {
		got, err := ytdlpAsset(goos)
		if err != nil || got != asset {
			t.Errorf("ytdlpAsset(%q) = %q, %v; хочу %q", goos, got, err, asset)
		}
	}
	if _, err := ytdlpAsset("plan9"); err == nil {
		t.Error("ytdlpAsset(plan9) должна вернуть ошибку")
	}
}

func TestDenoAsset(t *testing.T) {
	cases := map[string]string{
		"linux/amd64":   "deno-x86_64-unknown-linux-gnu.zip",
		"linux/arm64":   "deno-aarch64-unknown-linux-gnu.zip",
		"darwin/amd64":  "deno-x86_64-apple-darwin.zip",
		"darwin/arm64":  "deno-aarch64-apple-darwin.zip",
		"windows/amd64": "deno-x86_64-pc-windows-msvc.zip",
	}
	for key, want := range cases {
		parts := strings.Split(key, "/")
		got, err := denoAsset(parts[0], parts[1])
		if err != nil || got != want {
			t.Errorf("denoAsset(%s) = %q, %v; хочу %q", key, got, err, want)
		}
	}
	if _, err := denoAsset("windows", "arm64"); err == nil {
		t.Error("denoAsset(windows/arm64) должна вернуть ошибку")
	}
}

func TestFFmpegSource(t *testing.T) {
	kind, urls, err := ffmpegSource("linux", "amd64")
	if err != nil || kind != "tarxz" || len(urls) != 1 || !strings.Contains(urls[0], "amd64-static") {
		t.Errorf("linux/amd64: kind=%q urls=%v err=%v", kind, urls, err)
	}
	kind, urls, err = ffmpegSource("darwin", "arm64")
	if err != nil || kind != "evermeet" || len(urls) != 2 {
		t.Errorf("darwin: kind=%q urls=%v err=%v", kind, urls, err)
	}
	kind, urls, err = ffmpegSource("windows", "amd64")
	if err != nil || kind != "gyan" || len(urls) != 1 {
		t.Errorf("windows: kind=%q urls=%v err=%v", kind, urls, err)
	}
	if _, _, err := ffmpegSource("linux", "riscv64"); err == nil {
		t.Error("linux/riscv64 должна вернуть ошибку")
	}
}

func TestExeName(t *testing.T) {
	got := exeName("tool")
	if runtime.GOOS == "windows" {
		if got != "tool.exe" {
			t.Errorf("exeName на windows = %q", got)
		}
	} else if got != "tool" {
		t.Errorf("exeName = %q", got)
	}
	if got := exeName("tool.exe"); got != "tool.exe" {
		t.Errorf("exeName(tool.exe) = %q, не должно удваиваться", got)
	}
}

// serveFile поднимает тестовый HTTP-сервер, отдающий content.
func serveFile(t *testing.T, content []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	t.Cleanup(server.Close)
	return server
}

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	writer := zip.NewWriter(buf)
	for name, body := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeTarXZ(t *testing.T, files map[string]string) []byte {
	t.Helper()
	raw := &bytes.Buffer{}
	tarWriter := tar.NewWriter(raw)
	for name, body := range files {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tarWriter, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	xzWriter, err := xz.NewWriter(buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := xzWriter.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := xzWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestInstallFromZip(t *testing.T) {
	archive := makeZip(t, map[string]string{
		"bundle/bin/deno":    "deno-binary",
		"bundle/docs/readme": "ignore me",
		"bundle\\bin\\deno2": "backslash paths", // Windows-стиль путей внутри zip
	})
	server := serveFile(t, archive)
	dir := t.TempDir()
	if err := installFromZip(server.URL, dir, "deno"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "deno"))
	if err != nil || string(got) != "deno-binary" {
		t.Errorf("содержимое = %q, %v", got, err)
	}
	info, _ := os.Stat(filepath.Join(dir, "deno"))
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Error("бинарь должен быть исполняемым")
	}
	if err := installFromZip(server.URL, dir, "no-such-file"); err == nil {
		t.Error("отсутствующий файл должен дать ошибку")
	}
}

func TestInstallFromTarXZ(t *testing.T) {
	archive := makeTarXZ(t, map[string]string{
		"ffmpeg-7.0-static/ffmpeg":  "ffmpeg-binary",
		"ffmpeg-7.0-static/ffprobe": "ffprobe-binary",
		"ffmpeg-7.0-static/GPLv3":   "license",
	})
	server := serveFile(t, archive)
	dir := t.TempDir()
	if err := installFromTarXZ(server.URL, dir, "ffmpeg", "ffprobe"); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"ffmpeg": "ffmpeg-binary", "ffprobe": "ffprobe-binary"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v", name, got, err)
		}
	}
	if err := installFromTarXZ(server.URL, dir, "ghost"); err == nil {
		t.Error("отсутствующий файл должен дать ошибку")
	}
}

func TestInstallBinaryCollision(t *testing.T) {
	dir := t.TempDir()
	if _, err := installBinary(strings.NewReader("one"), dir, "tool"); err != nil {
		t.Fatal(err)
	}
	path, err := installBinary(strings.NewReader("two"), dir, "tool")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "two" {
		t.Errorf("перезапись не сработала: %q", got)
	}
}
