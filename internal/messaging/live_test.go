package messaging

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// TestLiveChatBridge exercises both launcher sessions against the actual API
// and PostgreSQL. The explicit fixture must contain disposable local accounts.
func TestLiveChatBridge(t *testing.T) {
	path := os.Getenv("TYPHON_CHAT_FIXTURE")
	if path == "" {
		t.Skip("set TYPHON_CHAT_FIXTURE to a disposable local chat fixture")
	}
	var f struct {
		URL        string `json:"url"`
		Alice, Bob struct{ ID, Token string }
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(f.URL)
	if err != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("live chat fixture must use loopback API")
	}
	makeClient := func(token string) (*Service, chan Event) {
		s, err := NewService(f.URL, func() (string, error) { return token, nil }, func() bool { return true })
		if err != nil {
			t.Fatal(err)
		}
		ch := make(chan Event, 256)
		s.emit = func(e Event) { ch <- e }
		s.ServiceStartup(context.Background(), application.ServiceOptions{})
		t.Cleanup(func() { s.ServiceShutdown() })
		if err = s.Start(); err != nil {
			t.Fatal(err)
		}
		awaitKind(t, ch, "sync")
		return s, ch
	}
	a, ae := makeClient(f.Alice.Token)
	b, be := makeClient(f.Bob.Token)
	cid := uuid.NewString()
	first, err := a.Send(f.Bob.ID, cid, "Проверка лички 🎮 "+cid)
	if err != nil {
		t.Fatal(err)
	}
	e := awaitKind(t, be, "message")
	if e.PeerID != f.Alice.ID || e.OwnerID != f.Bob.ID || e.Message.ID != first.ID {
		t.Fatalf("wrong incoming routing: %+v", e)
	}
	sent := awaitKind(t, ae, "message")
	if sent.PeerID != f.Bob.ID {
		t.Fatalf("wrong outgoing peer: %+v", sent)
	}
	duplicate, err := a.Send(f.Bob.ID, cid, first.Text)
	if err != nil || duplicate.ID != first.ID {
		t.Fatalf("retry duplicated: %v", err)
	}
	expiry, err := time.Parse(time.RFC3339Nano, first.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	created, err := time.Parse(time.RFC3339Nano, first.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if diff := expiry.Sub(created) - 7*24*time.Hour; diff > time.Second || diff < -time.Second {
		t.Fatalf("retention differs: %v", diff)
	}
	edited, err := a.Edit(f.Bob.ID, first.ID, "Отредактировано 🎮")
	if err != nil || edited.EditedAt == nil || edited.ExpiresAt != first.ExpiresAt {
		t.Fatalf("edit/TTL: %+v %v", edited, err)
	}
	awaitKind(t, be, "updated")
	if _, err = b.Edit(f.Alice.ID, first.ID, "not mine"); err == nil {
		t.Fatal("recipient edited author's message")
	}
	if err = b.React(f.Alice.ID, first.ID, "heart"); err != nil {
		t.Fatal(err)
	}
	awaitKind(t, ae, "updated")
	page, err := a.Messages(f.Bob.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range page.Messages {
		if m.ID == first.ID {
			found = len(m.Reactions) == 1 && m.Reactions[0].Emoji == "heart" && m.Reactions[0].UserIDs[0] == f.Bob.ID
		}
	}
	if !found {
		t.Fatal("reaction did not cross API -> launcher model")
	}
	if err = b.Unreact(f.Alice.ID, first.ID, "heart"); err != nil {
		t.Fatal(err)
	}
	if err = b.Read(f.Alice.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	cs, err := b.Conversations()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if c.Peer.ID == f.Alice.ID && c.Unread != 0 {
			t.Fatalf("unread after view=%d", c.Unread)
		}
	}
	if err = a.Typing(f.Bob.ID, true); err != nil {
		t.Fatal(err)
	}
	typing := awaitKind(t, be, "typing")
	if !typing.Typing || typing.PeerID != f.Alice.ID {
		t.Fatalf("typing routing: %+v", typing)
	}
	if err = a.Typing(f.Bob.ID, false); err != nil {
		t.Fatal(err)
	}
	if stopped := awaitKind(t, be, "typing"); stopped.Typing {
		t.Fatal("typing did not stop")
	}
	b.Stop()
	offline, err := a.Send(f.Bob.ID, uuid.NewString(), "Доставить после входа")
	if err != nil {
		t.Fatal(err)
	}
	if err = b.Start(); err != nil {
		t.Fatal(err)
	}
	awaitKind(t, be, "sync")
	page, err = b.Messages(f.Alice.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, m := range page.Messages {
		if m.ID == offline.ID && strings.Contains(m.Text, "после входа") {
			found = true
		}
	}
	if !found {
		t.Fatal("offline message absent after reconnect")
	}
}
