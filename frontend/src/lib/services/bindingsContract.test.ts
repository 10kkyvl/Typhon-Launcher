import { describe, expect, it } from 'vitest';
import { balanced } from '../testing/scan';
import { bindingSources, frontendSources, goSources } from '../testing/sources';

const MODULE = String.raw`bindings\/typhon\/internal\/(\w+)(?:\/(\w+))?`;
const IMPORT = new RegExp(
  String.raw`import\s+(?:type\s+)?(?:\*\s+as\s+(\w+)|\{([^}]*)\})\s+from\s+['"][^'"]*${MODULE}['"]`,
  'g',
);

interface BindingImport {
  file: string;
  pkg: string;
  sub: string;
  namespace: string | null;
  names: Array<{ name: string; alias: string }>;
}

function bindingImports(): BindingImport[] {
  const found: BindingImport[] = [];
  for (const [file, text] of Object.entries(frontendSources)) {
    for (const match of text.matchAll(IMPORT)) {
      const names = (match[2] ?? '')
        .split(',')
        .map((part) => part.trim())
        .filter(Boolean)
        .map((part) => {
          const [name, alias] = part.replace(/^type\s+/, '').split(/\s+as\s+/);
          return { name, alias: alias ?? name };
        });
      found.push({
        file,
        pkg: match[3],
        sub: match[4] ?? '',
        namespace: match[1] ?? null,
        names,
      });
    }
  }
  return found;
}

function modulePath(imp: Pick<BindingImport, 'pkg' | 'sub'>): string {
  return `typhon/internal/${imp.pkg}/${imp.sub || 'index'}.ts`;
}

function exportedNames(source: string): Set<string> {
  const names = new Set<string>();
  for (const match of source.matchAll(/export\s+(?:type\s+)?\{([^}]*)\}/g)) {
    for (const part of match[1].split(',')) {
      const name = part.trim().split(/\s+as\s+/).pop();
      if (name) names.add(name);
    }
  }
  for (const match of source.matchAll(/export\s+(?:declare\s+)?(?:interface|class|enum|const|type|function)\s+(\w+)/g)) {
    names.add(match[1]);
  }
  return names;
}

function serviceModules(pkg: string): Map<string, string> {
  const index = bindingSources[`typhon/internal/${pkg}/index.ts`] ?? '';
  const modules = new Map<string, string>();
  for (const match of index.matchAll(/import\s+\*\s+as\s+(\w+)\s+from\s+"\.\/(\w+)\.js"/g)) {
    modules.set(match[1], `typhon/internal/${pkg}/${match[2]}.ts`);
  }
  return modules;
}

