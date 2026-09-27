package messaging

type UserCard struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
}

type Reaction struct {
	Emoji   string   `json:"emoji"`
	UserIDs []string `json:"userIds"`
}

type Message struct {
	ID          string     `json:"id"`
	ClientID    string     `json:"clientId"`
	SenderID    string     `json:"senderId"`
	RecipientID string     `json:"recipientId"`
	Text        string     `json:"text"`
	CreatedAt   string     `json:"createdAt"`
	EditedAt    *string    `json:"editedAt"`
	ExpiresAt   string     `json:"expiresAt"`
	Reactions   []Reaction `json:"reactions"`
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
