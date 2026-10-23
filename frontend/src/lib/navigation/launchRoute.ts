import type { RouteName } from '../stores/router';

const LAUNCH_ROUTES: readonly RouteName[] = ['library', 'downloads'];

export function launchRoute(value: unknown): RouteName | null {
  if (typeof value !== 'string') return null;
  return LAUNCH_ROUTES.find((route) => route === value) ?? null;
}
