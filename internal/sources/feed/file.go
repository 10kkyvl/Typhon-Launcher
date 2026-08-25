package feed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrEmptyPath      = errors.New("путь к файлу фида не указан")
	ErrRelativePath   = errors.New("путь к файлу фида должен быть абсолютным")
	ErrNotRegularFile = errors.New("файл фида не является обычным файлом")
)

func ValidatePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrEmptyPath
	}
	if !filepath.IsAbs(trimmed) {
		return "", ErrRelativePath
	}
	return filepath.Clean(trimmed), nil
}

func ReadFile(ctx context.Context, raw string) (Result, error) {
	path, err := ValidatePath(raw)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	f, err := os.Open(path)
	if err != nil {
		return Result{}, fmt.Errorf("не удалось открыть файл фида: %w", err)
	}
	result, err := readFeedFile(ctx, f)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		return Result{}, fmt.Errorf("не удалось закрыть файл фида: %w", closeErr)
	}
	if err != nil {
		return Result{}, err
	}

	slog.Info("feed file read", "bytes", result.Bytes, "entries", len(result.Feed.Entries), "invalid", result.Feed.Invalid)
	return result, nil
}

func readFeedFile(ctx context.Context, f *os.File) (Result, error) {
	info, err := f.Stat()
	if err != nil {
		return Result{}, fmt.Errorf("не удалось прочитать сведения о файле фида: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Result{}, ErrNotRegularFile
	}
	if info.Size() > MaxBytes {
		return Result{}, ErrTooLarge
	}

	body, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		return Result{}, fmt.Errorf("ошибка чтения файла фида: %w", err)
	}
	if int64(len(body)) > MaxBytes {
		return Result{}, ErrTooLarge
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	parsed, err := Parse(body)
	if err != nil {
		return Result{}, err
	}
	return Result{Feed: parsed, Bytes: int64(len(body))}, nil
}
