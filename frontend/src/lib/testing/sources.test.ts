import { describe, expect, it } from 'vitest';
import { bindingSources, frontendSources, goSources, sourcesUnder } from './sources';

describe('frontend sources', () => {
  it('are keyed by their path under src', () => {
    expect(frontendSources['App.svelte']).toBeTypeOf('string');
    expect(frontendSources['lib/stores/router.ts']).toBeTypeOf('string');
    expect(frontendSources['routes/downloads/Downloads.svelte']).toBeTypeOf('string');
    expect(frontendSources['overlay/data.ts']).toBeTypeOf('string');
  });

  it('leave out tests, test helpers and the message catalogs', () => {
    const paths = Object.keys(frontendSources);

    expect(paths.filter((path) => path.endsWith('.test.ts'))).toEqual([]);
    expect(paths.filter((path) => path.startsWith('lib/testing/'))).toEqual([]);
    expect(paths.filter((path) => path.startsWith('lib/i18n/catalog/'))).toEqual([]);
  });

  it('can be narrowed to a directory', () => {
    const stores = sourcesUnder('lib/stores/');

    expect(Object.keys(stores).length).toBeGreaterThan(10);
    expect(Object.keys(stores).every((path) => path.startsWith('lib/stores/'))).toBe(true);
  });
});

describe('go sources', () => {
  it('are keyed by their path from the repository root', () => {
    expect(goSources['internal/account/errors.go']).toContain('package account');
    expect(goSources['internal/settings/settings.go']).toContain('package settings');
  });

  it('leave out the Go tests', () => {
    expect(Object.keys(goSources).filter((path) => path.endsWith('_test.go'))).toEqual([]);
  });
});

describe('generated bindings', () => {
  it('are keyed by their path under the bindings directory', () => {
    expect(bindingSources['typhon/internal/settings/models.ts']).toContain('export interface Settings');
    expect(bindingSources['typhon/internal/download/manager.ts']).toContain('export function List');
  });
});
