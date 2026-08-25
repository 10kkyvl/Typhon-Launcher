package app

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"typhon/internal/settings"
)

const maxLogSize = 5 << 20

// В windowsgui-сборке нет консоли, запись в os.Stderr возвращает ошибку, а
// anacrolix/log паникует на любой ошибке обработчика, роняя приложение.
type quietWriter struct {
	w io.Writer
}

func (q quietWriter) Write(p []byte) (int, error) {
	q.w.Write(p)
	return len(p), nil
}

func InitLogging() {
	writers := []io.Writer{quietWriter{os.Stderr}}
	if f, err := openLogFile(); err == nil {
		writers = append(writers, quietWriter{f})
	}
	handler := slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(handler))
}

func openLogFile() (*os.File, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "typhon.log")
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogSize {
		os.Remove(path + ".old")
		os.Rename(path, path+".old")
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}
