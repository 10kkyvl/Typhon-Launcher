import { derived, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import {
  listInstallations,
  type Installation,
  type InstallStatus,
  type InstallType,
} from '../services/install';
import { msg } from '../i18n';
import { toast } from './toasts';

export const installations = writable<Installation[]>([]);

export const installationsByDownload = derived(installations, ($items) => {
  const map = new Map<string, Installation>();
  for (const item of $items) {
    const current = map.get(item.downloadId);
    if (!current || Date.parse(item.startedAt) >= Date.parse(current.startedAt)) {
      map.set(item.downloadId, item);
    }
  }
  return map;
});

export function installStatusLabels(status: InstallStatus): string {
  const labels: Record<InstallStatus, string> = {
    pending: msg('state.installStatusPending'),
    preparing: msg('state.installStatusPreparing'),
    installing: msg('state.installStatusInstalling'),
    extracting: msg('state.installStatusExtracting'),
    verifying: msg('state.installStatusVerifying'),
    waiting_for_user: msg('state.installStatusWaitingForUser'),
    completed: msg('state.installStatusCompleted'),
    failed: msg('common.error'),
    cancelled: msg('state.installStatusCancelled'),
    interrupted: msg('state.installStatusInterrupted'),
  };
  return labels[status];
}

export function installTypeLabels(type: InstallType): string {
  const labels: Record<InstallType, string> = {
    portable: msg('state.installTypePortable'),
    archive_zip: msg('state.installTypeArchiveZip'),
    archive_7z: msg('state.installTypeArchive7z'),
    archive_rar: msg('state.installTypeArchiveRar'),
    exe_installer: msg('state.installTypeExeInstaller'),
    msi_installer: msg('state.installTypeMsiInstaller'),
    unknown: msg('state.installTypeUnknown'),
  };
  return labels[type];
}

const activeStatuses: InstallStatus[] = ['pending', 'preparing', 'installing', 'extracting', 'verifying'];

export function installActive(status: InstallStatus) {
  return activeStatuses.includes(status);
}

export function upsertInstallation(item: Installation) {
  installations.update((list) => {
    const index = list.findIndex((i) => i.id === item.id);
    if (index < 0) return [...list, item];
    const next = [...list];
    next[index] = item;
    return next;
  });
}

export async function initInstalls() {
  installations.set(await listInstallations());
  if (!inWails) return;

  Events.On('install:started', (event) => {
    upsertInstallation(event.data as Installation);
  });
  Events.On('install:updated', (event) => {
    upsertInstallation(event.data as Installation);
  });
  Events.On('install:completed', (event) => {
    const item = event.data as Installation;
    upsertInstallation(item);
    toast(msg('state.installCompletedToast', { name: item.name }), 'success');
  });
  Events.On('install:failed', (event) => {
    const item = event.data as Installation;
    upsertInstallation(item);
    toast(msg('state.installFailedToast', { name: item.name, error: item.error }), 'danger');
  });
  Events.On('install:cancelled', (event) => {
    upsertInstallation(event.data as Installation);
  });
  Events.On('install:removed', (event) => {
    const { id } = event.data as { id: string };
    installations.update((list) => list.filter((i) => i.id !== id));
  });
}
