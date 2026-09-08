import { msg } from '../i18n';

export interface ConfirmPrompt {
  title: string;
  text: string;
  note?: string;
  confirm: string;
  busy?: string;
  cancel?: string;
}

export function sendLogsPrompt(): ConfirmPrompt {
  return {
    title: msg('settings.aboutLogsSendConfirmTitle'),
    text: msg('settings.aboutLogsSendConfirmText'),
    note: msg('settings.aboutLogsSendConfirmNote'),
    confirm: msg('settings.aboutLogsSendConfirmButton'),
    busy: msg('settings.aboutLogsSendingEllipsis'),
  };
}
