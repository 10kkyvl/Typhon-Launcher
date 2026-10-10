import type { Installation } from '../services/install';

type Sized = Pick<Installation, 'status' | 'bytesTotal' | 'silent'>;

function noTotal(item: Sized): boolean {
  return item.status === 'installing' && item.bytesTotal <= 0;
}

export function installTotalUnknown(item: Sized): boolean {
  return noTotal(item) && item.silent;
}

export function installIndeterminate(item: Sized): boolean {
  return item.status === 'verifying' || noTotal(item);
}
