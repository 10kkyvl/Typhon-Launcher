import { sizeText, totalText } from '../../lib/diskusage/usage';
import { msg } from '../../lib/i18n';
import { bytesSize } from '../../lib/utils/format';

function format(bytes: number) {
  return bytesSize(bytes);
}

/** A row's size: unknown, at least, or exact. */
export function size(item: { sizeBytes: number; sizeUnknown: boolean }) {
  return sizeText(item.sizeBytes, item.sizeUnknown, format, msg('diskusage.sizeUnknown'));
}

/** A total, said as "at least" when a size under it is unknown. */
export function total(bytes: number, unknown: boolean) {
  return totalText(bytes, unknown, format, (value) => msg('diskusage.atLeast', { size: value }));
}
