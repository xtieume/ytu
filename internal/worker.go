package internal

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Worker manages concurrent download slots and dispatches jobs.
type Worker struct {
	cond     *sync.Cond
	running  int
	maxConc  int
	queue    *JobQueue
	hub      *Hub
	history  *HistoryStore
	settings *SettingsStore
}

func NewWorker(maxConcurrent int, q *JobQueue, hub *Hub, history *HistoryStore, settings *SettingsStore) *Worker {
	return &Worker{
		cond:     sync.NewCond(&sync.Mutex{}),
		maxConc:  maxConcurrent,
		queue:    q,
		hub:      hub,
		history:  history,
		settings: settings,
	}
}

// ActiveCount returns number of currently running downloads.
func (w *Worker) ActiveCount() int {
	w.cond.L.Lock()
	defer w.cond.L.Unlock()
	return w.running
}

// MaxCount returns the current concurrency limit.
func (w *Worker) MaxCount() int {
	w.cond.L.Lock()
	defer w.cond.L.Unlock()
	return w.maxConc
}

// SetMaxConcurrent updates the concurrency limit at runtime.
// Running jobs continue; pending jobs re-evaluate immediately.
func (w *Worker) SetMaxConcurrent(n int) {
	if n < 1 {
		n = 1
	}
	w.cond.L.Lock()
	w.maxConc = n
	w.cond.L.Unlock()
	w.cond.Broadcast() // wake any goroutines waiting for a slot
}

func (w *Worker) acquire() {
	w.cond.L.Lock()
	for w.running >= w.maxConc {
		w.cond.Wait()
	}
	w.running++
	w.cond.L.Unlock()
}

func (w *Worker) release() {
	w.cond.L.Lock()
	w.running--
	w.cond.L.Unlock()
	w.cond.Broadcast()
}

// Submit starts a job goroutine; it waits until a slot is available.
func (w *Worker) Submit(job *Job) {
	go func() {
		w.acquire()
		defer func() {
			w.release()
			w.startNextPending()
		}()

		job.mu.Lock()
		if job.Status == StatusCancelled {
			job.mu.Unlock()
			return
		}
		job.Status = StatusRunning
		job.mu.Unlock()

		w.broadcast(job, map[string]any{"type": "status", "jobId": job.ID, "status": string(StatusRunning)})

		ctx, cancel := context.WithCancel(context.Background())
		job.SetCancel(cancel)
		defer cancel()

		cfg := w.settings.Get()
		outputPath, err := RunDownload(ctx, job, cfg.OutputDir, func(pct float64, speed, eta string) {
			job.mu.Lock()
			job.Progress = pct
			job.Speed = speed
			job.ETA = eta
			job.mu.Unlock()
			w.broadcast(job, map[string]any{
				"type":    "progress",
				"jobId":   job.ID,
				"percent": pct,
				"speed":   speed,
				"eta":     eta,
				"status":  string(StatusRunning),
			})
		})

		if err != nil {
			job.mu.Lock()
			if job.Status != StatusCancelled {
				job.Status = StatusError
				job.Error = err.Error()
			}
			job.mu.Unlock()
			w.broadcast(job, map[string]any{"type": "error", "jobId": job.ID, "message": err.Error(), "status": string(job.Status)})
			return
		}

		// Thumbnail embed for MP3
		if job.Format == "mp3" && outputPath != "" && job.ThumbnailURL != "" && cfg.EmbedThumbnail {
			if HasFFmpeg() {
				thumbPath := filepath.Join(os.TempDir(), fmt.Sprintf("ytu_thumb_%s.jpg", job.ID))
				if dlErr := DownloadThumbnail(job.ThumbnailURL, thumbPath); dlErr != nil {
					log.Printf("[warn] thumbnail download failed for %s: %v", job.ID, dlErr)
				} else {
					if embErr := EmbedThumbnail(outputPath, thumbPath); embErr != nil {
						log.Printf("[warn] thumbnail embed failed for %s: %v", job.ID, embErr)
					}
					_ = os.Remove(thumbPath)
				}
			} else {
				log.Printf("[warn] ffmpeg not found, skipping thumbnail embed for %s", job.ID)
			}
		}

		var fileSize int64
		if outputPath != "" {
			if fi, statErr := os.Stat(outputPath); statErr == nil {
				fileSize = fi.Size()
			}
		}

		job.mu.Lock()
		job.Status = StatusDone
		job.Progress = 100
		job.OutputPath = outputPath
		job.FileSize = fileSize
		job.mu.Unlock()

		w.broadcast(job, map[string]any{
			"type":       "done",
			"jobId":      job.ID,
			"outputPath": outputPath,
			"fileSize":   fileSize,
			"status":     string(StatusDone),
		})

		_ = w.history.Append(HistoryEntry{
			ID:           job.ID,
			URL:          job.URL,
			Title:        job.Title,
			Format:       job.Format,
			Quality:      job.Quality,
			OutputPath:   outputPath,
			FileSize:     fileSize,
			ThumbnailURL: job.ThumbnailURL,
			CompletedAt:  time.Now(),
		})
	}()
}

func (w *Worker) startNextPending() {
	if next := w.queue.NextPending(); next != nil {
		w.Submit(next)
	}
}

func (w *Worker) broadcast(job *Job, msg map[string]any) {
	w.hub.Broadcast(job.ID, msg)
}
