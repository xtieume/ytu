package internal

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Server holds all dependencies for HTTP handlers.
type Server struct {
	Queue      *JobQueue
	Worker     *Worker
	Hub        *Hub
	History    *HistoryStore
	Settings   *SettingsStore
	LogManager *LogManager
	Port       int
}

// GET /api/settings
func (s *Server) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	cfg := s.Settings.Get()
	writeJSON(w, http.StatusOK, SettingsResponse{
		Settings: cfg,
		ServerInfo: ServerInfo{
			Port:      s.Port,
			HasFFmpeg: HasFFmpeg(),
			HasYtdlp:  HasYtdlp(),
			ActiveDL:  s.Worker.ActiveCount(),
			MaxDL:     s.Worker.MaxCount(),
		},
	})
}

// POST /api/settings
func (s *Server) HandleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var patch SettingsPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	old := s.Settings.Get()
	if err := s.Settings.Patch(patch); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg := s.Settings.Get()

	// Apply concurrency change immediately
	if cfg.MaxConcurrent != old.MaxConcurrent {
		s.Worker.SetMaxConcurrent(cfg.MaxConcurrent)
	}

	// Ensure output dir exists
	if cfg.OutputDir != "" {
		_ = mkdirAll(cfg.OutputDir)
	}

	// Switch log directory if changed
	if s.LogManager != nil && (cfg.LogDir != old.LogDir || cfg.LogRetentionDays != old.LogRetentionDays) {
		if err := s.LogManager.SetDir(cfg.LogDir, cfg.LogRetentionDays); err != nil {
			log.Printf("[warn] log dir switch failed: %v", err)
		}
	}

	s.HandleGetSettings(w, r)
}

func mkdirAll(p string) error {
	return os.MkdirAll(p, 0o755)
}

// GET /api/pick-dir — opens native OS folder picker, returns selected path.
func (s *Server) HandlePickDir(w http.ResponseWriter, r *http.Request) {
	path, err := pickDirectory()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// POST /api/info
func (s *Server) HandleInfo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		writeError(w, http.StatusBadRequest, "url required")
		return
	}
	meta, err := FetchMetadata(req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, meta)
}

// POST /api/playlist
func (s *Server) HandlePlaylist(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		writeError(w, http.StatusBadRequest, "url required")
		return
	}
	items, err := FetchPlaylist(req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// POST /api/download
func (s *Server) HandleDownload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL     string `json:"url"`
		Format  string `json:"format"`
		Quality string `json:"quality"`
		Title   string `json:"title"`
		Thumb   string `json:"thumbnailUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url required")
		return
	}
	if req.Format == "" {
		req.Format = "mp3"
	}
	if req.Quality == "" {
		req.Quality = "best"
	}
	if s.Queue.IsDuplicate(req.URL) {
		writeError(w, http.StatusConflict, "URL already in queue")
		return
	}

	job := &Job{
		ID:           newID(),
		URL:          req.URL,
		Title:        req.Title,
		Format:       req.Format,
		Quality:      req.Quality,
		ThumbnailURL: req.Thumb,
		Status:       StatusPending,
		CreatedAt:    time.Now(),
	}
	s.Queue.Enqueue(job)
	s.Worker.Submit(job)
	writeJSON(w, http.StatusCreated, job)
}

// GET /api/jobs
func (s *Server) HandleJobs(w http.ResponseWriter, r *http.Request) {
	jobs := s.Queue.GetAll()
	// Return newest first
	result := make([]*Job, len(jobs))
	for i, j := range jobs {
		result[len(jobs)-1-i] = j
	}
	writeJSON(w, http.StatusOK, result)
}

// DELETE /api/jobs/{id}
func (s *Server) HandleCancelJob(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}
	if !s.Queue.Cancel(id) {
		writeError(w, http.StatusNotFound, "job not found or already finished")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/history
func (s *Server) HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.History.GetAll())
}

// DELETE /api/history
func (s *Server) HandleClearHistory(w http.ResponseWriter, r *http.Request) {
	if err := s.History.Clear(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /ws/{jobId}
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimPrefix(r.URL.Path, "/ws/")
	if jobID == "" {
		http.Error(w, "jobId required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.Hub.Register(jobID, conn)

	// Send current job state immediately
	if job := s.Queue.GetByID(jobID); job != nil {
		job.mu.Lock()
		_ = conn.WriteJSON(map[string]any{
			"type":    "state",
			"jobId":   job.ID,
			"status":  string(job.Status),
			"percent": job.Progress,
			"speed":   job.Speed,
			"eta":     job.ETA,
		})
		job.mu.Unlock()
	}

	// Keep connection alive, unregister on client disconnect
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			s.Hub.Unregister(jobID, conn)
			return
		}
	}
}

// newID generates a simple unique ID.
func newID() string {
	return strings.ReplaceAll(time.Now().Format("20060102150405.000000000"), ".", "")
}
