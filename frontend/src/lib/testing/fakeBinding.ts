export interface Call {
  key: string;
  args: unknown[];
}

interface State {
  calls: Call[];
  answers: Map<string, unknown>;
}

const SLOT = '__typhonFakeBindings';

function state(): State {
  const holder = globalThis as unknown as Record<string, State | undefined>;
  holder[SLOT] ??= { calls: [], answers: new Map() };
  return holder[SLOT];
}

export const calls: Call[] = state().calls;

export function answer(key: string, value: unknown): void {
  state().answers.set(key, value);
}

export function resetBindings(): void {
  state().calls.length = 0;
  state().answers.clear();
}

export function fakeBinding(prefix: string): object {
  return new Proxy(
    {},
    {
      has: () => true,
      get: (_target, method) => {
        if (typeof method !== 'string' || method === 'then') return undefined;
        const key = `${prefix}.${method}`;
        return (...args: unknown[]) => {
          state().calls.push({ key, args });
          const value = state().answers.get(key);
          return Promise.resolve(typeof value === 'function' ? (value as (...a: unknown[]) => unknown)(...args) : value);
        };
      },
    },
  );
}
