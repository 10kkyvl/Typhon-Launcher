import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../services/sources', () => ({ openByIGDB: vi.fn() }));
vi.mock('../stores/router', () => ({ navigate: vi.fn() }));
vi.mock('../stores/toasts', () => ({ toast: vi.fn() }));

const { openByIGDB } = await import('../services/sources');
const { navigate } = await import('../stores/router');
const { toast } = await import('../stores/toasts');
const { openGameByIGDB } = await import('./openGame');

describe('openGameByIGDB', () => {
  beforeEach(() => {
    vi.mocked(openByIGDB).mockReset();
    vi.mocked(navigate).mockReset();
    vi.mocked(toast).mockReset();
  });

  it('открывает страницу игры по каноническому id, а не по igdb', async () => {
    vi.mocked(openByIGDB).mockResolvedValue({ id: 'game-7' } as never);

    await openGameByIGDB(1877, 'Cyberpunk 2077');

    expect(openByIGDB).toHaveBeenCalledWith(1877, 'Cyberpunk 2077');
    expect(navigate).toHaveBeenCalledWith('game', { id: 'game-7' });
    expect(toast).not.toHaveBeenCalled();
  });

  it('без id никуда не уходим и говорим об этом', async () => {
    vi.mocked(openByIGDB).mockResolvedValue({ id: '' } as never);

    await openGameByIGDB(1877, 'Cyberpunk 2077');

    expect(navigate).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith('Не удалось открыть игру', 'danger');
  });

  it('ошибка бэкенда не роняет страницу профиля', async () => {
    vi.mocked(openByIGDB).mockRejectedValue(new Error('boom'));

    await expect(openGameByIGDB(1877, 'Cyberpunk 2077')).resolves.toBeUndefined();

    expect(navigate).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledWith('Не удалось открыть игру', 'danger');
  });
});
