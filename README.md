# Universal Content Intake

**Universal Content Intake** (команда `uci`) — кроссплатформенный загрузчик контента:
одна ссылка → скачанное содержимое: видео, документ, картинки, статья или страница целиком.
Результат складывается в папку загрузок, рядом кладётся короткий `*.info.md`
(источник, автор, исходная ссылка).

Это самостоятельная Go-реализация ядра-загрузчика проекта
[YIAN6557/universal-content-intake](https://github.com/YIAN6557/universal-content-intake),
собранная в статический бинарь под Linux / macOS / Windows (у оригинала — Python,
только macOS и Windows). Автомониторинг, субтитры и облачная часть оригинала
**не входят** — это только загрузчик.

## Установка

Готовые бинари — на странице [релизов](https://github.com/sarnas-it/uci/releases):

```bash
# Linux (amd64)
curl -L https://github.com/sarnas-it/uci/releases/download/v0.1.0/uci-linux-amd64.tar.gz | tar xz
sudo install uci-linux-amd64 /usr/local/bin/uci
```

## Использование

```bash
uci <ссылка>                        # тип определяется сам
uci --type video <ссылка>           # или принудительно: video|document|image|images|article|webpage
uci "https://example.com/report.pdf" --to ~/docs
uci --quality 720p <ссылка-на-видео>
uci doctor                          # проверка окружения, предложит поставить недостающее
uci doctor --yes                    # то же, но ставит всё недостающее без вопросов
```

Как определяется тип, если не задан `--type`:

1. известные платформы (YouTube, Bilibili, Rutube, Vimeo, TikTok, VK и др.) → видео;
2. Google Drive → документ;
3. расширение файла в URL → документ или картинка;
4. иначе — пробинг `Content-Type` у сервера; HTML → статья (readability → Markdown).

## Движки-зависимости

Бинарь самодостаточен для документов, картинок, статей и простых страниц.
Для остального нужны внешние инструменты — **`uci doctor --yes` скачает и поставит
недостающие сам**, без sudo, в `~/.local/bin` (на Windows — в папку данных uci):

| Инструмент | Зачем | Как ставится автоматически |
|---|---|---|
| yt-dlp | видео с тысяч сайтов | статический бинарь с GitHub-релизов |
| ffmpeg | склейка видео+аудио (1080p+) | пакетный менеджер (apt/brew/winget/…), иначе статическая сборка |
| deno | JS-рантайм yt-dlp для YouTube | zip с GitHub-релизов |
| gallery-dl | альбомы и галереи картинок | `pip install --user` (нужен Python 3) |
| SingleFile | снимок страницы целиком | `npm install -g` (нужен Node.js) |
| Chrome | движок для SingleFile | вручную — doctor подскажет команду для вашей ОС |

Если `pip`/`npm` нет, doctor честно скажет, чего не хватает, и покажет ручную команду.
Без ffmpeg видео скачивается одиночным форматом (без склейки) — частичные файлы
не остаются. Качество по умолчанию 1080p, исходник с меньшим разрешением не апскейлится.

## Сборка

```bash
make test     # go test ./...
make build    # статические бинари в bin/: linux (amd64/arm64), darwin (amd64/arm64), windows (amd64)
```

CGO отключён, бинари статические — копируются на целевую машину как есть.

## Платформенные различия

Вся платформенная логика — в `platform.go` (папка загрузок с учётом
`XDG_DOWNLOAD_DIR`, поиск инструментов вне `PATH`, подсказки по установке)
и `installer.go` (автоустановка: ассеты GitHub-релизов, статические сборки
ffmpeg под linux/darwin/windows, pip/npm для gallery-dl и SingleFile).

## Ограничения порта

- нет автомониторинга авторов (Google Apps Script + очередь) — только ручные скачивания;
- нет субтитров/перевода (whisper.cpp, Apple Translation);
- Google Drive — только публичные файлы (без OAuth).
