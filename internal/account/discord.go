package account

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CodeDiscordUnavailable means the backend does not serve the Discord routes
// (yet): a 404 on them is a missing feature, not a missing record.
const CodeDiscordUnavailable = "discord_unavailable"

// DiscordLinkCode is a one-time code the player types into the Typhon bot.
type DiscordLinkCode struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// DiscordLink is the state of the account's Discord link. The identity fields
// are empty while Linked is false.
type DiscordLink struct {
	Linked          bool      `json:"linked"`
	DiscordID       string    `json:"discordId"`
	DiscordUsername string    `json:"discordUsername"`
	LinkedAt        time.Time `json:"linkedAt"`
}

const discordPath = APIPrefix + "/me/discord"

func (c *Client) discordRequest(ctx context.Context, method, path string) (*http.Response, error) {
	tok, err := c.resolveToken()
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, method, path, nil, "", c.httpClient, tok)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	defer closeBody(resp)
	err = decodeError(resp.StatusCode, io.LimitReader(resp.Body, maxResponseBodySize))
	if resp.StatusCode == http.StatusNotFound {
		return nil, &Error{Code: CodeDiscordUnavailable, Status: resp.StatusCode, cause: err}
	}
	return nil, err
}

func decodeDiscordBody[T any](resp *http.Response, what string) (T, error) {
	var out T
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBodySize)).Decode(&out); err != nil {
		return out, &Error{Code: CodeServer, Status: resp.StatusCode, cause: fmt.Errorf("decode %s: %w", what, err)}
	}
	return out, nil
}

func (c *Client) CreateDiscordLinkCode(ctx context.Context) (DiscordLinkCode, error) {
	resp, err := c.discordRequest(ctx, http.MethodPost, discordPath+"/link-code")
	if err != nil {
		return DiscordLinkCode{}, err
	}
	defer closeBody(resp)
	code, err := decodeDiscordBody[DiscordLinkCode](resp, "discord link code")
	if err != nil {
		return DiscordLinkCode{}, err
	}
	if code.Code == "" {
		return DiscordLinkCode{}, &Error{Code: CodeServer, Status: resp.StatusCode, cause: fmt.Errorf("discord link code reply has no code")}
	}
	return code, nil
}

func (c *Client) GetDiscordLink(ctx context.Context) (DiscordLink, error) {
	resp, err := c.discordRequest(ctx, http.MethodGet, discordPath)
	if err != nil {
		return DiscordLink{}, err
	}
	defer closeBody(resp)
	link, err := decodeDiscordBody[DiscordLink](resp, "discord link")
	if err != nil {
		return DiscordLink{}, err
	}
	if !link.Linked {
		return DiscordLink{}, nil
	}
	return link, nil
}

func (c *Client) UnlinkDiscord(ctx context.Context) error {
	resp, err := c.discordRequest(ctx, http.MethodDelete, discordPath)
	if err != nil {
		return err
	}
	closeBody(resp)
	return nil
}

func (s *Service) CreateDiscordLinkCode() (DiscordLinkCode, error) {
	ctx, cancel, err := s.requestContext()
	if err != nil {
		return DiscordLinkCode{}, err
	}
	defer cancel()
	return s.client.CreateDiscordLinkCode(ctx)
}

func (s *Service) GetDiscordLink() (DiscordLink, error) {
	ctx, cancel, err := s.requestContext()
	if err != nil {
		return DiscordLink{}, err
	}
	defer cancel()
	return s.client.GetDiscordLink(ctx)
}

func (s *Service) UnlinkDiscord() error {
	ctx, cancel, err := s.requestContext()
	if err != nil {
		return err
	}
	defer cancel()
	return s.client.UnlinkDiscord(ctx)
}

// OpenDiscordInvite opens the community server invite in the system browser.
func (s *Service) OpenDiscordInvite() error {
	if err := s.openURL(DiscordInviteURL); err != nil {
		return fmt.Errorf("open discord invite: %w", err)
	}
	return nil
}
