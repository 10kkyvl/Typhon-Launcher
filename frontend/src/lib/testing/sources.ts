const HERE = 'frontend/src/lib/testing';

function normalize(path: string): string {
  const parts: string[] = [];
  for (const part of `${HERE}/${path}`.split('/')) {
    if (part === '' || part === '.') continue;
    if (part === '..') parts.pop();
    else parts.push(part);
  }
  return parts.join('/');
}

function rooted(files: Record<string, string>, root: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [path, text] of Object.entries(files)) {
    const full = normalize(path);
    out[full.startsWith(root) ? full.slice(root.length) : full] = text;
  }
  return out;
}

export const frontendSources = rooted(
  import.meta.glob<string>(
    [
      '../../**/*.ts',
      '../../**/*.svelte',
      '!../../**/*.test.ts',
      '!../../**/*.d.ts',
      '!../../lib/testing/**',
      '!../../lib/i18n/catalog/**',
    ],
    { query: '?raw', import: 'default', eager: true },
  ),
  'frontend/src/',
);

export const goSources = rooted(
  import.meta.glob<string>(['../../../../internal/**/*.go', '!../../../../internal/**/*_test.go'], {
    query: '?raw',
    import: 'default',
    eager: true,
  }),
  '',
);

export const bindingSources = rooted(
  import.meta.glob<string>('../../../bindings/typhon/**/*.ts', {
    query: '?raw',
    import: 'default',
    eager: true,
  }),
  'frontend/bindings/',
);

export function sourcesUnder(prefix: string): Record<string, string> {
  return Object.fromEntries(Object.entries(frontendSources).filter(([path]) => path.startsWith(prefix)));
}
