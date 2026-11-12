import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

// The shell finds the page's heading; the page only makes it focusable. A heading focus cannot reach
// is a page whose change leaves focus on <body>. The sign-in screen is outside the shell.
describe('every page heading can take focus', () => {
  const OUTSIDE = ['routes/auth/AuthScreen.svelte'];

  it('each h1 of a page or of the page header is focusable by script', () => {
    const headings = Object.entries(frontendSources)
      .filter(([path]) => path.endsWith('.svelte') && (path.startsWith('routes/') || path === 'lib/components/PageHeader.svelte') && !OUTSIDE.includes(path))
      .flatMap(([path, text]) => [...text.matchAll(/<h1\b[^>]*>/g)].map((m) => ({ path, tag: m[0] })));
    expect(headings.map(({ path }) => path).sort()).toEqual([
      'lib/components/PageHeader.svelte',
      'routes/game/GameDetails.svelte',
      'routes/support/TicketView.svelte',
    ]);
    expect(headings.filter(({ tag }) => !/\btabindex="-1"/.test(tag)).map(({ path }) => path)).toEqual([]);
  });
});
