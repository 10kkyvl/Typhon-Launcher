import { flushSync, mount, unmount } from 'svelte';
import { afterEach, describe, expect, it } from 'vitest';
import DownloadsAnnouncer from './DownloadsAnnouncer.svelte';

let mounted: Record<string, unknown>[] = [];

// Mounted by hand rather than through Testing Library: the test calls the component's `announce`.
function announcer() {
  const target = document.createElement('div');
  document.body.append(target);
  const component = mount(DownloadsAnnouncer, { target });
  mounted.push(component);
  const region = target.querySelector('[aria-live="polite"]') as HTMLElement;
  const say = (text: string) => {
    component.announce(text);
    flushSync();
  };
  return { region, say };
}

afterEach(async () => {
  for (const component of mounted) await unmount(component);
  mounted = [];
  document.body.replaceChildren();
});

// What the lists say through the screen's one polite region: what the player did, never progress.
describe('the downloads screen announcer', () => {
  it('says what it is given, once, in a polite region that is empty until then', () => {
    const { region, say } = announcer();
    expect(region.textContent?.trim()).toBe('');

    say('Celeste moved to place 2 of 3');

    expect(region.textContent?.trim()).toBe('Celeste moved to place 2 of 3');
    expect(region.querySelectorAll('span')).toHaveLength(1);
  });

  // A region that only changes its text says nothing when the text is the same: the second removal of
  // two downloads of one name must be heard too.
  it('says the same words again as a new node', () => {
    const { region, say } = announcer();

    say('Removed: Celeste');
    const first = region.querySelector('span');
    say('Removed: Celeste');

    expect(region.textContent?.trim()).toBe('Removed: Celeste');
    expect(region.querySelector('span')).not.toBe(first);
  });

  it('ignores empty words', () => {
    const { region, say } = announcer();
    say('Cleared');
    say('');

    expect(region.textContent?.trim()).toBe('Cleared');
  });
});
