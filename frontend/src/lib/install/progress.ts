import type { Installation } from '../services/install';

type Sized = Pick<Installation, 'status' | 'bytesTotal'>;

export function installTotalUnknown(item: Sized): boolean {
  return item.status === 'installing' && item.bytesTotal <= 0;
}

export function installIndeterminate(item: Sized): boolean {
  return item.status === 'verifying' || installTotalUnknown(item);
}
