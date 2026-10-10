import { msg } from '../i18n';
import type { ConfirmPrompt } from '../confirm/prompts';

export function ticketLogsPrompt(): ConfirmPrompt {
  return {
    title: msg('tickets.logsConfirmTitle'),
    text: msg('tickets.logsConfirmText'),
    note: msg('tickets.logsConfirmNote'),
    confirm: msg('tickets.logsConfirmButton'),
    busy: msg('tickets.logsConfirmBusy'),
  };
}
