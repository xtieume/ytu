package internal

import (
	"context"
	"sync"
	"time"
)

type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusRunning   JobStatus = "running"
	StatusDone      JobStatus = "done"
	StatusError     JobStatus = "error"
	StatusCancelled JobStatus = "cancelled"
)

type Job struct {
	ID           string    `json:"id"`
	URL          string    `json:"url"`
	Title        string    `json:"title"`
	Format       string    `json:"format"`  // mp3, mp4, webm
	Quality      string    `json:"quality"` // best, 320k, 192k, 128k, 1080p, 720p
	Status       JobStatus `json:"status"`
	Progress     float64   `json:"progress"`
	Speed        string    `json:"speed"`
	ETA          string    `json:"eta"`
	OutputPath   string    `json:"outputPath"`
	FileSize     int64     `json:"fileSize"`
	ThumbnailURL string    `json:"thumbnailUrl"`
	CreatedAt    time.Time `json:"createdAt"`
	Error        string    `json:"error,omitempty"`

	cancel context.CancelFunc
	mu     sync.Mutex
}

func (j *Job) SetCancel(fn context.CancelFunc) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.cancel = fn
}

func (j *Job) Cancel() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.cancel != nil {
		j.cancel()
	}
}

type JobQueue struct {
	mu   sync.Mutex
	jobs []*Job
}

func NewJobQueue() *JobQueue {
	return &JobQueue{}
}

func (q *JobQueue) Enqueue(job *Job) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, job)
}

func (q *JobQueue) GetAll() []*Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	result := make([]*Job, len(q.jobs))
	copy(result, q.jobs)
	return result
}

func (q *JobQueue) GetByID(id string) *Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range q.jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}

func (q *JobQueue) Cancel(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range q.jobs {
		if j.ID == id {
			j.mu.Lock()
			st := j.Status
			j.mu.Unlock()
			if st == StatusPending || st == StatusRunning {
				j.Cancel()
				if st == StatusPending {
					j.mu.Lock()
					j.Status = StatusCancelled
					j.mu.Unlock()
				}
				return true
			}
			return false
		}
	}
	return false
}

// IsDuplicate returns true if a pending/running job with same URL exists.
func (q *JobQueue) IsDuplicate(url string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range q.jobs {
		j.mu.Lock()
		st := j.Status
		j.mu.Unlock()
		if j.URL == url && (st == StatusPending || st == StatusRunning) {
			return true
		}
	}
	return false
}

// NextPending returns the first pending job, or nil.
func (q *JobQueue) NextPending() *Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range q.jobs {
		j.mu.Lock()
		st := j.Status
		j.mu.Unlock()
		if st == StatusPending {
			return j
		}
	}
	return nil
}
