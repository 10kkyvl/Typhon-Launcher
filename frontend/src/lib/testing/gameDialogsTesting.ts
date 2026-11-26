import { vi } from 'vitest';
import { wailsRuntime } from './dom';
import { calls, type Call } from './fakeBinding';

// The Wails runtime of the tests that mount a game or library dialog (lib/components/gameDialogs.mocks.test.ts).
// Every generated binding reaches Go through the runtime's `Call`, and the runtime opens pages in the
// browser and writes the clipboard: here each of those is recorded in `calls` (lib/testing/fakeBinding)
// instead of sent, so a call that slipped past a test's mocks (`library.PlayGame` starts a real game;
// the review report, the metadata search and the log export leave the device) is seen, not made. Each
// test file mocks the runtime with it and asserts that nothing was sent:
//
//   vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
//   afterEach(() => { const sent = sentToGo(); resetBindings(); ...; expect(sent).toEqual([]); });

/**
 * The id a generated binding calls Go by (`$Call.ByID`): the FNV-1a hash of the Go method's full name,
 * so `bindingId('library.PlayGame')` names a call recorded as `Call.ByID:<id>`.
 */
export function bindingId(name: string): number {
  let hash = 0x811c9dc5;
  for (const byte of new TextEncoder().encode(`typhon/internal/${name.replace('.', '.Service.')}`)) {
    hash ^= byte;
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return hash;
}

function recorder(key: string) {
  return vi.fn(async (...args: unknown[]) => {
    calls.push({ key, args });
    return undefined;
  });
}

export function goRuntime() {
  return {
    ...wailsRuntime(),
    Call: {
      ByID: vi.fn(async (id: number, ...args: unknown[]) => {
        calls.push({ key: `Call.ByID:${id}`, args });
        return undefined;
      }),
      ByName: vi.fn(async (name: string, ...args: unknown[]) => {
        calls.push({ key: `Call.ByName:${name}`, args });
        return undefined;
      }),
    },
    Browser: { OpenURL: recorder('Browser.OpenURL') },
    Clipboard: { SetText: recorder('Clipboard.SetText'), Text: recorder('Clipboard.Text') },
  };
}

/**
 * What the test sent towards Go. The metadata service tells Go the interface language when it loads
 * and whenever the locale changes (lib/services/metadata.ts): a setting of the launcher, not an action
 * of a dialog, so it is left out; everything else counts.
 */
export function sentToGo(): Call[] {
  const language = `Call.ByID:${bindingId('metadata.SetLanguage')}`;
  return calls.filter((call) => call.key !== language);
}

/** A promise the test settles by hand, so the state while a request runs can be looked at. */
export function held<T>() {
  let resolve: (value: T) => void = () => {};
  let reject: (err: unknown) => void = () => {};
  const promise = new Promise<T>((ok, fail) => {
    resolve = ok;
    reject = fail;
  });
  return { promise, resolve, reject };
}
