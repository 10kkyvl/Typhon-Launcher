// Package installguard controls only windows belonging to an installer process tree.
package installguard

import "strings"

// Options selects what the guard does to a running installer tree.
type Options struct {
	HideProgress bool
	// VerifyRepack lets the repack's own file check (QuickSFV) run to completion.
	// Off, its window is closed on sight and the process is ended if it lingers.
	VerifyRepack bool
}

type verifierAction int

const (
	verifierObserve verifierAction = iota
	verifierClose
	verifierWait
	verifierTerminate
)

// verifierKillAfter is counted in guard passes (200 ms each): enough for
// QuickSFV to honour WM_CLOSE, short enough that a stuck hash is not waited out.
const verifierKillAfter = 25

func nextVerifierAction(verify, closed bool, waited int) verifierAction {
	switch {
	case verify:
		return verifierObserve
	case !closed:
		return verifierClose
	case waited < verifierKillAfter:
		return verifierWait
	default:
		return verifierTerminate
	}
}

// MusicState recognises explicit playback controls, not game soundtrack components.
func MusicState(label string) (checked bool, recognised bool) {
	label = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(label, "&", "")))
	label = strings.Join(strings.Fields(label), " ")
	switch label {
	case "music", "play music", "enable music", "music on", "installation music", "installer music", "background music",
		"музыка", "играть музыку", "включить музыку", "музыка установки", "фоновая музыка":
		return false, true
	case "mute music", "disable music", "music off", "no music", "выключить музыку", "отключить музыку", "без музыки":
		return true, true
	}
	return false, false
}

// OptionalSiteAction excludes game components, licenses and file verification.
func OptionalSiteAction(label string) bool {
	label = strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(label, "&", "")), " "))
	site := strings.Contains(label, "site") || strings.Contains(label, "web site") || strings.Contains(label, "сайт")
	action := strings.Contains(label, "visit") || strings.Contains(label, "open ") || strings.Contains(label, "посет") || strings.Contains(label, "перейти") || strings.Contains(label, "открыть")
	redirect := strings.Contains(label, "redirect") || strings.Contains(label, "перенаправ") || strings.Contains(label, "переадрес")
	return site && (action || redirect)
}
