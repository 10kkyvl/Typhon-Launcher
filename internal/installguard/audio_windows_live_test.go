//go:build windows

package installguard

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	ole "github.com/go-ole/go-ole"
)

// TYPHON_AUDIO_LIVE=1 plays a system sound in a child PowerShell, mutes its
// session through the real Core Audio API and unmutes it again, so the
// per-application mute Windows remembers for powershell.exe is not left on.
func TestLiveMuteChildProcess(t *testing.T) {
	if os.Getenv("TYPHON_AUDIO_LIVE") != "1" {
		t.Skip("set TYPHON_AUDIO_LIVE=1 to play and mute a real sound")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		t.Fatal(err)
	}
	defer ole.CoUninitialize()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
		`$p = New-Object Media.SoundPlayer "$env:WINDIR\Media\Windows Ding.wav"; $p.PlayLooping(); Start-Sleep 20`)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Log(err)
		}
		if err := cmd.Wait(); err != nil {
			t.Log(err)
		}
	}()
	//nolint:gosec // G115: a Windows process ID is a DWORD.
	pid := uint32(cmd.Process.Pid)
	owned := func(p uint32) bool { return p == pid }

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	silenced := 0
	for silenced == 0 {
		select {
		case <-ctx.Done():
			t.Fatal("the child's audio session never appeared")
		case <-ticker.C:
		}
		n, err := muteOwnedAudio(owned)
		if err != nil {
			t.Logf("mute pass: %v", err)
		}
		silenced = n
	}
	t.Logf("muted %d session(s) of pid %d", silenced, pid)

	sessions, err := renderSessions()
	if err != nil {
		t.Logf("list sessions: %v", err)
	}
	found := false
	for _, s := range sessions {
		p, err := s.processID()
		if err == nil && p == pid {
			muted, err := s.muted()
			if err != nil || !muted {
				t.Errorf("session of pid %d: muted %v, err %v", pid, muted, err)
			}
			if _, err := s.volume.call("ISimpleAudioVolume.SetMute", slotSetMute, 0, 0); err != nil {
				t.Errorf("unmute: %v", err)
			}
			found = true
		}
		s.release()
	}
	if !found {
		t.Fatal("muted session disappeared before it could be checked")
	}
}
