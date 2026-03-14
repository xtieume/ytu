package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// rotatingLogger writes to a daily log file and purges files older than retentionDays.
type rotatingLogger struct {
	dir           string
	retentionDays int
	currentDate   string
	file          *os.File
}

func newRotatingLogger(dir string, retentionDays int) (*rotatingLogger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	rl := &rotatingLogger{dir: dir, retentionDays: retentionDays}
	if err := rl.rotate(); err != nil {
		return nil, err
	}
	rl.purgeOld()
	return rl, nil
}

func (rl *rotatingLogger) Write(p []byte) (int, error) {
	today := time.Now().Format("2006-01-02")
	if today != rl.currentDate {
		if err := rl.rotate(); err != nil {
			return 0, err
		}
		rl.purgeOld()
	}
	return rl.file.Write(p)
}

func (rl *rotatingLogger) Close() error {
	if rl.file != nil {
		return rl.file.Close()
	}
	return nil
}

func (rl *rotatingLogger) rotate() error {
	today := time.Now().Format("2006-01-02")
	path := filepath.Join(rl.dir, fmt.Sprintf("ytu-%s.log", today))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if rl.file != nil {
		_ = rl.file.Close()
	}
	rl.file = f
	rl.currentDate = today
	return nil
}

func (rl *rotatingLogger) purgeOld() {
	if rl.retentionDays <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -rl.retentionDays)
	entries, err := os.ReadDir(rl.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "ytu-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		dateStr := strings.TrimSuffix(strings.TrimPrefix(name, "ytu-"), ".log")
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			_ = os.Remove(filepath.Join(rl.dir, name))
		}
	}
}

// LogManager is a thread-safe daily-rotating logger that can switch directories at runtime.
type LogManager struct {
	mu sync.Mutex
	rl *rotatingLogger
}

func NewLogManager(dir string, retentionDays int) (*LogManager, error) {
	rl, err := newRotatingLogger(dir, retentionDays)
	if err != nil {
		return nil, err
	}
	return &LogManager{rl: rl}, nil
}

func (lm *LogManager) Write(p []byte) (int, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	return lm.rl.Write(p)
}

// SetDir switches to a new log directory, closing the current log file.
func (lm *LogManager) SetDir(dir string, retentionDays int) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	rl, err := newRotatingLogger(dir, retentionDays)
	if err != nil {
		return err
	}
	_ = lm.rl.Close()
	lm.rl = rl
	return nil
}

func (lm *LogManager) Dir() string {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	return lm.rl.dir
}

func (lm *LogManager) RetentionDays() int {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	return lm.rl.retentionDays
}

func (lm *LogManager) Close() error {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	return lm.rl.Close()
}
