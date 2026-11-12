import type { Component } from 'svelte';

/**
 * The showcase of the base components (routes/showcase) exists in development builds only: `vite`
 * and the Wails development build (`vite build --mode development`, where `import.meta.env.DEV` is
 * false, so the mode is the gate). The mode is compared inline, where it is used, never through a
 * function: the production build folds the comparison to `false` and drops what it guards, the
 * page's chunk included. Behind a function call it is not folded, and the page ships as a chunk
 * nothing loads.
 */
export const SHOWCASE: boolean = import.meta.env.MODE === 'development';

/** The name of its sidebar entry. The page's words are its own: a dev-only page, never shipped, not translated. */
export const SHOWCASE_LABEL = 'Component showcase';

/** The page, or null outside development and when its chunk fails to load (logged). */
export const loadShowcase: () => Promise<Component | null> =
  import.meta.env.MODE === 'development'
    ? () =>
        import('../../routes/showcase/Showcase.svelte').then(
          (module) => module.default,
          (err: unknown) => {
            console.error('showcase', err);
            return null;
          },
        )
    : async () => null;
