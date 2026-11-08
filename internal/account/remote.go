package account

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// RemoteCommand is a command the Discord bot left for this launcher.
type RemoteCommand struct {
	ID        string    `json:"id"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	CreatedAt time.Time `json:"createdAt"`
}

// RemoteAnswer is the launcher's reply to a RemoteCommand.
type RemoteAnswer struct {
	Status    string          `json:"status"`
	Result    json.RawMessage `json:"result,omitempty"`
	ErrorCode string          `json:"errorCode,omitempty"`
}

const remotePath = APIPrefix + "/me/remote"

func (c *Client) remoteRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	tok, err := c.resolveToken()
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	contentType := ""
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode remote request: %w", err)
		}
		reader, contentType = bytes.NewReader(raw), "application/json"
	}
	resp, err := c.do(ctx, method, path, reader, contentType, c.httpClient, tok)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	defer closeBody(resp)
	return nil, decodeError(resp.StatusCode, io.LimitReader(resp.Body, maxResponseBodySize))
}

func remoteDevicePath(installationID string) string {
	return remotePath + "/devices/" + url.PathEscape(installationID)
}

// EnableRemoteDevice tells the backend that this launcher allows control from
// Discord.
func (c *Client) EnableRemoteDevice(ctx context.Context, installationID, name string) error {
	resp, err := c.remoteRequest(ctx, http.MethodPut, remoteDevicePath(installationID), map[string]string{"name": name})
	if err != nil {
		return err
	}
	closeBody(resp)
	return nil
}

// DisableRemoteDevice withdraws it.
func (c *Client) DisableRemoteDevice(ctx context.Context, installationID string) error {
	resp, err := c.remoteRequest(ctx, http.MethodDelete, remoteDevicePath(installationID), nil)
	if err != nil {
		return err
	}
	closeBody(resp)
	return nil
}

// RemoteCommands returns the commands waiting for this launcher.
func (c *Client) RemoteCommands(ctx context.Context, installationID string) ([]RemoteCommand, error) {
	resp, err := c.remoteRequest(ctx, http.MethodGet, remoteDevicePath(installationID)+"/commands", nil)
	if err != nil {
		return nil, err
	}
	defer closeBody(resp)
	var out struct {
		Commands []RemoteCommand `json:"commands"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBodySize)).Decode(&out); err != nil {
		return nil, &Error{Code: CodeServer, Status: resp.StatusCode, cause: fmt.Errorf("decode remote commands: %w", err)}
	}
	return out.Commands, nil
}

// RemoteNotification is a message for the owner's linked Discord account.
type RemoteNotification struct {
	InstallationID string `json:"installationId"`
	Kind           string `json:"kind"`
	Title          string `json:"title"`
	Code           string `json:"code,omitempty"`
}

// PostRemoteNotification asks the backend to have the bot send it.
func (c *Client) PostRemoteNotification(ctx context.Context, n RemoteNotification) error {
	resp, err := c.remoteRequest(ctx, http.MethodPost, remotePath+"/notifications", n)
	if err != nil {
		return err
	}
	closeBody(resp)
	return nil
}

// FinishRemoteCommand posts the answer to a command.
func (c *Client) FinishRemoteCommand(ctx context.Context, installationID, id string, a RemoteAnswer) error {
	body := struct {
		InstallationID string `json:"installationId"`
		RemoteAnswer
	}{installationID, a}
	resp, err := c.remoteRequest(ctx, http.MethodPost, remotePath+"/commands/"+url.PathEscape(id), body)
	if err != nil {
		return err
	}
	closeBody(resp)
	return nil
}
