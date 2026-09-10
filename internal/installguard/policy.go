// Package installguard controls only windows belonging to an installer process tree.
package installguard

import "strings"

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
