import { msg } from '../i18n';

export interface ConfirmPrompt {
  title: string;
  text: string;
  note?: string;
  confirm: string;
  busy?: string;
  cancel?: string;
}

export function unfriendPrompt(name: string): ConfirmPrompt {
  return {
    title: msg('social.unfriendLabel'),
    text: msg('social.unfriendConfirmText', { name }),
    confirm: msg('social.unfriendLabel'),
  };
}

export function blockPrompt(name: string, friend: boolean): ConfirmPrompt {
  return {
    title: msg('social.blockLabel'),
    text: msg('social.blockConfirmText', { name }),
    note: friend ? msg('social.blockConfirmFriendNote') : undefined,
    confirm: msg('social.blockLabel'),
  };
}

export function removeDownloadPrompt(name: string): ConfirmPrompt {
  return {
    title: msg('games.detailRemoveDownloadLabel'),
    text: msg('games.detailRemoveDownloadConfirm', { name }),
    confirm: msg('games.detailRemoveDownloadLabel'),
  };
}

export function discardDownloadPrompt(name: string): ConfirmPrompt {
  return {
    title: msg('games.detailDiscardDownloadLabel'),
    text: msg('games.detailDiscardDownloadConfirm', { name }),
    confirm: msg('games.detailDiscardDownloadLabel'),
  };
}

export function cancelDownloadPrompt(name: string): ConfirmPrompt {
  return {
    title: msg('ui.cancelDownloadTitle'),
    text: msg('ui.cancelDownloadWarning', { name }),
    confirm: msg('ui.cancelDownload'),
    cancel: msg('ui.dontCancel'),
  };
}

export function removeSourcePrompt(name: string): ConfirmPrompt {
  return {
    title: msg('transfers.sourcesRemoveAction'),
    text: msg('transfers.sourcesConfirmRemove', { name }),
    confirm: msg('transfers.sourcesRemoveAction'),
  };
}

export function deleteThemePrompt(name: string): ConfirmPrompt {
  return {
    title: msg('settings.appearanceDeleteTitle'),
    text: msg('settings.appearanceDeleteConfirm', { name }),
    confirm: msg('common.delete'),
  };
}

export function forgetSyncPrompt(): ConfirmPrompt {
  return {
    title: msg('settings.generalSyncForgetLabel'),
    text: msg('settings.generalSyncForgetConfirm'),
    confirm: msg('settings.generalSyncForgetLabel'),
    busy: msg('settings.generalSyncForgetRunning'),
  };
}

export function resetAppearancePrompt(): ConfirmPrompt {
  return {
    title: msg('settings.appearanceResetLabel'),
    text: msg('settings.appearanceResetConfirm'),
    confirm: msg('settings.appearanceResetButton'),
  };
}

export function clearHistoryPrompt(): ConfirmPrompt {
  return {
    title: msg('transfers.historyClearAction'),
    text: msg('transfers.historyConfirmClearText'),
    confirm: msg('transfers.historyClearAction'),
    busy: msg('transfers.historyClearing'),
  };
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
