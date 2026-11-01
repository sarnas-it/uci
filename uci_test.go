package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyKnownPlatforms(t *testing.T) {
	cases := map[string]ContentType{
		"https://www.youtube.com/watch?v=abc123":   TypeVideo,
		"https://youtu.be/abc123":                  TypeVideo,
		"https://rutube.ru/video/abc/":             TypeVideo,
		"https://www.bilibili.com/video/BV1xx":     TypeVideo,
		"https://example.com/report.pdf":           TypeDocument,
		"https://example.com/data.XLSX":            TypeDocument,
		"https://example.com/pic.png":              TypeImage,
		"https://example.com/photo.JPEG?size=big":  TypeImage,
		"https://drive.google.com/file/d/XYZ/view": TypeDocument,
	}
	for raw, want := range cases {
		got, ok := classify(raw)
		if !ok || got != want {
			t.Errorf("classify(%q) = %q, %v; want %q, true", raw, got, ok, want)
		}
	}
}

func TestClassifyUnknownNeedsProbe(t *testing.T) {
	for _, raw := range []string{
		"https://example.com/blog/post-1",
		"https://en.wikipedia.org/wiki/Linux",
		"https://example.com/",
	} {
		if got, ok := classify(raw); ok {
			t.Errorf("classify(%q) = %q, true; want probe", raw, got)
		}
	}
}

func TestFromContentType(t *testing.T) {
	cases := map[string]ContentType{
		"text/html; charset=utf-8": TypeArticle,
		"application/pdf":          TypeDocument,
		"image/webp":               TypeImage,
		"video/mp4":                TypeVideo,
		"application/zip":          TypeDocument,
		"application/octet-stream": TypeDocument,
		"":                         TypeArticle,
		"application/xhtml+xml":    TypeArticle,
		"audio/mpeg":               TypeVideo,
		"text/plain":               TypeDocument,
	}
	for ct, want := range cases {
		if got := fromContentType(ct); got != want {
			t.Errorf("fromContentType(%q) = %q; want %q", ct, got, want)
		}
	}
}

func TestGdriveID(t *testing.T) {
	cases := map[string]string{
		"https://drive.google.com/file/d/1AbC_dEf/view?usp=sharing": "1AbC_dEf",
		"https://drive.google.com/open?id=1AbC_dEf":                 "1AbC_dEf",
		"https://drive.google.com/uc?id=1AbC_dEf&export=download":   "1AbC_dEf",
		"https://example.com/file/d/xxx":                            "",
	}
	for raw, want := range cases {
		u, _ := url.Parse(raw)
		if got := gdriveID(u); got != want {
			t.Errorf("gdriveID(%q) = %q; want %q", raw, got, want)
		}
	}
}

func TestSafeName(t *testing.T) {
	cases := map[string]string{
		"normal name.pdf":         "normal name.pdf",
		`a/b\c:d*e?f"g<h>i|j.pdf`: "abcdefghij.pdf",
		"CON":                     "_CON",
		"nul.txt":                 "_nul.txt",
		"  spaced  out  .pdf ":    "spaced out .pdf",
		"..":                      "download",
		"":                        "download",
	}
	for in, want := range cases {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q; want %q", in, got, want)
		}
	}
	long := strings.Repeat("ё", 300) + ".pdf"
	if got := safeName(long); len(got) > 200 || !strings.HasSuffix(got, ".pdf") {
		t.Errorf("safeName(dлинное) = %d байт, конец %q; want ≤200 байт и .pdf", len(got), got[len(got)-4:])
	}
}

func TestCollisionFree(t *testing.T) {
	dir := t.TempDir()
	first := collisionFree(dir, "a.pdf")
	if filepath.Base(first) != "a.pdf" {
		t.Fatalf("collisionFree на пустом = %q; want a.pdf", first)
	}
	os.WriteFile(first, []byte("1"), 0o644)
	second := collisionFree(dir, "a.pdf")
	if filepath.Base(second) != "a-2.pdf" {
		t.Fatalf("collisionFree при занятом = %q; want a-2.pdf", second)
	}
	os.WriteFile(second, []byte("2"), 0o644)
	if third := collisionFree(dir, "a.pdf"); filepath.Base(third) != "a-3.pdf" {
		t.Fatalf("collisionFree при двух занятых = %q; want a-3.pdf", third)
	}
}

func TestFormatSelector(t *testing.T) {
	for _, q := range []string{"", "1080p", "720p", "1440p", "4k", "highest", "что-угодно"} {
		if got := formatSelector(q, true); got == "" {
			t.Errorf("formatSelector(%q) пуст", q)
		}
	}
	if got := formatSelector("720p", true); !strings.Contains(got, "<=720") {
		t.Errorf("formatSelector(720p) = %q; want лимит <=720", got)
	}
}

func TestWriteInfoSidecar(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "video.mp4")
	os.WriteFile(file, []byte("x"), 0o644)
	info := &Info{Type: TypeVideo, SourceURL: "https://youtu.be/abc", Title: "T", Author: "A"}
	if err := writeInfoSidecar([]string{file}, info); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "video.info.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"video", "https://youtu.be/abc", "T", "A"} {
		if !strings.Contains(text, want) {
			t.Errorf("в info-файле нет %q:\n%s", want, text)
		}
	}
}

func TestXdgUserDir(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// os.UserHomeDir: на Linux читает HOME, на Windows — USERPROFILE.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	os.WriteFile(filepath.Join(dir, "user-dirs.dirs"),
		[]byte("XDG_DESKTOP_DIR=\"$HOME/Desktop\"\nXDG_DOWNLOAD_DIR=\"$HOME/Загрузки\"\n"), 0o644)
	// xdgUserDir подставляет $HOME строкой, слэши остаются прямыми на всех ОС.
	if got, want := xdgUserDir("DOWNLOAD"), home+"/Загрузки"; got != want {
		t.Errorf("xdgUserDir(DOWNLOAD) = %q; want %q", got, want)
	}
	if got := xdgUserDir("MUSIC"); got != "" {
		t.Errorf("xdgUserDir(MUSIC) = %q; want пусто", got)
	}
}

func TestFlagsFirst(t *testing.T) {
	got := flagsFirst([]string{"https://x.y/a.pdf", "--to", "/tmp/out", "--quality=720p"})
	want := []string{"--to", "/tmp/out", "--quality=720p", "https://x.y/a.pdf"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("flagsFirst = %v; want %v", got, want)
	}
}
