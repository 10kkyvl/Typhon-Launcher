package messaging

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"typhon/internal/dialogtext"
	"typhon/internal/uierr"
)

const (
	maxChatImage      = 8 << 20
	maxUploadResponse = 2 << 20
	maxChatImages     = 4
	uploadTimeout     = 120 * time.Second
)

const (
	codeImageTooLarge      = "chat.image_too_large"
	codeImageNotImage      = "chat.image_not_image"
	codeImageUnreadable    = "chat.image_unreadable"
	codeAttachmentTooLarge = "chat.attachment_too_large"
	codeAttachmentNotImage = "chat.attachment_not_image"
	codeAttachmentInvalid  = "chat.attachment_invalid"
	codeTooManyImages      = "chat.too_many_attachments"
	codeRateLimited        = "chat.rate_limited"
	codeMuted              = "chat.muted"
)

var (
	errImageTooLarge   = uierr.New(codeImageTooLarge, "изображение больше 8 МиБ")
	errImageNotImage   = uierr.New(codeImageNotImage, "файл не является изображением PNG, JPEG, WebP или GIF")
	errTooManyImages   = uierr.New(codeTooManyImages, "в сообщение можно добавить не больше 4 изображений")
	errBadAttachment   = errors.New("chat attachment response is not valid")
	errUploadTooLarge  = errors.New("chat attachment response is too large")
	attachmentFailures = map[string]struct{ code, detail string }{
		"chat_attachment_too_large": {codeAttachmentTooLarge, "сервер отклонил изображение: слишком большое"},
		"chat_attachment_not_image": {codeAttachmentNotImage, "сервер отклонил файл: это не изображение"},
		"chat_attachment_invalid":   {codeAttachmentInvalid, "сервер отклонил изображение: файл повреждён"},
		"rate_limited":              {codeRateLimited, "слишком много загрузок, повторите позже"},
		"muted":                     {codeMuted, "отправка в чат временно отключена"},
	}
)

type ChatImage struct {
	Name    string `json:"name"`
	MIME    string `json:"mime"`
	Bytes   int64  `json:"bytes"`
	Encoded string `json:"encoded"`
}

func imageMIME(data []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png", true
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg", true
	case len(data) >= 12 && bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp", true
	case len(data) >= 6 && bytes.HasPrefix(data, []byte("GIF8")) && (data[4] == '7' || data[4] == '9') && data[5] == 'a':
		return "image/gif", true
	}
	return "", false
}

func chooseImageFiles(labels dialogtext.Labels) ([]string, error) {
	a := application.Get()
	if a == nil {
		return nil, errors.New("application is not running")
	}
	return a.Dialog.OpenFile().
		SetTitle(labels.ChatImageTitle).
		SetMessage(labels.ChatImageTitle).
		CanChooseFiles(true).
		AddFilter(labels.ChatImages, "*.png;*.jpg;*.jpeg;*.webp;*.gif").
		AddFilter(labels.AllFiles, "*.*").
		PromptForMultipleSelection()
}

// PickChatImages reads the chosen file into memory only; nothing is sent
// anywhere until UploadChatImage is called for it.
func (s *Service) PickChatImages(ctx context.Context, language string) ([]ChatImage, error) {
	paths, err := dialogtext.Selections(s.pickFiles(dialogtext.For(language)))
	if err != nil {
		return nil, fmt.Errorf("select chat images: %w", err)
	}
	if len(paths) > maxChatImages {
		return nil, errTooManyImages
	}
	images := make([]ChatImage, 0, len(paths))
	for _, path := range paths {
		image, err := readChatImage(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		images = append(images, image)
	}
	return images, nil
}

func checkAttachmentIDs(ids []string) error {
	if len(ids) > maxChatImages {
		return errTooManyImages
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return uierr.New(codeAttachmentInvalid, "пустой идентификатор изображения")
		}
		if _, dup := seen[id]; dup {
			return uierr.New(codeAttachmentInvalid, "одно изображение указано дважды")
		}
		seen[id] = struct{}{}
	}
	return nil
}

