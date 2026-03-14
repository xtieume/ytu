# ytu

![GitHub stars](https://img.shields.io/github/stars/USER/ytu?style=flat-square)
![GitHub release](https://img.shields.io/github/v/release/USER/ytu?style=flat-square)
![Go version](https://img.shields.io/badge/go-1.21+-blue?style=flat-square)
![License](https://img.shields.io/badge/license-MIT-green?style=flat-square)
![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey?style=flat-square)

A minimal web UI for downloading YouTube videos and music via `yt-dlp`, packaged as a single Go binary.

![screenshot](https://i.imgur.com/placeholder.png)

## Features

- Download **MP3** (128 / 192 / 320 kbps) and **MP4 / WEBM** (720p / 1080p / best)
- Playlist support — browse, select tracks, download in bulk
- Concurrent downloads with live progress (WebSocket)
- Embed thumbnail into MP3 (requires ffmpeg)
- Download history
- Multi-language UI (English / Tiếng Việt)
- Settings auto-save, log rotation with configurable retention
- Single binary — no Node.js, no Docker

## Requirements

| Tool | Install |
|------|---------|
| `yt-dlp` | `pip install yt-dlp` or `brew install yt-dlp` · [Releases](https://github.com/yt-dlp/yt-dlp/releases) |
| `ffmpeg` | `brew install ffmpeg` · [Releases](https://github.com/FFmpeg/FFmpeg/releases) *(optional — thumbnail embedding only)* |

## Quick start

### Download pre-built binary

Grab the latest release from [Releases](https://github.com/USER/ytu/releases), then:

```bash
chmod +x ytu
./ytu
```

### Build from source

```bash
git clone https://github.com/USER/ytu.git
cd ytu
go build -o ytu .
./ytu
```

Browser opens automatically at `http://localhost:8080`.

## Options

```
./ytu [flags]

  -port         HTTP port (default: 8080)
  -output       Download directory (default: ~/Downloads/ytu)
  -concurrency  Max concurrent downloads (default: 3)
  -no-browser   Don't open browser on start
```

## License

MIT
