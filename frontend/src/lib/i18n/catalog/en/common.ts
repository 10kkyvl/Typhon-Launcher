import type { Message } from '../../types';
import type { CommonKey } from '../ru/common';

export const common: Record<CommonKey, Message> = {
  'common.email': 'Email',
  'common.cancel': 'Cancel',
  'common.close': 'Close',
  'common.retry': 'Retry',
  'common.save': 'Save',
  'common.refresh': 'Refresh',
  'common.add': 'Add',
  'common.continue': 'Continue',
  'common.delete': 'Delete',
  'common.open': 'Open',
  'common.done': 'Done',
  'common.back': 'Back',
  'common.copy': 'Copy',
  'common.edit': 'Edit',
  'common.gotIt': 'Got it',
  'common.select': 'Select',
  'common.loading': 'Loading',
  'common.error': 'Error',
  // 7-A kit
  'common.meterAlt': '{label}: {details}',
  'common.meterPart': '{name}: {size}',
  'common.meterAtLeast': 'at least {size}',
  'common.meterSizeUnknown': 'size unknown',
  'common.meterFree': 'Free',
  'common.meterFreeOf': '{free} free of {total}',
  'common.meterTotal': '{total} in total',
  'common.meterTotalUnknown': 'Total size unknown',
  'common.chartPeriod': 'Period',
  'common.chartValue': 'Value',
  // /7-A kit
};
