import { describe, expect, it, vi } from 'vitest';
const bindings = vi.hoisted(() => ({ Profile: vi.fn(), ProfileByCode: vi.fn() }));
vi.mock('../../../bindings/typhon/internal/social', () => ({ Service: bindings }));
vi.mock('./backend', () => ({ inWails: true }));
import { profile, profileByCode } from './social';
import { DEFAULT_APPEARANCE } from '../profile/appearance';

describe('public profile appearance bridge', () => {
  it('preserves cover, frame and explicit zero sliders in both profile lookups', async () => {
    const appearance = { ...DEFAULT_APPEARANCE, coverDim: 0, coverPosition: 0, coverUrl: 'https://cdn.test/cover.webp' };
    const user = { id: 'u', username: 'alice', displayName: 'Alice', avatarUrl: '', appearance };
    bindings.Profile.mockResolvedValue(user); bindings.ProfileByCode.mockResolvedValue(user);
    expect((await profile('alice')).appearance).toEqual(appearance);
    expect((await profileByCode('TY-1234-5678')).appearance).toEqual(appearance);
  });
  it('does not fabricate appearance hidden by privacy or absent on an older API', async () => {
    bindings.Profile.mockResolvedValue({ id: 'u', username: 'alice', appearance: undefined });
    expect((await profile('alice')).appearance).toBeUndefined();
  });
});
