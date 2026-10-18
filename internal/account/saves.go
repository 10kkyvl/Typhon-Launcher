package account

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	CodeSaveNotFound = "save_not_found"
	CodeSaveTooLarge = "save_too_large"
	CodeSaveQuota    = "save_quota_exceeded"
	CodeSaveInvalid  = "save_invalid"
	CodeSaveConflict = "save_conflict"
	CodeSaveBusy     = "saves_busy"
)

var (
	saveGameIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	saveDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type Save struct {
	GameID    string    `json:"gameId"`
	SizeBytes int64     `json:"sizeBytes"`
	Digest    string    `json:"digest"`
	DeviceID  string    `json:"deviceId"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type SaveList struct {
	Games      []Save `json:"games"`
	UsedBytes  int64  `json:"usedBytes"`
	QuotaBytes int64  `json:"quotaBytes"`
	MaxBytes   int64  `json:"maxBytes"`
}

// SaveUpload carries the headers of one upload. IfMatch is the server digest
// the client saw last; empty means "I believe nothing is stored". Force skips
// that check for a player who chose to overwrite the stored copy.
type SaveUpload struct {
	Digest   string
	DeviceID string
	IfMatch  string
	Force    bool
}

type SaveDownload struct {
	Digest string
	Size   int64
}

// ErrSaveConflict is the 409 of an upload whose If-Match no longer matches.
// Current is what the server holds now; nil means the server answered without
// one because the record no longer exists.
type ErrSaveConflict struct {
	Current *Save
}

func (e *ErrSaveConflict) Error() string {
	return CodeSaveConflict
}

func ValidSaveGameID(id string) bool {
	return saveGameIDPattern.MatchString(id)
}

func savePath(gameID string) (string, error) {
	if !ValidSaveGameID(gameID) {
		return "", &Error{Code: CodeBadRequest, Field: "gameId"}
	}
	return APIPrefix + "/me/saves/" + gameID, nil
}

func (c *Client) saveRequest(
	ctx context.Context,
	method, path string,
	body io.Reader,
	contentType string,
	hc *http.Client,
	adjust func(*http.Request),
) (*http.Response, error) {
	tok, err := c.resolveToken()
	if err != nil {
		return nil, err
	}
	if tok == "" {
		return nil, &Error{Code: CodeUnauthenticated, Status: http.StatusUnauthorized}
	}
	return c.doWith(ctx, method, path, body, contentType, hc, tok, adjust)
}

// retryAfterSeconds keeps Retry-After only when it is a number of seconds; the
// HTTP-date form is not worth parsing for a retry that happens on the next pass.
func retryAfterSeconds(v string) string {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func decodeSaveError(resp *http.Response, body io.Reader) error {
	status := resp.StatusCode
	data, err := io.ReadAll(body)
	if err != nil {
		return &Error{Code: CodeServer, Status: status, cause: fmt.Errorf("read error body: %w", err)}
	}
	if status == http.StatusServiceUnavailable {
		var env errorEnvelope
		if err := json.Unmarshal(data, &env); err == nil && env.Error.Code == CodeSaveBusy {
			return &Error{Code: CodeSaveBusy, Status: status, Message: retryAfterSeconds(resp.Header.Get("Retry-After"))}
		}
	}
	if status == http.StatusConflict {
		var env struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
			Current *Save `json:"current"`
		}
		if err := json.Unmarshal(data, &env); err == nil && env.Error.Code == CodeSaveConflict {
			return &ErrSaveConflict{Current: env.Current}
		}
	}
	return decodeError(status, bytes.NewReader(data))
}

func decodeSaveBody[T any](resp *http.Response, what string) (T, error) {
	var out T
	limited := io.LimitReader(resp.Body, maxResponseBodySize)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, decodeSaveError(resp, limited)
	}
	if err := json.NewDecoder(limited).Decode(&out); err != nil {
		return out, &Error{Code: CodeServer, Status: resp.StatusCode, cause: fmt.Errorf("decode %s response: %w", what, err)}
	}
	return out, nil
}

func (c *Client) ListSaves(ctx context.Context) (SaveList, error) {
	resp, err := c.saveRequest(ctx, http.MethodGet, APIPrefix+"/me/saves", nil, "", c.httpClient, nil)
	if err != nil {
		return SaveList{}, err
	}
	defer closeBody(resp)
	return decodeSaveBody[SaveList](resp, "saves list")
}

func (c *Client) GetSave(ctx context.Context, gameID string) (Save, error) {
	path, err := savePath(gameID)
	if err != nil {
		return Save{}, err
	}
	resp, err := c.saveRequest(ctx, http.MethodGet, path, nil, "", c.httpClient, nil)
	if err != nil {
		return Save{}, err
	}
	defer closeBody(resp)
	return decodeSaveBody[Save](resp, "save")
}

// DownloadSave streams the stored archive into dst and fails once the body
// exceeds maxBytes, so a server that sends more than it advertised cannot
// fill the disk.
func (c *Client) DownloadSave(ctx context.Context, gameID string, dst io.Writer, maxBytes int64) (SaveDownload, error) {
	path, err := savePath(gameID)
	if err != nil {
		return SaveDownload{}, err
	}
	if maxBytes <= 0 {
		return SaveDownload{}, &Error{Code: CodeBadRequest, Field: "maxBytes"}
	}
	resp, err := c.saveRequest(ctx, http.MethodGet, path+"/archive", nil, "", c.transferHTTP, nil)
	if err != nil {
		return SaveDownload{}, err
	}
	defer closeBody(resp)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SaveDownload{}, decodeSaveError(resp, io.LimitReader(resp.Body, maxResponseBodySize))
	}
	if resp.ContentLength > maxBytes {
		return SaveDownload{}, &Error{Code: CodeSaveTooLarge, Status: resp.StatusCode}
	}
	n, err := io.Copy(dst, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return SaveDownload{}, &Error{Code: CodeNetwork, cause: fmt.Errorf("read save archive: %w", err)}
	}
	if resp.ContentLength >= 0 && n != resp.ContentLength {
		return SaveDownload{}, &Error{Code: CodeNetwork, cause: fmt.Errorf("read save archive: got %d of %d bytes: %w", n, resp.ContentLength, io.ErrUnexpectedEOF)}
	}
	if n > maxBytes {
		return SaveDownload{}, &Error{Code: CodeSaveTooLarge, Status: resp.StatusCode}
	}
	return SaveDownload{Digest: resp.Header.Get("X-Typhon-Save-Digest"), Size: n}, nil
}

// UploadSave sends size bytes of body as the game's archive. The body is read
// from the start and never buffered whole.
func (c *Client) UploadSave(ctx context.Context, gameID string, body io.ReadSeeker, size int64, up SaveUpload) (Save, error) {
	path, err := savePath(gameID)
	if err != nil {
		return Save{}, err
	}
	switch {
	case size <= 0:
		return Save{}, &Error{Code: CodeSaveInvalid, Field: "size"}
	case !saveDigestPattern.MatchString(up.Digest):
		return Save{}, &Error{Code: CodeBadRequest, Field: "digest"}
	case up.DeviceID == "":
		return Save{}, &Error{Code: CodeBadRequest, Field: "deviceId"}
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return Save{}, fmt.Errorf("rewind save archive: %w", err)
	}
	adjust := func(req *http.Request) {
		req.ContentLength = size
		req.GetBody = func() (io.ReadCloser, error) {
			if _, err := body.Seek(0, io.SeekStart); err != nil {
				return nil, err
			}
			return io.NopCloser(body), nil
		}
		req.Header.Set("X-Typhon-Save-Digest", up.Digest)
		req.Header.Set("X-Typhon-Device", up.DeviceID)
		if up.IfMatch != "" {
			req.Header.Set("If-Match", up.IfMatch)
		}
		if up.Force {
			req.Header.Set("X-Typhon-Force", "1")
		}
	}
	resp, err := c.saveRequest(ctx, http.MethodPut, path, io.NopCloser(body), "application/zip", c.transferHTTP, adjust)
	if err != nil {
		return Save{}, err
	}
	defer closeBody(resp)
	saved, err := decodeSaveBody[Save](resp, "save upload")
	if err != nil {
		return Save{}, err
	}
	if saved.Digest == "" {
		return Save{}, &Error{Code: CodeServer, Status: resp.StatusCode, cause: errors.New("save reply has no digest")}
	}
	return saved, nil
}

func (c *Client) DeleteSave(ctx context.Context, gameID string) error {
	path, err := savePath(gameID)
	if err != nil {
		return err
	}
	resp, err := c.saveRequest(ctx, http.MethodDelete, path, nil, "", c.httpClient, nil)
	if err != nil {
		return err
	}
	defer closeBody(resp)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeSaveError(resp, io.LimitReader(resp.Body, maxResponseBodySize))
	}
	return nil
}
