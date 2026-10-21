import { pathKey } from '../library/folders';
import type { SpaceInfo } from '../services/install';

export type DestinationOrigin = 'pick' | 'typed';

export interface DestinationHint {
  kind: 'adjusted' | 'suggest' | 'reusable';
  path: string;
}

export interface DestinationAdvice {
  destination: string;
  hint: DestinationHint | null;
}

type Result = Pick<SpaceInfo, 'destinationBusy' | 'suggested' | 'reusable'>;

export function adviseDestination(result: Result, origin: DestinationOrigin, destination: string): DestinationAdvice {
  if (result.destinationBusy) {
    const suggested = result.suggested;
    if (suggested !== '' && pathKey(suggested) !== pathKey(destination)) {
      if (origin === 'pick') return { destination: suggested, hint: { kind: 'adjusted', path: suggested } };
      return { destination, hint: { kind: 'suggest', path: suggested } };
    }
    return { destination, hint: null };
  }
  if (result.reusable) return { destination, hint: { kind: 'reusable', path: destination } };
  return { destination, hint: null };
}
