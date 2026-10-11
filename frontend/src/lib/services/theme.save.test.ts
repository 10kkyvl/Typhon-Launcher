import { expect, it, vi } from 'vitest';

vi.mock('./backend', () => ({ inWails: true }));
vi.mock('../../../bindings/typhon/internal/theme', () => ({
  Service: {
    Save: vi.fn(async (theme: Record<string, unknown>) => ({ ...theme, id: 'dark-2', builtIn: false })),
  },
}));

import { Service } from '../../../bindings/typhon/internal/theme';
import { saveTheme } from './theme';

it('saves an edited built-in theme as a user theme so the backend forks it', async () => {
  const saved = await saveTheme({
    id: 'dark',
    name: 'Тёмная',
    base: 'dark',
    tokens: { '--accent': '#123456' },
    css: '',
    builtIn: true,
    updatedAt: '',
  });
  expect(Service.Save).toHaveBeenCalledWith(expect.objectContaining({ id: 'dark', builtIn: false }));
  expect(saved.id).toBe('dark-2');
  expect(saved.builtIn).toBe(false);
});
