package internal

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// VideoMeta holds metadata fetched from yt-dlp.
type VideoMeta struct {
	Title        string `json:"title"`
	ThumbnailURL string `json:"thumbnailUrl"`
	Duration     int    `json:"duration"`
	ID           string `json:"id"`
	WebpageURL   string `json:"webpageUrl"`
}

type ytdlpMeta struct {
	Title      string `json:"title"`
	Thumbnail  string `json:"thumbnail"`
	Duration   int    `json:"duration"`
	ID         string `json:"id"`
	WebpageURL string `json:"webpage_url"`
	Thumbnails []struct {
		URL        string `json:"url"`
		Preference int    `json:"preference"`
	} `json:"thumbnails"`
}

// FetchMetadata calls yt-dlp --dump-json to get video metadata.
func FetchMetadata(url string) (*VideoMeta, error) {
	out, err := exec.Command("yt-dlp", "--dump-json", "--no-playlist", url).Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp metadata failed: %w", err)
	}
	var meta ytdlpMeta
	if err := json.Unmarshal(out, &meta); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}

	// Use yt-dlp's pre-selected thumbnail (already validated as available).
	// Iterating thumbnails by preference can land on maxresdefault which returns 404.
	thumb := meta.Thumbnail

	return &VideoMeta{
		Title:        meta.Title,
		ThumbnailURL: thumb,
		Duration:     meta.Duration,
		ID:           meta.ID,
		WebpageURL:   meta.WebpageURL,
	}, nil
}

// PlaylistItem is one item from a flat playlist dump.
type PlaylistItem struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Duration int    `json:"duration"`
	Thumb    string `json:"thumbnail"`
}

// FetchPlaylist calls yt-dlp --flat-playlist to list videos.
func FetchPlaylist(url string) ([]PlaylistItem, error) {
	cmd := exec.Command("yt-dlp", "--flat-playlist", "--yes-playlist", "--dump-json", url)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp playlist failed: %w", err)
	}

	items := make([]PlaylistItem, 0)
	idx := 1
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var entry struct {
			ID         string  `json:"id"`
			URL        string  `json:"url"`
			Title      string  `json:"title"`
			Duration   float64 `json:"duration"` // yt-dlp outputs 339.0 not 339
			Thumb      string  `json:"thumbnail"`
			Thumbnails []struct {
				URL    string `json:"url"`
				Width  int    `json:"width"`
				Height int    `json:"height"`
			} `json:"thumbnails"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		itemURL := entry.URL
		if itemURL == "" {
			itemURL = "https://www.youtube.com/watch?v=" + entry.ID
		}
		// flat-playlist entries may only have thumbnails[] not thumbnail
		thumb := entry.Thumb
		if thumb == "" {
			for _, t := range entry.Thumbnails {
				if thumb == "" || (t.Width > 0 && t.Height > 0) {
					thumb = t.URL
				}
			}
		}
		items = append(items, PlaylistItem{
			Index:    idx,
			ID:       entry.ID,
			URL:      itemURL,
			Title:    entry.Title,
			Duration: int(entry.Duration),
			Thumb:    thumb,
		})
		idx++
	}
	return items, nil
}

// BuildYtdlpArgs builds the yt-dlp argument list from a job.
func BuildYtdlpArgs(job *Job, outputDir string) []string {
	args := []string{
		"--no-playlist",
		"--newline",
		"-o", outputDir + "/%(title)s.%(ext)s",
	}

	switch job.Format {
	case "mp3":
		args = append(args, "--extract-audio", "--audio-format", "mp3")
		switch job.Quality {
		case "320k":
			args = append(args, "--audio-quality", "0")
		case "192k":
			args = append(args, "--audio-quality", "192K")
		default: // 128k
			args = append(args, "--audio-quality", "128K")
		}
	case "mp4":
		switch job.Quality {
		case "1080p":
			args = append(args, "-f", "bestvideo[height<=1080][ext=mp4]+bestaudio[ext=m4a]/best[height<=1080][ext=mp4]/best")
		case "720p":
			args = append(args, "-f", "bestvideo[height<=720][ext=mp4]+bestaudio[ext=m4a]/best[height<=720][ext=mp4]/best")
		default:
			args = append(args, "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best")
		}
	case "webm":
		switch job.Quality {
		case "1080p":
			args = append(args, "-f", "bestvideo[height<=1080][ext=webm]+bestaudio[ext=webm]/best[height<=1080]")
		case "720p":
			args = append(args, "-f", "bestvideo[height<=720][ext=webm]+bestaudio[ext=webm]/best[height<=720]")
		default:
			args = append(args, "-f", "bestvideo[ext=webm]+bestaudio[ext=webm]/best")
		}
	}

	args = append(args, job.URL)
	return args
}

// progressRe matches yt-dlp progress lines.
var progressRe = regexp.MustCompile(`\[download\]\s+([\d.]+)%\s+of\s+[\d.~]+\w+\s+at\s+([\S]+)\s+ETA\s+(\S+)`)

// ParseProgress extracts percent/speed/ETA from a yt-dlp output line.
func ParseProgress(line string) (percent float64, speed, eta string, ok bool) {
	m := progressRe.FindStringSubmatch(line)
	if m == nil {
		return
	}
	p, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return
	}
	return p, m[2], m[3], true
}

// outputFileRe matches the "Destination:" or "[ExtractAudio]" output lines.
var destRe = regexp.MustCompile(`(?:Destination|Merging formats into):\s+(.+)`)
var extractRe = regexp.MustCompile(`\[ExtractAudio\] Destination:\s+(.+)`)

// RunDownload runs yt-dlp for the given job, calling progressFn on each update.
// Returns the output file path on success.
func RunDownload(ctx context.Context, job *Job, outputDir string, progressFn func(percent float64, speed, eta string)) (string, error) {
	args := BuildYtdlpArgs(job, outputDir)
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)

	// Store cancel reference via context already handles kill,
	// but we also store cmd for direct kill if needed.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start yt-dlp: %w", err)
	}

	var outputPath string
	var stderrLines []string

	// Read stderr in background
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			stderrLines = append(stderrLines, scanner.Text())
		}
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if p, sp, e, ok := ParseProgress(line); ok {
			progressFn(p, sp, e)
		}
		// Capture output file path
		if m := extractRe.FindStringSubmatch(line); m != nil {
			outputPath = strings.TrimSpace(m[1])
		} else if m := destRe.FindStringSubmatch(line); m != nil {
			outputPath = strings.TrimSpace(m[1])
		}
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("cancelled")
		}
		return "", fmt.Errorf("yt-dlp error: %w — %s", err, strings.Join(stderrLines, "; "))
	}
	return outputPath, nil
}
