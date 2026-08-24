package metadata

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotConfigured = errors.New("провайдер метаданных не настроен")
	ErrAmbiguous     = errors.New("однозначное совпадение не найдено")
	ErrNoMatch       = errors.New("игра не найдена у провайдера метаданных")
	ErrRateLimited   = errors.New("сервер метаданных ограничил частоту запросов")
)

type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("%s: повтор через %s", ErrRateLimited, e.RetryAfter.Round(time.Second))
	}
	return ErrRateLimited.Error()
}

func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

func retryAfter(err error) (time.Duration, bool) {
	var limit *RateLimitError
	if errors.As(err, &limit) {
		return limit.RetryAfter, true
	}
	return 0, errors.Is(err, ErrRateLimited)
}

//wails:ignore
type Provider interface {
	Name() string
	Search(ctx context.Context, query string, limit int) ([]Candidate, error)
	Get(ctx context.Context, providerID string) (GameMetadata, error)
}

type Resolved struct {
	Title string
	Meta  GameMetadata
}

// BatchProvider умеет разобрать пачку названий одним запросом. Провайдер без
// этого интерфейса обслуживается поштучно, поэтому реализация необязательна.
//
//wails:ignore
type BatchProvider interface {
	Provider
	Resolve(ctx context.Context, titles []string) ([]Resolved, error)
}
