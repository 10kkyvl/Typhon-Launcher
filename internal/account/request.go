package account

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (c *Client) JSON(ctx context.Context, method, path string, in, out any) error {
	tok, err := c.resolveToken()
	if err != nil {
		return err
	}
	if tok == "" {
		return &Error{Code: CodeUnauthenticated, Status: http.StatusUnauthorized}
	}

	var body io.Reader
	contentType := ""
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode %s %s body: %w", method, path, err)
		}
		body = bytes.NewReader(data)
		contentType = "application/json"
	}

	resp, err := c.do(ctx, method, path, body, contentType, c.httpClient, tok)
	if err != nil {
		return err
	}
	defer closeBody(resp)

	limited := io.LimitReader(resp.Body, maxResponseBodySize)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeError(resp.StatusCode, limited)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(limited).Decode(out); err != nil {
		return &Error{Code: CodeServer, Status: resp.StatusCode, cause: fmt.Errorf("decode %s %s response: %w", method, path, err)}
	}
	return nil
}
