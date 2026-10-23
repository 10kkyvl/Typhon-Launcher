import { Events } from '@wailsio/runtime';
import { TakePending } from '../../../bindings/typhon/navigationservice';
import { inWails } from '../services/backend';
import { launchRoute } from '../navigation/launchRoute';
import { navigate } from './router';

function go(value: unknown) {
  const route = launchRoute(value);
  if (route) navigate(route);
}

export function initLaunchNavigation(): () => void {
  if (!inWails) return () => {};
  const off = Events.On('app:navigate', (event) => {
    go((event.data as { route?: unknown } | undefined)?.route);
  });
  TakePending()
    .then(go)
    .catch((err) => console.error('take pending navigation', err));
  return off;
}
