package support

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"typhon/internal/account"
)

const (
	supportPath = account.APIPrefix + "/support"
	ticketsPath = supportPath + "/tickets"
	unreadPath  = supportPath + "/unread"
)

type Client struct {
	api *account.Client
}

func NewClient(baseURL string, token func() (string, error)) (*Client, error) {
	if token == nil {
		return nil, errors.New("support: token resolver is nil")
	}
	api, err := account.NewClient(baseURL, token)
	if err != nil {
		return nil, err
	}
	return &Client{api: api}, nil
}

func ticketPath(number int64) (string, error) {
	if number <= 0 {
		return "", &account.Error{Code: CodeTicketNotFound, Status: http.StatusNotFound}
	}
	return ticketsPath + "/" + strconv.FormatInt(number, 10), nil
}

func (c *Client) Create(ctx context.Context, req createRequest) (Ticket, error) {
	var out ticketEnvelope
	if err := c.api.JSON(ctx, http.MethodPost, ticketsPath, req, &out); err != nil {
		return Ticket{}, err
	}
	return out.Ticket, nil
}

func (c *Client) List(ctx context.Context, cursor string) (Page, error) {
	path := ticketsPath
	if cursor != "" {
		path += "?" + url.Values{"cursor": {cursor}}.Encode()
	}
	var out Page
	if err := c.api.JSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return Page{}, err
	}
	if out.Tickets == nil {
		out.Tickets = []Ticket{}
	}
	return out, nil
}

func (c *Client) Get(ctx context.Context, number int64) (Detail, error) {
	path, err := ticketPath(number)
	if err != nil {
		return Detail{}, err
	}
	var out Detail
	if err := c.api.JSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return Detail{}, err
	}
	if out.Messages == nil {
		out.Messages = []Message{}
	}
	return out, nil
}

func (c *Client) Reply(ctx context.Context, number int64, body, bundleID string) (Message, error) {
	path, err := ticketPath(number)
	if err != nil {
		return Message{}, err
	}
	var out messageEnvelope
	if err := c.api.JSON(ctx, http.MethodPost, path+"/messages", replyRequest{Body: body, LogBundleID: bundleID}, &out); err != nil {
		return Message{}, err
	}
	return out.Message, nil
}

func (c *Client) Resolve(ctx context.Context, number int64) (Ticket, error) {
	path, err := ticketPath(number)
	if err != nil {
		return Ticket{}, err
	}
	var out ticketEnvelope
	if err := c.api.JSON(ctx, http.MethodPost, path+"/resolve", nil, &out); err != nil {
		return Ticket{}, err
	}
	return out.Ticket, nil
}

func (c *Client) Unread(ctx context.Context) (int, error) {
	var out unreadEnvelope
	if err := c.api.JSON(ctx, http.MethodGet, unreadPath, nil, &out); err != nil {
		return 0, err
	}
	return out.Count, nil
}
