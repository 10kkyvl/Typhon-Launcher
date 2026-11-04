// The search shortcut is drawn the way the keyboard labels it. navigator.platform
// is set by the web view before any script runs, while the Wails environment
// comes later, so the first render already shows the right keys.
export function onMacKeyboard(platform: string = navigator.platform): boolean {
  return /^mac/i.test(platform);
}

export function searchShortcut(mac: boolean = onMacKeyboard()): string {
  return mac ? '⌘K' : 'Ctrl K';
}
