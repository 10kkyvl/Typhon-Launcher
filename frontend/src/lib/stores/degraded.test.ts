import { describe, it, expect, beforeEach, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));

const { applyDegraded, degraded } = await import('./degraded');

describe('applyDegraded', () => {
  beforeEach(() => degraded.set({}));

  it('уведомляет один раз на подсистему, пока она не восстановится', () => {
    expect(applyDegraded('download:degraded', { degraded: true, message: 'disk full' })).toBe(true);
    expect(applyDegraded('download:degraded', { degraded: true, message: 'disk full' })).toBe(false);
    expect(get(degraded)).toEqual({ 'download:degraded': 'disk full' });
  });

  it('уведомляет о каждой подсистеме отдельно', () => {
    expect(applyDegraded('download:degraded', { degraded: true, message: 'a' })).toBe(true);
    expect(applyDegraded('update:degraded', { degraded: true, message: 'b' })).toBe(true);
    expect(get(degraded)).toEqual({ 'download:degraded': 'a', 'update:degraded': 'b' });
  });

  it('снимает запись при восстановлении и уведомляет снова после него', () => {
    applyDegraded('move:degraded', { degraded: true, message: 'a' });
    expect(applyDegraded('move:degraded', { degraded: false, message: '' })).toBe(false);
    expect(get(degraded)).toEqual({});
    expect(applyDegraded('move:degraded', { degraded: true, message: 'a' })).toBe(true);
  });
});
