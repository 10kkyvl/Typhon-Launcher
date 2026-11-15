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
  // One saying per pass, as the lists say what the player did: the pass ends before the next one.
  const say = async (text: string) => {
    component.announce(text);
    flushSync();
    await new Promise((done) => setTimeout(done, 0));
  };
  return { region, say, component };
}

afterEach(async () => {
  for (const component of mounted) await unmount(component);
  mounted = [];
  document.body.replaceChildren();
});

// What the lists say through the screen's one polite region: what the player did, never progress.
describe('the downloads screen announcer', () => {
  it('says what it is given, once, in a polite region that is empty until then', async () => {
    const { region, say } = announcer();
    expect(region.textContent?.trim()).toBe('');

    await say('Celeste moved to place 2 of 3');

    expect(region.textContent?.trim()).toBe('Celeste moved to place 2 of 3');
    expect(region.querySelectorAll('span')).toHaveLength(1);
  });

  // A region that only changes its text says nothing when the text is the same: the second removal of
  // two downloads of one name must be heard too.
  it('says the same words again as a new node', async () => {
    const { region, say } = announcer();

    await say('Removed: Celeste');
    const first = region.querySelector('span');
    await say('Removed: Celeste');

    expect(region.textContent?.trim()).toBe('Removed: Celeste');
    expect(region.querySelector('span')).not.toBe(first);
  });

  it('two sayings in one pass are two nodes, in order', () => {
    const { region, component } = announcer();

    component.announce('Celeste removed from the list');
    component.announce('Hades removed from the list');
    flushSync();

    expect(Array.from(region.querySelectorAll('span')).map((span) => span.textContent)).toEqual([
      'Celeste removed from the list',
      'Hades removed from the list',
    ]);
  });

  it('a later saying replaces what was said before', async () => {
    const { region, say } = announcer();

    await say('Celeste removed from the list');
    await say('Hades removed from the list');

    expect(Array.from(region.querySelectorAll('span')).map((span) => span.textContent)).toEqual(['Hades removed from the list']);
  });

  it('ignores empty words', async () => {
    const { region, say } = announcer();
    await say('Cleared');
    await say('');

    expect(region.textContent?.trim()).toBe('Cleared');
  });
});
