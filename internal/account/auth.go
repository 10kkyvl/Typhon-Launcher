package account

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type RegisterInput struct {
	Email       string `json:"email"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

type LoginInput struct {
	Identifier string `json:"emailOrUsername"`
	Password   string `json:"password"`
}

type Session struct {
	User      CurrentUser `json:"user"`
	Token     string      `json:"token"`
	ExpiresAt time.Time   `json:"expiresAt"`
}

func (c *Client) Register(ctx context.Context, in RegisterInput) (Session, error) {
	return c.authRequest(ctx, APIPrefix+"/auth/register", in, http.StatusCreated)
}

func (c *Client) Login(ctx context.Context, in LoginInput) (Session, error) {
	return c.authRequest(ctx, APIPrefix+"/auth/login", in, http.StatusOK)
}

func (c *Client) Logout(ctx context.Context, token string) error {
	resp, err := c.do(ctx, http.MethodPost, APIPrefix+"/auth/logout", nil, "", c.httpClient, token)
	if err != nil {
		return err
	}
	defer closeBody(resp)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeError(resp.StatusCode, io.LimitReader(resp.Body, maxResponseBodySize))
	}
	return nil
}

func (c *Client) authRequest(ctx context.Context, path string, in any, wantStatus int) (Session, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return Session{}, fmt.Errorf("encode %s body: %w", path, err)
	}

	resp, err := c.do(ctx, http.MethodPost, path, bytes.NewReader(body), "application/json", c.httpClient, "")
	if err != nil {
		return Session{}, err
	}
	defer closeBody(resp)

	limited := io.LimitReader(resp.Body, maxResponseBodySize)

	if resp.StatusCode != wantStatus {
		return Session{}, decodeError(resp.StatusCode, limited)
	}

	var session Session
	if err := json.NewDecoder(limited).Decode(&session); err != nil {
		return Session{}, &Error{
			Code:   CodeServer,
			Status: resp.StatusCode,
			cause:  fmt.Errorf("decode %s response body: %w", path, err),
		}
	}
	if session.Token == "" {
		return Session{}, &Error{Code: CodeServer, Status: resp.StatusCode, cause: fmt.Errorf("%s returned an empty token", path)}
	}
	session.User = withProfileDefaults(session.User)
	return session, nil
}

func closeBody(resp *http.Response) {
	if err := resp.Body.Close(); err != nil {
		slog.Debug("close response body", "error", err)
	}
}