func readChatImage(ctx context.Context, path string) (ChatImage, error) {
	if err := ctx.Err(); err != nil {
		return ChatImage{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return ChatImage{}, uierr.Wrap(codeImageUnreadable, fmt.Errorf("не удалось открыть файл: %w", err))
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Debug("close chat image", "error", err)
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return ChatImage{}, uierr.Wrap(codeImageUnreadable, fmt.Errorf("не удалось прочитать файл: %w", err))
	}
	if !info.Mode().IsRegular() {
		return ChatImage{}, uierr.New(codeImageUnreadable, "выбранный объект не является обычным файлом")
	}
	data, err := readLimited(f)
	if err != nil {
		return ChatImage{}, err
	}
	mime, ok := imageMIME(data)
	if !ok {
		return ChatImage{}, errImageNotImage
	}
	return ChatImage{
		Name:    filepath.Base(path),
		MIME:    mime,
		Bytes:   int64(len(data)),
		Encoded: base64.StdEncoding.EncodeToString(data),
	}, nil
}

func readLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxChatImage+1))
	if err != nil {
		return nil, uierr.Wrap(codeImageUnreadable, fmt.Errorf("не удалось прочитать файл: %w", err))
	}
	if len(data) > maxChatImage {
		return nil, errImageTooLarge
	}
	return data, nil
}

func decodeChatImage(encoded string) ([]byte, error) {
	// DecodedLen counts padding bytes, so an image of exactly the limit may be
	// two bytes above it here; the exact size is checked after decoding.
	if base64.StdEncoding.DecodedLen(len(encoded)) > maxChatImage+2 {
		return nil, errImageTooLarge
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, uierr.Wrap(codeImageNotImage, fmt.Errorf("изображение передано не в base64: %w", err))
	}
	if len(data) > maxChatImage {
		return nil, errImageTooLarge
	}
	if _, ok := imageMIME(data); !ok {
		return nil, errImageNotImage
	}
	return data, nil
}

// UploadChatImage sends the bytes to the chat server on an explicit user
// action and returns the attachment to reference from Send.
func (s *Service) UploadChatImage(ctx context.Context, encoded string) (Attachment, error) {
	data, err := decodeChatImage(encoded)
	if err != nil {
		return Attachment{}, err
	}
	r, err := s.active()
	if err != nil {
		return Attachment{}, err
	}
	return s.upload(ctx, r, data)
}

func (s *Service) upload(caller context.Context, r *session, data []byte) (Attachment, error) {
	if err := s.sessionError(r); err != nil {
		return Attachment{}, err
	}
	ctx, cancel := context.WithTimeout(caller, uploadTimeout)
	defer cancel()
	stop := r.afterEnd(cancel)
	defer stop()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.base+prefix+"/attachments", bytes.NewReader(data))
	if err != nil {
		return Attachment{}, err
	}
	authorize(req, r)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := s.http.Do(req)
	if err != nil {
		return Attachment{}, fmt.Errorf("messaging upload: %w", err)
	}
	defer closeResponseBody(resp.Body)
	if !s.valid(r) {
		return Attachment{}, errSignedOut
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Attachment{}, attachmentError(responseError(resp))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUploadResponse+1))
	if err != nil {
		return Attachment{}, fmt.Errorf("read chat attachment response: %w", err)
	}
	if len(body) > maxUploadResponse {
		return Attachment{}, errUploadTooLarge
	}
	var out Attachment
	if err = json.Unmarshal(body, &out); err != nil {
		return Attachment{}, fmt.Errorf("decode chat attachment response: %w", err)
	}
	if err = validAttachment(out); err != nil {
		return Attachment{}, err
	}
	if !s.valid(r) {
		return Attachment{}, errSignedOut
	}
	return out, nil
}

// An upload follows the caller's context and also ends with the session.
func (r *session) afterEnd(cancel context.CancelFunc) func() bool {
	return context.AfterFunc(r.ctx, cancel)
}

func validAttachment(a Attachment) error {
	if a.ID == "" {
		return fmt.Errorf("%w: empty id", errBadAttachment)
	}
	u, err := url.Parse(a.URL)
	if err != nil {
		return fmt.Errorf("%w: %w", errBadAttachment, err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("%w: url must be https", errBadAttachment)
	}
	return nil
}

func attachmentError(err error) error {
	var failure *responseFailure
	if !errors.As(err, &failure) {
		return err
	}
	known, ok := attachmentFailures[failure.code]
	if !ok && failure.status == http.StatusRequestEntityTooLarge {
		known.code, known.detail = codeAttachmentTooLarge, attachmentFailures["chat_attachment_too_large"].detail
		ok = true
	}
	if !ok {
		return err
	}
	return uierr.Wrap(known.code, fmt.Errorf("%s: %w", known.detail, err))
}
