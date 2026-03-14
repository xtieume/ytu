package internal

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

// DownloadThumbnail downloads a thumbnail URL to destPath.
func DownloadThumbnail(url, destPath string) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return fmt.Errorf("fetch thumbnail: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("thumbnail HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// EmbedThumbnail uses ffmpeg to embed a thumbnail image into an MP3 file.
// The original file is replaced with the new one containing cover art.
func EmbedThumbnail(mp3Path, thumbPath string) error {
	dir := filepath.Dir(mp3Path)
	base := filepath.Base(mp3Path)
	tmp := filepath.Join(dir, "._thumb_"+base)

	cmd := exec.Command("ffmpeg", "-y",
		"-i", mp3Path,
		"-i", thumbPath,
		"-map", "0",
		"-map", "1",
		"-c", "copy",
		"-id3v2_version", "3",
		"-metadata:s:v", "title=Album cover",
		"-metadata:s:v", "comment=Cover (front)",
		tmp,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("ffmpeg embed: %w — %s", err, string(out))
	}
	return os.Rename(tmp, mp3Path)
}

// HasFFmpeg returns true if ffmpeg is available in PATH.
func HasFFmpeg() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// HasYtdlp returns true if yt-dlp is available in PATH.
func HasYtdlp() bool {
	_, err := exec.LookPath("yt-dlp")
	return err == nil
}
