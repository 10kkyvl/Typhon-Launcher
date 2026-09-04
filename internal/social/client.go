package social

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"typhon/internal/account"
	"typhon/internal/app"
)

const (
	maxResponseBytes = 1 << 20
	requestTimeout   = 30 * time.Second
)

var (
	ErrUnauthorized = errors.New("social: not authenticated")
	ErrUnsupported  = errors.New("social: not supported by this server")
)

type APIError struct {
	Code   string
	Field  string
	Status int
}

func (e *APIError) Error() string { return e.Code }

type NetworkError struct {
	cause error
}

func (e *NetworkError) Error() string { return fmt.Sprintf("social: network error: %v", e.cause) }

func (e *NetworkError) Unwrap() error { return e.cause }

type ServerError struct {
	Status int
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("social: server error, status %d", e.Status)
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Field   string `json:"field"`
		Message string `json:"message"`
	} `json:"error"`
}

type sendRequestBody struct {
	Query string `json:"query"`
}

type blocksBody struct {
	Blocks []UserCard `json:"blocks"`
}

type friendCodeBody struct {
	Code string `json:"code"`
}

type client struct {
	baseURL    string
	token      func() (string, error)
	httpClient *http.Client
}

func newClient(baseURL string, token func() (string, error)) (*client, error) {
	base, err := account.ValidateBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, errors.New("social: token resolver is nil")
	}
	return &client{
		baseURL: base,
		token:   token,
		httpClient: &http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 20 * time.Second,
				ExpectContinueTimeout: 5 * time.Second,
			},
			CheckRedirect: account.CheckRedirect,
		},
	}, nil
}

func (c *client) friendsPage(ctx context.Context) (FriendsPage, error) {
	var page FriendsPage
	if err := c.do(ctx, http.MethodGet, account.APIPrefix+"/me/friends", nil, &page); err != nil {
		return FriendsPage{}, err
	}
	return page, nil
}

func (c *client) sendRequest(ctx context.Context, query string) (SendResult, error) {
	var result SendResult
	err := c.do(ctx, http.MethodPost, account.APIPrefix+"/me/friends/requests", sendRequestBody{Query: query}, &result)
	if err != nil {
		return SendResult{}, err
	}
	return result, nil
}

func (c *client) accept(ctx context.Context, userID string) error {
	path := account.APIPrefix + "/me/friends/requests/" + url.PathEscape(userID) + "/accept"
	return c.do(ctx, http.MethodPost, path, nil, nil)
}

