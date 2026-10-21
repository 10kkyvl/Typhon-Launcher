function windowsStyle(path: string): boolean {
  return /^[a-zA-Z]:/.test(path) || path.startsWith('\\\\');
}

export function pathKey(path: string): string {
  const slashed = path.trim().replace(/\\/g, '/');
  const unrooted = slashed.length > 1 ? slashed.replace(/\/+$/, '') : slashed;
  return windowsStyle(path.trim()) ? unrooted.toLowerCase() : unrooted;
}

export function insidePath(dir: string, path: string): boolean {
  const root = pathKey(dir);
  const target = pathKey(path);
  if (root === '' || target === '') return false;
  if (root === target) return true;
  return target.startsWith(root.endsWith('/') ? root : `${root}/`);
}

export function volumeKey(path: string): string {
  const key = pathKey(path);
  const drive = /^([a-z]):/.exec(key);
  if (drive) return `${drive[1]}:`;
  if (key.startsWith('//')) {
    const [server = '', share = ''] = key.slice(2).split('/');
    return `//${server}/${share}`;
  }
  const mount = /^\/volumes\/[^/]+/i.exec(key);
  if (mount) return mount[0].toLowerCase();
  return key.startsWith('/') ? '/' : '';
}

export function sameVolume(a: string, b: string): boolean {
  const first = volumeKey(a);
  return first !== '' && first === volumeKey(b);
}

export function gamesInFolder(folder: string, games: { installDir: string }[]): number {
  if (folder.trim() === '') return 0;
  return games.filter((game) => game.installDir !== '' && insidePath(folder, game.installDir)).length;
}

export function gamesOnVolume<T extends { installDir: string }>(volumePath: string, games: T[]): T[] {
  return games.filter((game) => sameVolume(volumePath, game.installDir));
}

export function rootOfDestination(roots: string[], destination: string): string {
  let best = '';
  for (const root of roots) {
    if (pathKey(root) === pathKey(destination) || !insidePath(root, destination)) continue;
    if (pathKey(root).length > pathKey(best).length) best = root;
  }
  return best;
}

function folderName(path: string): string {
  const parts = path.trim().replace(/[\\/]+$/, '').split(/[\\/]/);
  const last = parts[parts.length - 1] ?? '';
  return /^[a-z]:$/i.test(last) ? '' : last;
}

export function destinationInRoot(root: string, destination: string): string {
  const name = folderName(destination);
  const base = root.trim().replace(/[\\/]+$/, '');
  if (name === '') return base === '' ? root : base;
  const separator = windowsStyle(root.trim()) ? '\\' : '/';
  return `${base}${separator}${name}`;
}
