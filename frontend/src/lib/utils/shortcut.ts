// The search shortcut is drawn the way the keyboard labels it. navigator.platform
// is set by the web view before any script runs, while the Wails environment
// comes later, so the first render already shows the right keys.
export function onMacKeyboard(platform: string = navigator.platform): boolean {
  return /^mac/i.test(platform);
}

/**
 * A key that belongs to an input method's composition (Japanese, Chinese, Korean), not to the page: an
 * Enter here commits the composed text and must not send or submit. WebKit (the macOS web view) ends
 * the composition before that Enter, which then comes with `isComposing` off and key code 229.
 */
export function composing(event: KeyboardEvent): boolean {
  return event.isComposing || event.keyCode === 229;
}

export function searchShortcut(mac: boolean = onMacKeyboard()): string {
  return mac ? '⌘K' : 'Ctrl K';
}
