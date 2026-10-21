package messaging

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"typhon/internal/dialogtext"
	"typhon/internal/uierr"
)

func TestPickChatImagesCancelledDialogIsNotAnError(t *testing.T) {
	s := &Service{pickFiles: func(dialogtext.Labels) ([]string, error) { return nil, errors.New("cancelled by user") }}
	got, err := s.PickChatImages(context.Background(), "ru")
	if err != nil || len(got) != 0 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestPickChatImagesDialogFailureAndLabels(t *testing.T) {
	boom := errors.New("dialog unavailable")
	var labels dialogtext.Labels
	s := &Service{pickFiles: func(l dialogtext.Labels) ([]string, error) { labels = l; return nil, boom }}
	if _, err := s.PickChatImages(context.Background(), "en"); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	if labels != dialogtext.For("en") {
		t.Fatalf("labels for the resolved language were not used: %+v", labels)
	}
}

func TestPickChatImagesReadsEveryChosenFile(t *testing.T) {
	first := writeFile(t, "one.png", pngBytes(64))
	second := writeFile(t, "two.gif", append([]byte("GIF89a"), 1, 2))
	s := &Service{pickFiles: func(dialogtext.Labels) ([]string, error) { return []string{first, second}, nil }}
	got, err := s.PickChatImages(context.Background(), "ru")
	if err != nil || len(got) != 2 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if got[0].Name != "one.png" || got[0].Bytes != 64 || got[1].Name != "two.gif" || got[1].MIME != "image/gif" {
		t.Fatalf("got=%+v", got)
	}
}

func TestPickChatImagesChecksEachFileOnItsOwn(t *testing.T) {
	good := writeFile(t, "ok.png", pngBytes(64))
	bad := writeFile(t, "note.png", []byte("not a picture"))
	big := writeFile(t, "big.png", pngBytes(maxChatImage+1))
	tests := []struct {
		name  string
		paths []string
		code  string
	}{
		{"second is not an image", []string{good, bad}, codeImageNotImage},
		{"last is too large", []string{good, good, big}, codeImageTooLarge},
		{"more than four", []string{good, good, good, good, good}, codeTooManyImages},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{pickFiles: func(dialogtext.Labels) ([]string, error) { return tt.paths, nil }}
			got, err := s.PickChatImages(context.Background(), "ru")
			if uierr.Code(err) != tt.code || got != nil {
				t.Fatalf("code=%q got=%v err=%v", uierr.Code(err), got, err)
			}
		})
	}
	s := &Service{pickFiles: func(dialogtext.Labels) ([]string, error) { return []string{good, good, good, good}, nil }}
	if got, err := s.PickChatImages(context.Background(), "ru"); err != nil || len(got) != 4 {
		t.Fatalf("four files must be accepted: %d err=%v", len(got), err)
	}
}

func TestUploadChatImageAcceptsCreatedAndOK(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusCreated} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				if _, err := fmt.Fprint(w, attachmentJSON); err != nil {
					t.Error(err)
				}
			})
			got, err := h.svc.UploadChatImage(context.Background(), base64.StdEncoding.EncodeToString(pngBytes(32)))
			if err != nil || got.ID != "a1" {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}

func TestSendWithAttachments(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		if _, err := fmt.Fprint(w, `{"id":"42","text":"","attachments":[{"id":"a1","url":"https://cdn.example/a1.webp","width":640,"height":480,"bytes":1234},{"id":"a2","url":"https://cdn.example/a2.webp","width":1,"height":2,"bytes":3}]}`); err != nil {
			t.Error(err)
		}
	})
	m, err := h.svc.Send("b", "c1", "", []string{"a1", "a2"})
	if err != nil || len(m.Attachments) != 2 || m.Attachments[0].ID != "a1" || m.Attachments[1].ID != "a2" {
		t.Fatalf("m=%+v err=%v", m, err)
	}
	for _, none := range [][]string{nil, {}} {
		if _, err := h.svc.Send("b", "c2", "hello", none); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	ids, ok := bodies[0]["attachmentIds"].([]any)
	if !ok || len(ids) != 2 || ids[0] != "a1" || ids[1] != "a2" || bodies[0]["text"] != "" || bodies[0]["clientId"] != "c1" {
		t.Fatalf("body=%v", bodies[0])
	}
	for _, body := range bodies[1:] {
		if _, has := body["attachmentIds"]; has {
			t.Fatalf("empty list must not be sent: %v", body)
		}
	}
}

func TestSendValidatesTextAndAttachments(t *testing.T) {
	var mu sync.Mutex
	var hits int
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		if _, err := fmt.Fprint(w, `{"id":"1"}`); err != nil {
			t.Error(err)
		}
	})
	tests := []struct {
		name, text string
		ids        []string
		want       error
		code       string
	}{
		{"nothing", "", nil, errMessageEmpty, ""},
		{"whitespace only", " \n\t", []string{}, errMessageEmpty, ""},
		{"too long", strings.Repeat("я", maxMessageRunes+1), nil, errMessageTooLong, ""},
		{"too long with attachments", strings.Repeat("a", maxMessageRunes+1), []string{"a1"}, errMessageTooLong, ""},
		{"five attachments", "", []string{"a", "b", "c", "d", "e"}, nil, codeTooManyImages},
		{"empty id", "", []string{"a1", ""}, nil, codeAttachmentInvalid},
		{"duplicate id", "x", []string{"a1", "a1"}, nil, codeAttachmentInvalid},
		{"limit in runes, not bytes", strings.Repeat("я", maxMessageRunes), nil, nil, ""},
		{"attachments only", "", []string{"a1"}, nil, ""},
		{"four attachments", "", []string{"a", "b", "c", "d"}, nil, ""},
	}
	valid := 0
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.svc.Send("b", "c", tt.text, tt.ids)
			switch {
			case tt.want != nil:
				if !errors.Is(err, tt.want) {
					t.Fatalf("err=%v want=%v", err, tt.want)
				}
			case tt.code != "":
				if uierr.Code(err) != tt.code {
					t.Fatalf("code=%q err=%v", uierr.Code(err), err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
				valid++
			}
		})
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != valid || valid != 3 {
		t.Fatalf("server hits=%d, valid sends=%d, want 3 each", hits, valid)
	}
}

func TestMessageAttachmentsFromHistory(t *testing.T) {
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"messages":[{"id":"1","text":"","attachments":[{"id":"a1","url":"https://cdn.example/a1.webp","width":10,"height":20,"bytes":30},{"id":"a2","url":"https://cdn.example/a2.webp","width":1,"height":2,"bytes":3}]},{"id":"2","text":"plain","attachments":null},{"id":"3","text":"old"}],"next":"","canSend":true}`); err != nil {
			t.Error(err)
		}
	})
	page, err := h.svc.Messages("b", "")
	if err != nil || len(page.Messages) != 3 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	got := page.Messages[0].Attachments
	if len(got) != 2 || got[0] != (Attachment{ID: "a1", URL: "https://cdn.example/a1.webp", Width: 10, Height: 20, Bytes: 30}) || got[1].ID != "a2" {
		t.Fatalf("attachments=%+v", got)
	}
	for _, m := range page.Messages[1:] {
		if m.Attachments == nil || len(m.Attachments) != 0 {
			t.Fatalf("message %s: want an empty non-nil list, got %#v", m.ID, m.Attachments)
		}
	}
	encoded, err := json.Marshal(page.Messages[2])
	if err != nil || !strings.Contains(string(encoded), `"attachments":[]`) {
		t.Fatalf("UI payload=%s err=%v", encoded, err)
	}
}
