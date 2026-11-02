import { describe, expect, it, vi } from 'vitest';

const start = vi.fn();

vi.mock('../../../bindings/typhon/internal/install', () => ({ Service: { Start: (...args: unknown[]) => start(...args) } }));
vi.mock('./backend', () => ({ inWails: true }));

const { isControlled, startInstall } = await import('./install');

describe('console image install', () => {
  it('treats an image as an install the launcher controls', () => {
    expect(isControlled('console_image')).toBe(true);
  });

  it('sends the card with the start of an image install', async () => {
    start.mockResolvedValue({ id: 'i1' });
    await startInstall('d1', { platform: 'ps2', title: 'Game', cover: 'https://img/c.jpg' });
    expect(start).toHaveBeenCalledWith('d1', {
      destination: '',
      mode: '',
      type: '',
      installerPath: '',
      platform: 'ps2',
      title: 'Game',
      cover: 'https://img/c.jpg',
    });
  });

  it('leaves the console fields out of an ordinary install', async () => {
    start.mockResolvedValue({ id: 'i1' });
    await startInstall('d1', { destination: 'D:/Games/G', mode: 'copy', type: 'portable' });
    expect(start.mock.lastCall?.[1]).not.toHaveProperty('platform');
  });

  it('passes the backend refusal on to the caller', async () => {
    start.mockImplementation(() => Promise.reject(new Error('typhon:install.image_not_found: нет образа')));
    let failure = '';
    try {
      await startInstall('d1', { platform: 'ps1', title: 'G' });
    } catch (err) {
      failure = String(err);
    }
    expect(failure).toContain('install.image_not_found');
  });
});
