package main

import (
	"net/url"
	"path"
	"strings"
)

// ContentType — что находится за ссылкой.
type ContentType string

const (
	TypeVideo    ContentType = "video"
	TypeDocument ContentType = "document"
	TypeImage    ContentType = "image"
	TypeImages   ContentType = "images" // альбом/галерея
	TypeArticle  ContentType = "article"
	TypeWebpage  ContentType = "webpage"
)

// documentExts — файлы, которые скачиваются как есть.
var documentExts = map[string]bool{
	"pdf": true, "doc": true, "docx": true, "xls": true, "xlsx": true,
	"ppt": true, "pptx": true, "odt": true, "ods": true, "odp": true,
	"zip": true, "tar": true, "gz": true, "tgz": true, "bz2": true, "7z": true, "rar": true,
	"epub": true, "mobi": true, "djvu": true,
	"txt": true, "csv": true, "md": true, "json": true, "xml": true,
	"mp3": true, "wav": true, "flac": true, "ogg": true, "m4a": true,
}

var imageExts = map[string]bool{
	"jpg": true, "jpeg": true, "png": true, "gif": true, "webp": true,
	"avif": true, "svg": true, "bmp": true, "ico": true, "tiff": true, "tif": true, "heic": true,
}

// videoHosts — хосты, которые сразу отправляем в yt-dlp без пробинга.
// yt-dlp поддерживает тысячи сайтов; здесь — явное распознавание популярных,
// остальное решает пробинг (не-HTML ответ или --type video).
var videoHosts = []string{
	"youtube.com", "youtu.be", "m.youtube.com", "music.youtube.com",
	"bilibili.com", "b23.tv", "vimeo.com", "twitch.tv", "tiktok.com",
	"rutube.ru", "dzen.ru", "zen.yandex.ru", "vkvideo.ru", "ok.ru",
	"twitter.com", "x.com", "facebook.com", "instagram.com",
	"dailymotion.com", "soundcloud.com", "bandcamp.com",
}

// gdriveID извлекает ID файла из ссылки Google Drive, если это она.
func gdriveID(u *url.URL) string {
	if u.Host != "drive.google.com" && u.Host != "docs.google.com" {
		return ""
	}
	// /file/d/<id>/view, /open?id=<id>, /uc?id=<id>
	parts := strings.Split(u.Path, "/")
	for i, part := range parts {
		if part == "d" && i+1 < len(parts) && parts[i+1] != "" {
			return parts[i+1]
		}
	}
	return u.Query().Get("id")
}

// classify определяет тип содержимого по самой ссылке, без сети.
// Возвращает ok=false, когда нужен пробинг Content-Type у сервера.
func classify(rawURL string) (ContentType, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", false
	}
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")

	if gdriveID(u) != "" {
		return TypeDocument, true
	}
	for _, vh := range videoHosts {
		if host == vh || strings.HasSuffix(host, "."+vh) {
			return TypeVideo, true
		}
	}
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(u.Path), "."))
	ext = strings.SplitN(ext, "?", 2)[0]
	if documentExts[ext] {
		return TypeDocument, true
	}
	if imageExts[ext] {
		return TypeImage, true
	}
	return "", false // вероятно HTML: пробуем Content-Type
}

// fromContentType доводит классификацию по заголовку ответа.
func fromContentType(contentType string) ContentType {
	ct := strings.ToLower(strings.SplitN(contentType, ";", 2)[0])
	switch {
	case strings.HasPrefix(ct, "video/"), strings.HasPrefix(ct, "audio/"):
		return TypeVideo
	case strings.HasPrefix(ct, "image/"):
		return TypeImage
	case ct == "text/html", ct == "application/xhtml+xml":
		return TypeArticle
	case ct == "":
		return TypeArticle
	default:
		return TypeDocument
	}
}
