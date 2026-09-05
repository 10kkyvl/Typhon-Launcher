import type { Message } from '../../types';
import type { FormatKey } from '../ru/format';

export const format: Record<FormatKey, Message> = {
  'units.b': 'B',
  'units.kb': 'KB',
  'units.mb': 'MB',
  'units.gb': 'GB',
  'units.tb': 'TB',
  'units.kbs': 'KB/s',
  'units.mbs': 'MB/s',
  'units.hour': 'h',
  'units.minute': 'min',
  'units.second': 's',
  'format.noLimit': 'Unlimited',
  'format.lessThanMinute': 'less than a minute',
  'format.today': 'Today',
  'format.yesterday': 'Yesterday',
  'format.daysAgo': { one: '{count} day ago', other: '{count} days ago' },
  'format.weeksAgo': { one: '{count} week ago', other: '{count} weeks ago' },
};
