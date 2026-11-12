import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

// What is for developers only (the showcase, a warning about a broken contract) is gated on the
// build mode. `import.meta.env.DEV` is false in the Wails development build, which is
// `vite build --mode development`: gated on it, the code never runs where the app is developed.
describe('development-only code', () => {
  const uses = (pattern: RegExp) =>
    Object.entries(frontendSources)
      .filter(([, text]) => pattern.test(text.replace(/\/\*[\s\S]*?\*\/|\/\/[^\n]*/g, '')))
      .map(([path]) => path)
      .sort();

  it('is never gated on import.meta.env.DEV or PROD', () => {
    expect(uses(/import\.meta\.env\.(?:DEV|PROD)\b/)).toEqual([]);
  });

  it('is gated on the mode where it is used', () => {
    expect(uses(/import\.meta\.env\.MODE === 'development'/)).toEqual(['lib/components/Notice.svelte', 'lib/navigation/showcase.ts']);
  });
});
