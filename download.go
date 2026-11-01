package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Minute}

// fetchResult — ответ пробинга/скачивания.
type fetchResult struct {
	Body        io.ReadCloser
	ContentType string
	FileName    string // из Content-Disposition, если есть
	FinalURL    string
}

// fetch открывает URL с человеческим User-Agent (иначе многие сайты режут).
// Firefox-строка: упрощённые Chrome UA попадают в блок-листы (w3.org отдаёт 403).
func fetch(rawURL string) (*fetchResult, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("сервер ответил %s", resp.Status)
	}
	result := &fetchResult{
		Body:        resp.Body,
		ContentType: resp.Header.Get("Content-Type"),
		FinalURL:    resp.Request.URL.String(),
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			result.FileName = params["filename"]
		}
	}
	return result, nil
}

// probeContentType делает HEAD, а если сервер его не даёт — GET с обрывом после заголовков.
func probeContentType(rawURL string) string {
	req, err := http.NewRequest(http.MethodHead, rawURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", "Mozilla/5.0")
		client := &http.Client{Timeout: 20 * time.Second}
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return resp.Header.Get("Content-Type")
			}
		}
	}
	result, err := fetch(rawURL)
	if err != nil {
		return ""
	}
	result.Body.Close()
	return result.ContentType
}

// downloadFile сохраняет поток в файл внутри dir под именем name (уже безопасным).
func downloadFile(body io.Reader, dir, name string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := collisionFree(dir, name)
	tmp := path + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(out, body)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(tmp)
		return "", copyErr
	}
	if closeErr != nil {
		os.Remove(tmp)
		return "", closeErr
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, nil
}

// downloadDirect — прямая ссылка на документ/картинку.
func downloadDirect(rawURL, destDir string) ([]string, *Info, error) {
	result, err := fetch(rawURL)
	if err != nil {
		return nil, nil, err
	}
	defer result.Body.Close()
	name := result.FileName
	if name == "" {
		name = filepath.Base(result.finalPath())
	}
	if name == "." || name == "/" || name == "" {
		name = "download"
	}
	// Если у имени нет расширения, подставим по Content-Type.
	if filepath.Ext(name) == "" {
		if exts, _ := mime.ExtensionsByType(strings.SplitN(result.ContentType, ";", 2)[0]); len(exts) > 0 {
			name += exts[0]
		}
	}
	path, err := downloadFile(result.Body, destDir, safeName(name))
	if err != nil {
		return nil, nil, err
	}
	return []string{path}, &Info{SourceURL: rawURL, Type: TypeDocument}, nil
}

func (r *fetchResult) finalPath() string {
	if u, err := url.Parse(r.FinalURL); err == nil {
		return u.Path
	}
	return ""
}

// downloadGDrive — публичная ссылка Google Drive (с confirm-токеном для больших файлов).
func downloadGDrive(id, destDir string) ([]string, *Info, error) {
	source := "https://drive.google.com/file/d/" + id + "/view"
	downloadURL := "https://drive.usercontent.google.com/download?id=" + url.QueryEscape(id) + "&export=download&confirm=t"
	result, err := fetch(downloadURL)
	if err != nil {
		return nil, nil, err
	}
	defer result.Body.Close()
	ct := strings.ToLower(result.ContentType)
	if strings.HasPrefix(ct, "text/html") {
		return nil, nil, fmt.Errorf("Google Drive вернул страницу вместо файла (файл не публичный или требует вирус-проверки вручную)")
	}
	name := result.FileName
	if name == "" {
		name = id
		if exts, _ := mime.ExtensionsByType(strings.SplitN(result.ContentType, ";", 2)[0]); len(exts) > 0 {
			name += exts[0]
		}
	}
	path, err := downloadFile(result.Body, destDir, safeName(name))
	if err != nil {
		return nil, nil, err
	}
	return []string{path}, &Info{SourceURL: source, Type: TypeDocument}, nil
}
