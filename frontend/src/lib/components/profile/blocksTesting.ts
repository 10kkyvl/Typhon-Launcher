import { vi } from 'vitest';
import type { LibraryGame } from '../../services/library';

// Stand-ins of the tests of the profile's blocks. The game picker reads the catalog ids of the library's
// games from Go's profile service and, on a retry, the library itself from Go's library service; the
// blocks' settings and the picker of block kinds import the account service, whose functions reach Go
// with the player's session. So each test file that mounts a part of this folder replaces the three
// services whole (every function a mock; constants and types stay) through the factory here, and
// fakes their generated bindings, so a call that slipped past the mocks is recorded, not sent; the
// services run as inside Wails, so such a call does go on to its binding (blocks.mocks.test.ts):
//
//   vi.mock('../../services/backend', () => ({ inWails: true }));
//   vi.mock('../../../../bindings/typhon/internal/account', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'account'));
//   vi.mock('../../../../bindings/typhon/internal/library', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'library'));
//   vi.mock('../../../../bindings/typhon/internal/profile', async (original) => (await import('./blocksTesting')).fakeService(await original(), 'profile'));
//   vi.mock('../../services/account', async (original) => (await import('./blocksTesting')).wholeService(await original()));
//   vi.mock('../../services/library', async (original) => (await import('./blocksTesting')).wholeService(await original()));
//   vi.mock('../../services/profile', async (original) => (await import('./blocksTesting')).wholeService(await original()));
//
// The library store imports the library service through a cycle (services/library → stores/cloudsaves
// → stores/library): a file that imports both imports the store first. Games here are made up.

const isClass = (value: unknown) => typeof value === 'function' && /^class\b/.test(Function.prototype.toString.call(value));

/** Every function of `original` a mock that answers nothing; its constants and classes (an error type) as they are. */
export function wholeService<T extends object>(original: T): T {
  const out: Record<string, unknown> = {};
  for (const [name, value] of Object.entries(original)) out[name] = typeof value === 'function' && !isClass(value) ? vi.fn() : value;
  return out as T;
}

/** A generated binding whose `Service` records every call instead of reaching Go. */
export async function fakeService<T extends object>(original: T, prefix: string) {
  const { fakeBinding } = await import('../../testing/fakeBinding');
  return { ...original, Service: fakeBinding(prefix) };
}

/** A library game the picker can offer: one with a catalog id. */
export function libraryGame(id: string, title: string, patch: Partial<LibraryGame> = {}): LibraryGame {
  return { id, title, canonicalGameId: `canon-${id}`, hidden: false, ...patch } as LibraryGame;
}
