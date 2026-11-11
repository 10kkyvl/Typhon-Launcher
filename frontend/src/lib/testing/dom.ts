import { render, type RenderResult } from '@testing-library/svelte';
import { userEvent, type UserEvent } from '@testing-library/user-event';
import type { Component, ComponentProps } from 'svelte';
import { vi } from 'vitest';

// For *.dom.test.ts only (the `dom` project in vite.config.ts). Cleanup of the DOM and the locale
// runs from vitest.dom.setup.ts.

// user-event pauses between steps with setTimeout; under fake timers it has to move the clock itself.
export function user(): UserEvent {
  if (!vi.isFakeTimers()) return userEvent.setup();
  return userEvent.setup({ advanceTimers: (ms) => vi.advanceTimersByTime(ms) });
}

type Props = Record<string, unknown>;

type PropsArg<P extends Props> = Record<string, never> extends P ? [props?: P] : [props: P];

export type Rendered<P extends Props> = RenderResult<Component<P>> & { user: UserEvent };

// Call vi.useFakeTimers() before this when the test drives timers, so `user` advances them.
export function renderDom<P extends Props>(component: Component<P>, ...[props]: PropsArg<P>): Rendered<P> {
  // The library's option type is conditional on the component, which TypeScript cannot resolve for
  // a generic P; `{ props }` is its documented shape.
  const options = { props: props ?? {} } as Parameters<typeof render<Component<P>>>[1];
  const result = render(component, options);
  return Object.assign(result, { user: user() });
}

// The parts of @wailsio/runtime a mounted component reaches through the stores and services:
//   vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
// Assert on them through `vi.mocked(Window.Minimise)` after importing `Window` from the runtime.
export function wailsRuntime() {
  return {
    Events: {
      Types: { Common: {}, Windows: {}, Mac: {} },
      On: vi.fn((_name: string | undefined, _cb: (event: unknown) => void) => () => {}),
      Emit: vi.fn(async () => {}),
    },
    Window: {
      Minimise: vi.fn(async () => {}),
      ToggleMaximise: vi.fn(async () => {}),
      Close: vi.fn(async () => {}),
      IsMaximised: vi.fn(async () => false),
    },
  };
}
