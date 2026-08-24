package discord

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	opHandshake uint32 = 0
	opFrame     uint32 = 1
	opClose     uint32 = 2
	opPing      uint32 = 3
	opPong      uint32 = 4
)

const maxFrameLen = 64 * 1024

var (
	ErrNoTransport       = errors.New("не найден ни один IPC-канал Discord")
	ErrHandshakeRejected = errors.New("хендшейк Discord отклонён")
	ErrFrameTooLarge     = errors.New("кадр Discord превышает предельный размер")
	ErrConnectionClosed  = errors.New("соединение с Discord закрыто")
	ErrCommandRejected   = errors.New("discord отклонил команду")
	errMalformedFrame    = errors.New("битый кадр Discord")
	errDisabled          = errors.New("discord presence выключен")
)

type ipcFrame struct {
	op      uint32
	payload []byte
}

type handshakePayload struct {
	V        int    `json:"v"`
	ClientID string `json:"client_id"`
}

type dispatchEnvelope struct {
	Cmd string `json:"cmd"`
	Evt string `json:"evt"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type responseEnvelope struct {
	Cmd  string        `json:"cmd"`
	Evt  string        `json:"evt"`
	Data responseError `json:"data"`
}

type activityTimestamps struct {
	Start int64 `json:"start,omitempty"`
}

type activityAssets struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
	SmallImage string `json:"small_image,omitempty"`
	SmallText  string `json:"small_text,omitempty"`
}

type activityPayload struct {
	Type       int                 `json:"type"`
	Details    string              `json:"details,omitempty"`
	State      string              `json:"state,omitempty"`
	Timestamps *activityTimestamps `json:"timestamps,omitempty"`
	Assets     *activityAssets     `json:"assets,omitempty"`
}

type setActivityArgs struct {
	PID      int              `json:"pid"`
	Activity *activityPayload `json:"activity"`
}

type setActivityCommand struct {
	Cmd   string          `json:"cmd"`
	Nonce string          `json:"nonce"`
	Args  setActivityArgs `json:"args"`
}

func readFrame(r io.Reader) (ipcFrame, error) {
	var header [8]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return ipcFrame{}, fmt.Errorf("%w: %w", ErrConnectionClosed, err)
	}
	op := binary.LittleEndian.Uint32(header[0:4])
	length := binary.LittleEndian.Uint32(header[4:8])
	if length > maxFrameLen {
		return ipcFrame{}, fmt.Errorf("%w: %d байт", ErrFrameTooLarge, length)
	}
	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return ipcFrame{}, fmt.Errorf("%w: %w", ErrConnectionClosed, err)
		}
	}
	return ipcFrame{op: op, payload: payload}, nil
}

func writeFrame(w io.Writer, op uint32, payload []byte) error {
	if len(payload) > maxFrameLen {
		return fmt.Errorf("%w: %d байт", ErrFrameTooLarge, len(payload))
	}
	var header [8]byte
	binary.LittleEndian.PutUint32(header[0:4], op)
	binary.LittleEndian.PutUint32(header[4:8], uint32(len(payload))) //nolint:gosec // G115: len(payload) только что ограничен проверкой на maxFrameLen выше
	if _, err := w.Write(header[:]); err != nil {
		return fmt.Errorf("%w: %w", ErrConnectionClosed, err)
	}
	if len(payload) == 0 {
		return nil
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("%w: %w", ErrConnectionClosed, err)
	}
	return nil
}

func writeJSON(w io.Writer, op uint32, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("сериализация кадра Discord: %w", err)
	}
	return writeFrame(w, op, payload)
}

func handshake(rw io.ReadWriter, clientID string) error {
	if err := writeJSON(rw, opHandshake, handshakePayload{V: 1, ClientID: clientID}); err != nil {
		return err
	}
	f, err := readFrame(rw)
	if err != nil {
		return err
	}
	if f.op != opFrame {
		return fmt.Errorf("%w: opcode %d", ErrHandshakeRejected, f.op)
	}
	var env dispatchEnvelope
	if err := json.Unmarshal(f.payload, &env); err != nil {
		return fmt.Errorf("%w: %w", errMalformedFrame, err)
	}
	if env.Cmd != "DISPATCH" || env.Evt != "READY" {
		return fmt.Errorf("%w: cmd=%s evt=%s", ErrHandshakeRejected, env.Cmd, env.Evt)
	}
	return nil
}

func sendActivity(w io.Writer, nonce string, p *Presence) error {
	cmd := setActivityCommand{
		Cmd:   "SET_ACTIVITY",
		Nonce: nonce,
		Args:  setActivityArgs{PID: os.Getpid()},
	}
	if p != nil {
		act := &activityPayload{
			Type:    0,
			Details: p.Details,
			State:   p.State,
		}
		if !p.StartedAt.IsZero() {
			act.Timestamps = &activityTimestamps{Start: p.StartedAt.UnixMilli()}
		}
		if p.LargeImage != "" || p.LargeText != "" || p.SmallImage != "" || p.SmallText != "" {
			act.Assets = &activityAssets{
				LargeImage: p.LargeImage,
				LargeText:  p.LargeText,
				SmallImage: p.SmallImage,
				SmallText:  p.SmallText,
			}
		}
		cmd.Args.Activity = act
	}
	return writeJSON(w, opFrame, cmd)
}

func commandError(payload []byte) error {
	var env responseEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return fmt.Errorf("%w: %w", errMalformedFrame, err)
	}
	if env.Evt != "ERROR" {
		return nil
	}
	return fmt.Errorf("%w %s: код %d, %s", ErrCommandRejected, env.Cmd, env.Data.Code, env.Data.Message)
}
