import { render } from 'svelte/server';
import { describe, expect, it, vi } from 'vitest';
vi.mock('../services/backend', () => ({ inWails: false }));
import PlayerCard from './PlayerCard.svelte';
import type { UserCard } from '../services/social';

const base: UserCard = { id: '1', username: 'ann', displayName: 'Ann', avatarUrl: '' };

function html(user: UserCard, presence?: Record<string, unknown>) {
  return render(PlayerCard, { props: { user, presence } } as never).body;
}

describe('PlayerCard', () => {
  it('renders a plain card with the default accent when the backend sent no card', () => {
    const out = html(base);
    expect(out).toContain('Ann');
    expect(out).toContain('@ann');
    expect(out).toContain('--accent:#67d8ef');
    expect(out).not.toContain('pinned');
    expect(out).not.toContain('class="status"');
  });

  it('treats a null card like a missing one', () => {
    expect(html({ ...base, card: null })).toContain('--accent:#67d8ef');
  });

  it('renders the style, status and pinned game of a full card', () => {
    const out = html({
      ...base,
      card: {
        accent: '#ff8800', avatarFrame: 'neon', nameStyle: 'gradient', statusEmoji: '🎮', statusText: 'фармлю боссов',
        theme: 'custom', customFrom: '#401020', customTo: '#102040', customAngle: 45,
        coverUrl: 'https://cdn.example/cover.jpg',
        pinned: { igdbId: 1, title: 'Elden Ring', coverUrl: 'https://cdn.example/er.jpg', heroUrl: 'https://cdn.example/er-hero.jpg' },
      },
    });
    expect(out).toContain('frame-neon');
    expect(out).toContain('gradient');
    expect(out).toContain('🎮');
    expect(out).toContain('фармлю боссов');
    expect(out).toContain('Elden Ring');
    expect(out).toContain('linear-gradient(45deg, #401020, #102040)');
    expect(out).toContain('https://cdn.example/cover.jpg');
    expect(out).not.toContain('er-hero.jpg');
  });

  it('falls back to the pinned game art when there is no cover', () => {
    const out = html({
      ...base,
      card: { accent: '#ff8800', avatarFrame: 'none', nameStyle: 'plain', pinned: { igdbId: 1, title: 'Hades', coverUrl: 'c.jpg', heroUrl: 'hero.jpg' } },
    });
    expect(out).toContain('hero.jpg');
  });

  it('survives unknown style values from a newer backend', () => {
    const out = html({ ...base, card: { accent: 'nope', avatarFrame: 'hologram' as never, nameStyle: 'rainbow' as never } });
    expect(out).toContain('--accent:#67d8ef');
    expect(out).not.toContain('frame-hologram');
  });

  it('shows presence when it is passed', () => {
    const out = html(base, { status: 'online', gameId: 5, gameTitle: 'Hades' });
    expect(out).toContain('Hades');
  });
});
