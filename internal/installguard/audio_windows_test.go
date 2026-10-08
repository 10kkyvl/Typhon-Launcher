//go:build windows

package installguard

import (
	"errors"
	"testing"
)

type fakeSession struct {
	pid     uint32
	pidErr  error
	isMuted bool
	muteErr error
	readErr error
	calls   int
}

func (s *fakeSession) processID() (uint32, error) { return s.pid, s.pidErr }

func (s *fakeSession) muted() (bool, error) { return s.isMuted, s.readErr }

func (s *fakeSession) mute() error {
	s.calls++
	if s.muteErr != nil {
		return s.muteErr
	}
	s.isMuted = true
	return nil
}

func TestMuteInstallerAudio(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name      string
		sessions  []*fakeSession
		wantCount int
		wantMuted []bool
		wantErr   bool
	}{
		{"installer session is muted", []*fakeSession{{pid: 10}}, 1, []bool{true}, false},
		{"other programs keep playing", []*fakeSession{{pid: 99}}, 0, []bool{false}, false},
		{"system sounds session has no process", []*fakeSession{{pid: 0}}, 0, []bool{false}, false},
		{"already muted session is left alone", []*fakeSession{{pid: 10, isMuted: true}}, 0, []bool{true}, false},
		{"every installer process is covered", []*fakeSession{{pid: 10}, {pid: 11}, {pid: 99}}, 2, []bool{true, true, false}, false},
		{"unreadable process does not stop the rest", []*fakeSession{{pid: 10, pidErr: boom}, {pid: 11}}, 1, []bool{false, true}, true},
		{"unreadable mute state is reported", []*fakeSession{{pid: 10, readErr: boom}}, 0, []bool{false}, true},
		{"failed mute is reported", []*fakeSession{{pid: 10, muteErr: boom}, {pid: 11}}, 1, []bool{false, true}, true},
		{"no sessions", nil, 0, nil, false},
	}
	owned := func(pid uint32) bool { return pid == 10 || pid == 11 }
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			view := make([]audioSession, len(c.sessions))
			for i, s := range c.sessions {
				view[i] = s
			}
			n, err := muteInstallerAudio(view, owned)
			if n != c.wantCount {
				t.Fatalf("silenced %d, want %d", n, c.wantCount)
			}
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, want error %v", err, c.wantErr)
			}
			if c.wantErr && !errors.Is(err, boom) {
				t.Fatalf("err = %v, want it to wrap the session error", err)
			}
			for i, s := range c.sessions {
				if s.isMuted != c.wantMuted[i] {
					t.Fatalf("session %d muted = %v, want %v", i, s.isMuted, c.wantMuted[i])
				}
			}
		})
	}
}

func TestAudioReportLogsEachErrorOnce(t *testing.T) {
	r := audioReport{}
	boom := errors.New("boom")
	r.note(0, boom)
	if r.lastErr != "boom" {
		t.Fatalf("lastErr = %q", r.lastErr)
	}
	r.note(2, nil)
	if r.lastErr != "" || r.silenced != 2 {
		t.Fatalf("after success: lastErr %q, silenced %d", r.lastErr, r.silenced)
	}
}
