package messaging

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"typhon/internal/account"
	"typhon/internal/app"
)

func (s *Service) newRequest(ctx context.Context, r *session, method, path string, body any) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.base+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("User-Agent", account.UserAgent)
	req.Header.Set("X-Typhon-Version", app.Version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}
func (s *Service) request(r *session, method, path string, body, out any) error {
	if err := s.sessionError(r); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.ctx, 25*time.Second)
	defer cancel()
	req, err := s.newRequest(ctx, r, method, path, body)
	if err != nil {
		return err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("messaging request: %w", err)
	}
	defer closeResponseBody(resp.Body)
	if !s.valid(r) {
		return errSignedOut
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out); err != nil {
		return fmt.Errorf("decode chat response: %w", err)
	}
	if !s.valid(r) {
		return errSignedOut
	}
	return nil
}

type responseFailure struct {
	status int
	code   string
}

func (e *responseFailure) Error() string { return e.code }

func permanentStreamError(err error) bool {
	var failure *responseFailure
	return errors.As(err, &failure) && failure.status >= 400 && failure.status < 500 &&
		failure.status != http.StatusRequestTimeout && failure.status != http.StatusTooManyRequests
}

func responseError(resp *http.Response) error {
	if resp.StatusCode == http.StatusUnauthorized {
		return errSignedOut
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&envelope) == nil && envelope.Error.Code != "" {
		return &responseFailure{status: resp.StatusCode, code: envelope.Error.Code}
	}
	return &responseFailure{status: resp.StatusCode, code: fmt.Sprintf("chat_server_error_%d", resp.StatusCode)}
}

func (s *Service) loop(r *session) {
	defer s.disconnect(r)
	// Credential changes and consent withdrawal also stop an idle connection;
	// they do not depend on the webview delivering its Stop call.
	done := make(chan struct{})
	watcherDone := make(chan struct{})
	defer func() { close(done); <-watcherDone }()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	go func() {
		defer close(watcherDone)
		s.watchSession(r, tick.C, done)
	}()
	delay := time.Second
	for s.valid(r) {
		started := time.Now()
		err := s.stream(r)
		s.publish(r, Event{Kind: "connection", Connected: false})
		if errors.Is(err, errSignedOut) || permanentStreamError(err) {
			r.cancel()
			return
		}
		if time.Since(started) > 30*time.Second {
			delay = time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-r.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(delay*2, 30*time.Second)
	}
}

// watchSession accepts ticks separately so tests can drive an idle credential
// check without relying on the scheduler or waiting for wall-clock time.
func (s *Service) watchSession(r *session, ticks <-chan time.Time, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case <-r.ctx.Done():
			return
		case <-ticks:
			if !s.valid(r) {
				s.disconnect(r)
				return
			}
		}
	}
}

func (s *Service) stream(r *session) error {
	req, err := s.newRequest(r.ctx, r, http.MethodGet, prefix+"/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer closeResponseBody(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return responseError(resp)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return errors.New("chat_invalid_stream")
	}
	s.publish(r, Event{Kind: "connection", Connected: true})
	// A black-holed connection must reconnect even if the TCP socket stays open.
	idle := time.AfterFunc(75*time.Second, func() { closeResponseBody(resp.Body) })
	defer idle.Stop()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 256<<10)
	var data strings.Builder
	for scanner.Scan() {
		idle.Reset(75 * time.Second)
		line := scanner.Text()
		if line == "" {
			if data.Len() > 0 {
				var event Event
				if err := json.Unmarshal([]byte(data.String()), &event); err != nil {
					return errors.New("chat_invalid_event")
				}
				switch event.Kind {
				case "sync", "message", "updated", "read", "typing":
					s.publish(r, event)
				}
				data.Reset()
			}
		} else if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			if data.Len() > 256<<10 {
				return errors.New("chat_event_too_large")
			}
		}
	}
	return scanner.Err()
}

func closeResponseBody(body io.Closer) {
	if err := body.Close(); err != nil {
		slog.Debug("close chat response", "error", err)
	}
}
