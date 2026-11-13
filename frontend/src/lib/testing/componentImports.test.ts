import { describe, expect, it } from 'vitest';
import { frontendSources } from './sources';

/**
 * Stores, services and utils hold state and logic; components draw it. A value import of a component
 * there (`@lucide/svelte`, whose barrel loads every icon, or any `.svelte` file) makes every store and
 * test that imports the module load the components too: the downloads store importing one icon was
 * enough to push the first test of the files that import it past vitest's 5 s timeout under the
 * default workers. A message that needs an icon names it by a word and the component maps the word.
 *
 * A type-only import (`import type`) loads nothing and is allowed. No exception is listed.
 */

const LAYERS = ['lib/stores/', 'lib/services/', 'lib/utils/'];
const FORBIDDEN = (specifier: string) => specifier === '@lucide/svelte' || specifier.startsWith('@lucide/svelte/') || /\.svelte$/.test(specifier);

/** Every module a source loads at run time: static imports and re-exports that are not type-only, side-effect imports, dynamic imports. */
function valueImports(text: string): string[] {
  const code = text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '');
  const found: string[] = [];
  for (const m of code.matchAll(/(?:^|[\n;])\s*(import|export)\s+(type\s+)?(?:[\w*{},\s$]+?\s+from\s+)?['"]([^'"]+)['"]/g)) {
    if (!m[2]) found.push(m[3]);
  }
  for (const m of code.matchAll(/\bimport\(\s*['"]([^'"]+)['"]\s*\)/g)) found.push(m[1]);
  return found;
}

describe('stores, services and utils import no component', () => {
  const files = Object.entries(frontendSources).filter(([path]) => path.endsWith('.ts') && LAYERS.some((layer) => path.startsWith(layer)));

  it('reads the three layers', () => {
    for (const layer of LAYERS) expect(files.some(([path]) => path.startsWith(layer)), layer).toBe(true);
    expect(files.length).toBeGreaterThan(100);
  });

  it('finds a value import however it is written, and lets a type-only one pass', () => {
    const sample = [
      "import { CircleAlert } from '@lucide/svelte';",
      "import {\n  Info,\n  TriangleAlert,\n} from '@lucide/svelte';",
      "import Avatar from '../components/Avatar.svelte';",
      "export { default as Card } from './Card.svelte';",
      "const page = import('../routes/Page.svelte');",
      "import type { LucideIcon } from '@lucide/svelte';",
      "import { motionOn } from '../motion/motion';",
      "import { state } from './counter.svelte.ts';",
      "// import { Gamepad2 } from '@lucide/svelte';",
    ].join('\n');
    expect(valueImports(sample).filter(FORBIDDEN)).toEqual(['@lucide/svelte', '@lucide/svelte', '../components/Avatar.svelte', './Card.svelte', '../routes/Page.svelte']);
  });

  it('none of them loads a component', () => {
    const found = files.flatMap(([path, text]) => valueImports(text).filter(FORBIDDEN).map((specifier) => `${path}: ${specifier}`));
    expect(found).toEqual([]);
  });
});
