package support

import "time"

const (
	TopicInstall  = "install"
	TopicLaunch   = "launch"
	TopicAccount  = "account"
	TopicLauncher = "launcher"
	TopicOther    = "other"
)

const (
	StatusOpen        = "open"
	StatusWaitingUser = "waiting_user"
	StatusResolved    = "resolved"
)

const (
	MaxSubjectRunes = 120
	MaxBodyRunes    = 5000
)

var topics = []string{TopicInstall, TopicLaunch, TopicAccount, TopicLauncher, TopicOther}

const (
	CodeTicketNotFound          = "ticket_not_found"
	CodeTicketInvalidTopic      = "ticket_invalid_topic"
	CodeTicketInvalidSubject    = "ticket_invalid_subject"
	CodeTicketInvalidBody       = "ticket_invalid_body"
	CodeTicketInvalidGame       = "ticket_invalid_game"
	CodeTicketInvalidClient     = "ticket_invalid_client"
	CodeTicketLimit             = "ticket_limit"
	CodeTicketBundleLimit       = "ticket_bundle_limit"
	CodeTicketBundleUnavailable = "ticket_bundle_unavailable"
)

type ClientInfo struct {
	AppVersion string `json:"appVersion"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
}

type Ticket struct {
	Number     int64       `json:"number"`
	Ref        string      `json:"ref"`
	Topic      string      `json:"topic"`
	Subject    string      `json:"subject"`
	Status     string      `json:"status"`
	GameIgdbID *int64      `json:"gameIgdbId"`
	Client     *ClientInfo `json:"client"`
	Unread     bool        `json:"unread"`
	CreatedAt  time.Time   `json:"createdAt"`
	UpdatedAt  time.Time   `json:"updatedAt"`
	ResolvedAt *time.Time  `json:"resolvedAt"`
}

type Message struct {
	ID          int64     `json:"id"`
	Author      string    `json:"author"`
	Body        string    `json:"body"`
	LogBundleID *string   `json:"logBundleId"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Page struct {
	Tickets []Ticket `json:"tickets"`
	Next    string   `json:"next"`
}

type Detail struct {
	Ticket   Ticket    `json:"ticket"`
	Messages []Message `json:"messages"`
}

type Limits struct {
	MaxSubjectRunes int `json:"maxSubjectRunes"`
	MaxBodyRunes    int `json:"maxBodyRunes"`
}

type CreateInput struct {
	Topic      string `json:"topic"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	GameIgdbID *int64 `json:"gameIgdbId"`
	AttachLogs bool   `json:"attachLogs"`
}

type createRequest struct {
	Topic       string      `json:"topic"`
	Subject     string      `json:"subject"`
	Body        string      `json:"body"`
	GameIgdbID  *int64      `json:"gameIgdbId,omitempty"`
	LogBundleID string      `json:"logBundleId,omitempty"`
	Client      *ClientInfo `json:"client,omitempty"`
	Lang        string      `json:"lang,omitempty"`
}

type replyRequest struct {
	Body        string `json:"body"`
	LogBundleID string `json:"logBundleId,omitempty"`
}

type ticketEnvelope struct {
	Ticket Ticket `json:"ticket"`
}

type messageEnvelope struct {
	Message Message `json:"message"`
}

type unreadEnvelope struct {
	Count int `json:"count"`
}
