import { setSavesDir } from '../../services/library';
import { selectFolder } from '../../services/settings';
import { msg } from '../../i18n';

export async function chooseSavesDir(gameId: string, title: string): Promise<string> {
  const dir = await selectFolder(msg('ui.savesDirDialogTitle', { title }));
  if (!dir) return '';
  await setSavesDir(gameId, dir);
  return dir;
}
