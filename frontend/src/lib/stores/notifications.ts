import { derived } from 'svelte/store';
import { active } from './downloads';
import { installActive, installations } from './install';
import type { RouteName } from './router';
import { sources } from './sources';
import { updates } from './updates';

function pct(value: number) {
  return `${Math.round(value * 100)}%`;
}

export interface Notification {
  id: string;
  title: string;
  text: string;
  route: RouteName;
}

export const notifications = derived(
  [active, installations, updates, sources],
  ([$active, $installations, $updates, $sources]): Notification[] => {
    const items: Notification[] = [];

    for (const source of $sources) {
      if (!source.lastError) continue;
      items.push({
        id: `source:${source.id}`,
        title: source.name,
        text: source.lastError,
        route: 'sources',
      });
    }

    for (const update of $updates) {
      if (update.error) {
        items.push({
          id: `update-error:${update.gameId}`,
          title: update.title,
          text: `Обновление не удалось: ${update.error}`,
          route: 'installed',
        });
        continue;
      }
      if (update.state === 'update_downloading' || update.state === 'updating') {
        items.push({
          id: `update:${update.gameId}`,
          title: update.title,
          text: `Обновление идёт — ${pct(update.progress)}`,
          route: 'installed',
        });
        continue;
      }
      if (update.availability.available) {
        items.push({
          id: `update-available:${update.gameId}`,
          title: update.title,
          text: 'Доступно обновление',
          route: 'installed',
        });
      }
    }

    for (const install of $installations) {
      if (!installActive(install.status)) continue;
      items.push({
        id: `install:${install.id}`,
        title: install.name,
        text: `Установка — ${pct(install.progress)}`,
        route: 'downloads',
      });
    }

    for (const download of $active) {
      items.push({
        id: `download:${download.id}`,
        title: download.name,
        text: `Загрузка — ${pct(download.progress)}`,
        route: 'downloads',
      });
    }

    return items;
  },
);
