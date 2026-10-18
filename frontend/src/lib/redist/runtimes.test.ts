import { describe, expect, it } from 'vitest';
import { missingVcArches } from './runtimes';

const rt = (arch: 'x86' | 'x64', version: string) => ({ name: 'Visual C++ 2015-2022', version, arch });

describe('missingVcArches', () => {
  it('reports both when nothing is installed', () => {
    expect(missingVcArches([])).toEqual(['x64', 'x86']);
  });

  it('accepts a 14.x runtime for its own architecture only', () => {
    expect(missingVcArches([rt('x64', '14.38.33135')])).toEqual(['x86']);
    expect(missingVcArches([rt('x64', '14.38.33135'), rt('x86', '14.29.30133')])).toEqual([]);
  });

  it('does not count a pre-2015 runtime', () => {
    expect(missingVcArches([rt('x64', '12.0.40664')])).toEqual(['x64', 'x86']);
  });
});
