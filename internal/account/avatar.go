package account

import (
	"bytes"
	"encoding/base64"
	"errors"
	"log/slog"
	"os"
)

type AvatarImage struct {
	Data string `json:"data"`
	MIME string `json:"mime"`
}

func avatarMIME(data []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png", true
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg", true
	case len(data) >= 12 && bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp", true
	}
	return "", false
}

func avatarImage(data []byte) (AvatarImage, error) {
	if len(data) == 0 {
		return AvatarImage{}, &Error{Code: CodeInvalidAvatar}
	}
	if len(data) > maxAvatarSize {
		return AvatarImage{}, &Error{Code: CodeAvatarTooLarge}
	}
	mime, ok := avatarMIME(data)
	if !ok {
		return AvatarImage{}, &Error{Code: CodeUnsupportedAvatar}
	}
	return AvatarImage{Data: base64.StdEncoding.EncodeToString(data), MIME: mime}, nil
}

func readAvatarImage(path string) (AvatarImage, error) {
	if path == "" {
		return AvatarImage{}, &Error{Code: CodeInvalidAvatar}
	}
	info, err := os.Stat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Error("stat avatar file", "error", err)
		}
		return AvatarImage{}, &Error{Code: CodeInvalidAvatar, cause: err}
	}
	if info.Size() > maxAvatarSize {
		return AvatarImage{}, &Error{Code: CodeAvatarTooLarge}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		slog.Error("read avatar file", "error", err)
		return AvatarImage{}, &Error{Code: CodeInvalidAvatar, cause: err}
	}
	return avatarImage(data)
}

func decodeAvatar(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, &Error{Code: CodeInvalidAvatar}
	}
	if base64.StdEncoding.DecodedLen(len(encoded)) > maxAvatarSize {
		return nil, &Error{Code: CodeAvatarTooLarge}
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, &Error{Code: CodeInvalidAvatar, cause: err}
	}
	if len(data) == 0 {
		return nil, &Error{Code: CodeInvalidAvatar}
	}
	if _, ok := avatarMIME(data); !ok {
		return nil, &Error{Code: CodeUnsupportedAvatar}
	}
	return data, nil
}
