// Package messaging owns the authenticated chat connection. A session's requests,
// stream and notifications share cancellation; no chat data is persisted locally.
package messaging

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"typhon/internal/account"
)

var errSignedOut = errors.New("unauthenticated")

const prefix = account.APIPrefix + "/me/chat"

type session struct {
	ctx          context.Context
	cancel       context.CancelFunc
	token, owner string
}

type Service struct {
	mu          sync.Mutex
	ctx         context.Context
	run         *session
	token       func() (string, error)
	enabled     func() bool
	base        string
	http        *http.Client
	emit        func(Event)
	notify      func(owner, peer, title, body string) bool
	clearNotify func()
	wg          sync.WaitGroup
}

func NewService(base string, token func() (string, error), enabled func() bool) (*Service, error) {
	base, err := account.ValidateBaseURL(base)
	if err != nil {
		return nil, err
	}
	if token == nil || enabled == nil {
		return nil, errors.New("messaging: missing session ports")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 20 * time.Second
	s := &Service{base: base, token: token, enabled: enabled, http: &http.Client{Transport: transport, CheckRedirect: account.CheckRedirect}}
	s.emit = func(event Event) {
		if a := application.Get(); a != nil {
			a.Event.Emit(EventName, event)
		}
	}
	return s, nil
}

func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctx = ctx
	return nil
}
func (s *Service) ServiceShutdown() error {
	s.mu.Lock()
	s.ctx = nil
	s.mu.Unlock()
	s.Stop()
	s.wg.Wait()
	s.http.CloseIdleConnections()
	return nil
}

// Start binds a fresh stream to the actual server-authenticated account. It is
// safe for a concurrent logout to cancel the initial identity request.
func (s *Service) Start() error {
	s.mu.Lock()
	if s.ctx == nil {
		s.mu.Unlock()
		return errSignedOut
	}
	if s.run != nil {
		s.run.cancel()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	r := &session{ctx: ctx, cancel: cancel}
	s.run = r
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()
	fail := func(err error) error {
		cancel()
		s.mu.Lock()
		if s.run == r {
			s.run = nil
		}
		s.mu.Unlock()
		return err
	}
	if !s.enabled() {
		return fail(errors.New("sync_disabled"))
	}
	tok, err := s.token()
	if err != nil {
		return fail(err)
	}
	if tok == "" {
		return fail(errSignedOut)
	}
	// Token is assigned before the session is published as authenticated (owner).
	s.mu.Lock()
	r.token = tok
	s.mu.Unlock()
	var user struct {
		ID string `json:"id"`
	}
	if err = s.request(r, http.MethodGet, account.APIPrefix+"/me", nil, &user); err != nil {
		return fail(err)
	}
	if user.ID == "" {
		return fail(errSignedOut)
	}
	s.mu.Lock()
	if s.run != r || ctx.Err() != nil || s.ctx == nil {
		s.mu.Unlock()
		return fail(errSignedOut)
	}
	r.owner = user.ID
	s.wg.Add(1)
	s.mu.Unlock()
	go func() { defer s.wg.Done(); s.loop(r) }()
	return nil
}

func (s *Service) Stop() {
	s.mu.Lock()
	r := s.run
	s.run = nil
	clear := s.clearNotify
	if r != nil {
		r.cancel()
	}
	s.mu.Unlock()
	if clear != nil {
		clear()
	}
}

func (s *Service) active() (*session, error) {
	s.mu.Lock()
	r := s.run
	ready := r != nil && r.owner != ""
	s.mu.Unlock()
	if !ready || !s.valid(r) {
		return nil, errSignedOut
	}
	return r, nil
}
func (s *Service) valid(r *session) bool {
	if r.ctx.Err() != nil || !s.enabled() {
		return false
	}
	tok, err := s.token()
	s.mu.Lock()
	defer s.mu.Unlock()
	return err == nil && s.run == r && tok != "" && tok == r.token
}
func (s *Service) publish(r *session, e Event) {
	if !s.valid(r) {
		return
	}
	e.OwnerID = r.owner
	s.emit(e)
}

//wails:ignore
func (s *Service) SetNotifier(notify func(owner, peer, title, body string) bool, clear func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notify = notify
	s.clearNotify = clear
}
func (s *Service) Notify(peerID, title, body string) bool {
	r, err := s.active()
	if err != nil {
		return false
	}
	s.mu.Lock()
	fn := s.notify
	s.mu.Unlock()
	if fn == nil {
		return false
	}
	return fn(r.owner, peerID, title, body)
}

func (s *Service) Conversations() ([]Conversation, error) {
	r, err := s.active()
	if err != nil {
		return nil, err
	}
	out := struct {
		Conversations []Conversation `json:"conversations"`
	}{Conversations: []Conversation{}}
	err = s.request(r, http.MethodGet, prefix+"/conversations", nil, &out)
	return out.Conversations, err
}
func (s *Service) Messages(peerID, before string) (Page, error) {
	r, err := s.active()
	if err != nil {
		return Page{}, err
	}
	path := prefix + "/" + url.PathEscape(peerID) + "/messages"
	if before != "" {
		path += "?before=" + url.QueryEscape(before)
	}
	out := Page{Messages: []Message{}}
	err = s.request(r, http.MethodGet, path, nil, &out)
	return out, err
}
func (s *Service) Send(peerID, clientID, text string) (Message, error) {
	return s.message(http.MethodPost, peerID, "", map[string]string{"clientId": clientID, "text": text})
}
func (s *Service) Edit(peerID, messageID, text string) (Message, error) {
	return s.message(http.MethodPatch, peerID, messageID, map[string]string{"text": text})
}
func (s *Service) message(method, peerID, messageID string, body any) (Message, error) {
	r, err := s.active()
	if err != nil {
		return Message{}, err
	}
	path := prefix + "/" + url.PathEscape(peerID) + "/messages"
	if messageID != "" {
		path += "/" + url.PathEscape(messageID)
	}
	var out Message
	err = s.request(r, method, path, body, &out)
	return out, err
}
func (s *Service) React(peerID, messageID, emoji string) error {
	return s.reaction(http.MethodPut, peerID, messageID, emoji)
}
func (s *Service) Unreact(peerID, messageID, emoji string) error {
	return s.reaction(http.MethodDelete, peerID, messageID, emoji)
}
func (s *Service) reaction(method, peerID, messageID, emoji string) error {
	return s.action(method, peerID, "messages/"+url.PathEscape(messageID)+"/reactions/"+url.PathEscape(emoji), nil)
}
func (s *Service) Read(peerID, messageID string) error {
	return s.action(http.MethodPost, peerID, "read", map[string]string{"messageId": messageID})
}
func (s *Service) Typing(peerID string, typing bool) error {
	return s.action(http.MethodPost, peerID, "typing", map[string]bool{"typing": typing})
}
func (s *Service) action(method, peerID, path string, body any) error {
	r, err := s.active()
	if err != nil {
		return err
	}
	return s.request(r, method, prefix+"/"+url.PathEscape(peerID)+"/"+path, body, nil)
}