func (c *client) decline(ctx context.Context, userID string) error {
	path := account.APIPrefix + "/me/friends/requests/" + url.PathEscape(userID)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

func (c *client) unfriend(ctx context.Context, userID string) error {
	path := account.APIPrefix + "/me/friends/" + url.PathEscape(userID)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

func (c *client) block(ctx context.Context, userID string) error {
	path := account.APIPrefix + "/me/blocks/" + url.PathEscape(userID)
	return c.do(ctx, http.MethodPost, path, nil, nil)
}

func (c *client) unblock(ctx context.Context, userID string) error {
	path := account.APIPrefix + "/me/blocks/" + url.PathEscape(userID)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

func (c *client) blocks(ctx context.Context) ([]UserCard, error) {
	var body blocksBody
	if err := c.do(ctx, http.MethodGet, account.APIPrefix+"/me/blocks", nil, &body); err != nil {
		return nil, err
	}
	return body.Blocks, nil
}

func (c *client) friendCode(ctx context.Context) (string, error) {
	var body friendCodeBody
	if err := c.do(ctx, http.MethodGet, account.APIPrefix+"/me/friend-code", nil, &body); err != nil {
		return "", err
	}
	return body.Code, nil
}

func (c *client) rotateFriendCode(ctx context.Context) (string, error) {
	var body friendCodeBody
	if err := c.do(ctx, http.MethodPost, account.APIPrefix+"/me/friend-code/rotate", nil, &body); err != nil {
		return "", err
	}
	return body.Code, nil
}

func (c *client) profile(ctx context.Context, username string) (PublicProfile, error) {
	var profile PublicProfile
	path := account.APIPrefix + "/users/" + url.PathEscape(username)
	if err := c.do(ctx, http.MethodGet, path, nil, &profile); err != nil {
		return PublicProfile{}, err
	}
	return normalizeProfile(profile), nil
}

func (c *client) profileByCode(ctx context.Context, code string) (PublicProfile, error) {
	var profile PublicProfile
	path := account.APIPrefix + "/users/by-code/" + url.PathEscape(code)
	if err := c.do(ctx, http.MethodGet, path, nil, &profile); err != nil {
		return PublicProfile{}, err
	}
	return normalizeProfile(profile), nil
}

func normalizeProfile(profile PublicProfile) PublicProfile {
	if profile.RecentActivity == nil {
		profile.RecentActivity = []ActivityView{}
	}
	return profile
}

func (c *client) userGames(ctx context.Context, username, cursor string) (GamesPage, error) {
	path := account.APIPrefix + "/users/" + url.PathEscape(username) + "/games"
	if cursor != "" {
		path += "?" + url.Values{"cursor": {cursor}}.Encode()
	}
	var page GamesPage
	if err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
		return GamesPage{}, err
	}
	return page, nil
}

func (c *client) gameFriends(ctx context.Context, igdbID string) (GameFriends, error) {
	var friends GameFriends
	path := account.APIPrefix + "/games/" + url.PathEscape(igdbID) + "/friends"
	if err := c.do(ctx, http.MethodGet, path, nil, &friends); err != nil {
		return GameFriends{}, err
	}
	return friends, nil
}

func (c *client) feed(ctx context.Context, cursor int64, limit int) (FeedPage, error) {
	query := url.Values{
		"cursor": {strconv.FormatInt(cursor, 10)},
		"limit":  {strconv.Itoa(limit)},
	}
	path := account.APIPrefix + "/me/feed?" + query.Encode()
	var page FeedPage
	if err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
		return FeedPage{}, err
	}
	return normalizeFeedPage(page), nil
}

func (c *client) react(ctx context.Context, id int64, emoji string) error {
	return c.do(ctx, http.MethodPut, reactionPath(id, emoji), nil, nil)
}

func (c *client) unreact(ctx context.Context, id int64, emoji string) error {
	return c.do(ctx, http.MethodDelete, reactionPath(id, emoji), nil, nil)
}

func reactionPath(id int64, emoji string) string {
	return account.APIPrefix + "/activity/" + strconv.FormatInt(id, 10) + "/reactions/" + url.PathEscape(emoji)
}

func normalizeFeedPage(page FeedPage) FeedPage {
	if page.Events == nil {
		page.Events = []Event{}
	}
	for i := range page.Events {
		if page.Events[i].Mine == nil {
			page.Events[i].Mine = []string{}
		}
		if page.Events[i].Reactions == nil {
			page.Events[i].Reactions = []ReactionCount{}
		}
	}
	return page
}

func (c *client) do(ctx context.Context, method, path string, reqBody, out any) error {
	tok, err := c.resolveToken()
	if err != nil {
		return err
	}

	var reader io.Reader
	if reqBody != nil {
		encoded, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("encode social request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build social request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("User-Agent", account.UserAgent)
	req.Header.Set("X-Typhon-Version", app.Version)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &NetworkError{cause: err}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("close social response body", "error", err)
		}
	}()

	limited := io.LimitReader(resp.Body, maxResponseBytes)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeError(resp.StatusCode, limited)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(limited).Decode(out); err != nil {
		return fmt.Errorf("decode social response: %w", err)
	}
	return nil
}

func (c *client) resolveToken() (string, error) {
	tok, err := c.token()
	if err != nil {
		if errors.Is(err, account.ErrNoCredential) {
			return "", ErrUnauthorized
		}
		return "", fmt.Errorf("resolve social token: %w", err)
	}
	if tok == "" {
		return "", ErrUnauthorized
	}
	return tok, nil
}

func decodeError(status int, body io.Reader) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return &ServerError{Status: status}
	}

	var env errorEnvelope
	if err := json.Unmarshal(data, &env); err != nil || env.Error.Code == "" {
		switch status {
		case http.StatusUnauthorized:
			return ErrUnauthorized
		case http.StatusNotFound:
			return ErrUnsupported
		}
		return &ServerError{Status: status}
	}

	switch {
	case status == http.StatusUnauthorized:
		return ErrUnauthorized
	case status >= 500:
		return &ServerError{Status: status}
	default:
		return &APIError{Code: env.Error.Code, Field: env.Error.Field, Status: status}
	}
}