function exportedFunctions(source: string): string[] {
  return [...source.matchAll(/export function (\w+)\(/g)].map((match) => match[1]);
}

interface Use {
  file: string;
  pkg: string;
  type: string;
  method: string;
}

function methodUses(): Use[] {
  const uses: Use[] = [];
  for (const imp of bindingImports()) {
    const text = frontendSources[imp.file];
    const bound: Array<{ alias: string; type: string }> = [];
    if (imp.namespace && imp.sub) {
      bound.push({ alias: imp.namespace, type: imp.sub[0].toUpperCase() + imp.sub.slice(1) });
    }
    if (!imp.sub) {
      const services = serviceModules(imp.pkg);
      for (const { name, alias } of imp.names) {
        if (services.has(name)) bound.push({ alias, type: name });
      }
    }
    for (const { alias, type } of bound) {
      const calls = new RegExp(String.raw`(?<![\w.$])${alias}\.(\w+)`, 'g');
      for (const call of text.matchAll(calls)) {
        uses.push({ file: imp.file, pkg: imp.pkg, type, method: call[1] });
      }
    }
  }
  return uses;
}

function goMethods(pkg: string, type: string): Set<string> {
  const names = new Set<string>();
  const pattern = new RegExp(String.raw`func\s+\(\s*\w+\s+\*?${type}\s*\)\s+([A-Z]\w*)\s*\(`, 'g');
  for (const [path, text] of Object.entries(goSources)) {
    if (!path.startsWith(`internal/${pkg}/`) || path.slice(`internal/${pkg}/`.length).includes('/')) continue;
    for (const match of text.matchAll(pattern)) names.add(match[1]);
  }
  return names;
}

describe('generated bindings', () => {
  const imports = bindingImports();

  it('are present for the tests that rely on them', () => {
    expect(Object.keys(bindingSources).length).toBeGreaterThan(50);
    expect(imports.length).toBeGreaterThan(40);
  });

  it('has a module for every binding the interface imports', () => {
    const missing = imports.filter((imp) => bindingSources[modulePath(imp)] === undefined);
    expect(missing.map((imp) => `${imp.file} -> ${modulePath(imp)}`)).toEqual([]);
  });

  it('exports every name the interface imports from them', () => {
    const missing: string[] = [];
    for (const imp of imports) {
      const source = bindingSources[modulePath(imp)];
      if (source === undefined) continue;
      const exported = exportedNames(source);
      for (const { name } of imp.names) {
        if (!exported.has(name)) missing.push(`${imp.file}: ${name} from ${modulePath(imp)}`);
      }
    }
    expect(missing).toEqual([]);
  });

  it('declares every service method the interface calls', () => {
    const uses = methodUses();
    expect(uses.length).toBeGreaterThan(150);
    const missing: string[] = [];
    for (const use of uses) {
      const file = serviceModules(use.pkg).get(use.type) ?? `typhon/internal/${use.pkg}/${use.type.toLowerCase()}.ts`;
      const source = bindingSources[file];
      if (source === undefined || !exportedFunctions(source).includes(use.method)) {
        missing.push(`${use.file}: ${use.type}.${use.method} in ${use.pkg}`);
      }
    }
    expect([...new Set(missing)]).toEqual([]);
  });

  it('match the Go services they were generated from', () => {
    const stale: string[] = [];
    for (const path of Object.keys(bindingSources)) {
      const match = /^typhon\/internal\/(\w+)\/(service|manager)\.ts$/.exec(path);
      if (!match) continue;
      const type = match[2][0].toUpperCase() + match[2].slice(1);
      const methods = goMethods(match[1], type);
      for (const name of exportedFunctions(bindingSources[path])) {
        if (!methods.has(name)) stale.push(`${path}: ${name}`);
      }
    }
    expect(stale, 'regenerate with: wails3 task common:generate:bindings').toEqual([]);
  });

  it('call methods that exist on the Go side, not only in the generated files', () => {
    const missing: string[] = [];
    for (const use of methodUses()) {
      if (!goMethods(use.pkg, use.type).has(use.method)) {
        missing.push(`${use.file}: ${use.type}.${use.method} in internal/${use.pkg}`);
      }
    }
    expect([...new Set(missing)]).toEqual([]);
  });
});

describe('runtime events', () => {
  function subscribedEvents(): Array<{ file: string; name: string }> {
    const found: Array<{ file: string; name: string }> = [];
    for (const [file, text] of Object.entries(frontendSources)) {
      for (const match of text.matchAll(/Events\.(?:On|Once)\(/g)) {
        const args = balanced(text, text.indexOf('(', match.index));
        const literal = args === null ? null : /^\s*(['"])([A-Za-z0-9:_.-]+)\1/.exec(args);
        if (literal) found.push({ file, name: literal[2] });
      }
    }
    return found;
  }

  it('are all raised somewhere in the Go sources', () => {
    const events = subscribedEvents();
    expect(events.length).toBeGreaterThan(40);
    const gone = events.filter(({ name }) => !Object.values(goSources).some((text) => text.includes(`"${name}"`)));
    expect(gone.map(({ file, name }) => `${file}: ${name}`)).toEqual([]);
  });
});
