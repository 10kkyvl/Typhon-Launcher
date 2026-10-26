package messaging

import (
	"encoding/json"

	"typhon/internal/account"
)

type UserCard struct {
	ID          string              `json:"id"`
	Username    string              `json:"username"`
	DisplayName string              `json:"displayName"`
	AvatarURL   string              `json:"avatarUrl"`
	AvatarFrame string              `json:"avatarFrame"`
	FrameColor  string              `json:"frameColor"`
	FrameImage  *account.FrameImage `json:"frameImage,omitempty"`
}

// UnmarshalJSON drops a frame picture that fails validation instead of failing the whole peer.
func (u *UserCard) UnmarshalJSON(data []byte) error {
	type plain UserCard
	var v plain
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	v.FrameImage = account.SanitizeFrameImage(v.FrameImage)
	*u = UserCard(v)
	return nil
}

type Reaction struct {
	Emoji   string   `json:"emoji"`
	UserIDs []string `json:"userIds"`
}

type Attachment struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Bytes  int64  `json:"bytes"`
}

type Message struct {
	ID          string       `json:"id"`
	ClientID    string       `json:"clientId"`
	SenderID    string       `json:"senderId"`
	RecipientID string       `json:"recipientId"`
	Text        string       `json:"text"`
	CreatedAt   string       `json:"createdAt"`
	EditedAt    *string      `json:"editedAt"`
	ExpiresAt   string       `json:"expiresAt"`
	Reactions   []Reaction   `json:"reactions"`
	Attachments []Attachment `json:"attachments"`
}

// The server always sends an array; null or a missing field must still reach the UI as an empty list.
func (m *Message) UnmarshalJSON(data []byte) error {
	type plain Message
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	if p.Attachments == nil {
		p.Attachments = []Attachment{}
	}
	*m = Message(p)
	return nil
}

type Conversation struct {
	Peer        UserCard `json:"peer"`
	LastMessage Message  `json:"lastMessage"`
	Unread      int      `json:"unread"`
	CanSend     bool     `json:"canSend"`
}

type Page struct {
	Messages []Message `json:"messages"`
	Next     string    `json:"next"`
	CanSend  bool      `json:"canSend"`
}

const EventName = "chat:event"

type Event struct {
	OwnerID   string   `json:"ownerId"`
	Kind      string   `json:"kind"`
	PeerID    string   `json:"peerId"`
	Message   *Message `json:"message,omitempty"`
	Typing    bool     `json:"typing"`
	Connected bool     `json:"connected"`
}

type OpenEvent struct {
	OwnerID string `json:"ownerId"`
	PeerID  string `json:"peerId"`
}
