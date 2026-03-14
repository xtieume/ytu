package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"ytu/internal"
)

//go:embed all:static
var staticFiles embed.FS

func main() {
	port := flag.Int("port", 8080, "HTTP server port")
	outputDir := flag.String("output", defaultOutputDir(), "Download output directory")
	maxConc := flag.Int("concurrency", 3, "Max concurrent downloads")
	noBrowser := flag.Bool("no-browser", false, "Do not open browser automatically")
	flag.Parse()

	// Settings live next to the binary (resolved, not CWD)
	configDir := "."
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		configDir = filepath.Dir(exe)
	}

	// Dependency checks
	if !internal.HasYtdlp() {
		log.Println("[warn] yt-dlp not found in PATH. Install: pip install yt-dlp")
	}
	if !internal.HasFFmpeg() {
		log.Println("[warn] ffmpeg not found in PATH. Thumbnail embedding will be skipped.")
	}

	// Ensure output directory exists
	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		log.Fatalf("cannot create output dir %s: %v", *outputDir, err)
	}

	// History lives in ~/.config/ytu/ (persistent across reinstalls)
	home, _ := os.UserHomeDir()
	historyDir := filepath.Join(home, ".config", "ytu")

	history, err := internal.NewHistoryStore(historyDir)
	if err != nil {
		log.Fatalf("history store: %v", err)
	}

	defaultLogDir, _ := filepath.Abs(filepath.Join(configDir, "logs"))
	settings, err := internal.NewSettingsStore(configDir, internal.Settings{
		OutputDir:        *outputDir,
		MaxConcurrent:    *maxConc,
		DefaultFormat:    "mp3",
		DefaultQuality:   "320k",
		EmbedThumbnail:   true,
		Language:         "en",
		LogDir:           defaultLogDir,
		LogRetentionDays: 15,
	})
	if err != nil {
		log.Fatalf("settings store: %v", err)
	}

	// Start daily-rotating logger
	cfg := settings.Get()
	logMgr, err := internal.NewLogManager(cfg.LogDir, cfg.LogRetentionDays)
	if err != nil {
		log.Printf("[warn] cannot initialize log manager: %v", err)
	} else {
		log.SetOutput(io.MultiWriter(os.Stderr, logMgr))
		defer logMgr.Close()
	}

	if err := os.MkdirAll(settings.Get().OutputDir, 0o755); err != nil {
		log.Printf("[warn] cannot create output dir: %v", err)
	}

	queue := internal.NewJobQueue()
	hub := internal.NewHub()
	worker := internal.NewWorker(*maxConc, queue, hub, history, settings)

	srv := &internal.Server{
		Queue:      queue,
		Worker:     worker,
		Hub:        hub,
		History:    history,
		Settings:   settings,
		LogManager: logMgr,
		Port:       *port,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/pick-dir", method("GET", srv.HandlePickDir))
	mux.HandleFunc("/api/info", method("POST", srv.HandleInfo))
	mux.HandleFunc("/api/playlist", method("POST", srv.HandlePlaylist))
	mux.HandleFunc("/api/download", method("POST", srv.HandleDownload))
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			srv.HandleJobs(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/jobs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			srv.HandleCancelJob(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/history", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			srv.HandleGetHistory(w, r)
		case http.MethodDelete:
			srv.HandleClearHistory(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			srv.HandleGetSettings(w, r)
		case http.MethodPost:
			srv.HandleUpdateSettings(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/ws/", srv.HandleWebSocket)

	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	addr := fmt.Sprintf(":%d", *port)
	httpServer := &http.Server{Addr: addr, Handler: mux}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		url := fmt.Sprintf("http://localhost:%d", *port)
		log.Printf("ytu listening on %s  (output: %s)", url, settings.Get().OutputDir)
		if !*noBrowser {
			time.Sleep(200 * time.Millisecond)
			openBrowser(url)
		}
	}()

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-stop
	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	log.Println("bye")
}

func method(m string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != m {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h(w, r)
	}
}

func defaultOutputDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads", "ytu")
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	_ = exec.Command(cmd, args...).Start()
}
