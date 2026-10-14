import { describe, expect, it } from 'vitest';
import { crashReasonText, redistCodeText } from './messages';

describe('crashReasonText', () => {
  it('translates every known code', () => {
    for (const code of ['dll_not_found', 'dll_init_failed', 'bad_image', 'access_violation', 'stack_overflow', 'dotnet_exception']) {
      expect(crashReasonText(code), code).not.toBe('');
    }
  });

  it('gives nothing for an empty or unknown code', () => {
    expect(crashReasonText('')).toBe('');
    expect(crashReasonText('weird')).toBe('');
  });
});

describe('redistCodeText', () => {
  it('knows the cancel failure', () => {
    expect(redistCodeText('redist.cancel_failed')).toContain('дождитесь');
  });
});
