import { describe, expect, it } from 'vitest';
import lightbox from './Lightbox.svelte?raw';
import chatPanel from './ChatPanel.svelte?raw';

function zIndex(source: string, selector: string): number {
  const block = source.match(new RegExp(`\\${selector}\\s*\\{([^}]*)\\}`));
  const value = block?.[1].match(/z-index:\s*(\d+)/);
  if (!value) throw new Error(`no z-index for ${selector}`);
  return Number(value[1]);
}

// Reproduces the user report: an image opened from the chat could not be
// dismissed by clicking around it, and its close button closed the launcher.
// The chat dock is pointer-events: none, so a viewer nested inside it let
// every click through to the window chrome underneath.
describe('Lightbox placement', () => {
  it('moves its backdrop to the document body', () => {
    expect(lightbox).toMatch(/function portal\(node: HTMLDivElement\) \{\s*document\.body\.appendChild\(node\);/);
    expect(lightbox).toMatch(/class="backdrop"\s+use:portal/);
  });

  it('takes clicks even when its parent lets them through', () => {
    expect(chatPanel).toMatch(/\.chat-root \{[^}]*pointer-events: none/);
    expect(lightbox).toMatch(/\.backdrop \{[^}]*pointer-events: auto/);
  });

  it('sits above the chat dock it is opened from', () => {
    expect(zIndex(lightbox, '.backdrop')).toBeGreaterThan(zIndex(chatPanel, '.chat-root'));
  });
});
