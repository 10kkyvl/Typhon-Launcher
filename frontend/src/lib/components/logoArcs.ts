/**
 * The Typhon mark as five vector arcs in a 256 by 256 view box, outermost blade first: the pieces the
 * update show flies (UpdateShow.svelte). `box` is each arc's bounding box (x, y, width, height), the
 * point it turns and grows around. The arcs follow the shapes of the picture (public/typhon.png) but
 * are not a copy of it: the resting logo stays the picture, and the arcs are drawn only while the
 * show runs.
 */
export type LogoArc = { id: number; d: string; opacity: number; box: readonly [number, number, number, number] };

export const LOGO_VIEWBOX = 256;

export const LOGO_ARCS: readonly LogoArc[] = [
  {
    id: 0,
    d: 'M25,37.5 C37.5,58.8 82.5,58.8 130,46.3 C162.5,37.5 190,28.8 212.5,41.3 C230,51.3 238.8,75 233.8,105 C231.3,117.5 226.3,125 221.3,130 C228.8,105 220,75 195,62.5 C162.5,57.5 112.5,72.5 75,72.5 C45,72.5 26.3,57.5 25,37.5 Z',
    opacity: 0.42,
    box: [25, 35.1, 210.2, 94.9],
  },
  {
    id: 1,
    d: 'M25,60 C33.8,77.5 67.5,80 100,68.8 C130,59.5 155,53 180,54.5 C207.5,56.3 230,72.5 235,96.3 C236.3,112.5 230,122.5 221.3,130 C226.3,110 217.5,87.5 197.5,80.5 C175,72 145,73.8 135,91.3 C130,103.8 137.5,120 153.8,126.3 C135,123.8 120,117.5 107.5,105 C102.5,98.8 102.5,93.8 105,91.3 C82.5,96.3 47.5,92.5 30,71.3 C27,67.5 25.5,63.8 25,60 Z',
    opacity: 0.9,
    box: [25, 54.3, 210.2, 75.7],
  },
  {
    id: 2,
    d: 'M33.8,100 C60,108.8 70,116.3 58,128.8 C58.8,147.5 80,162.5 101.3,170.5 C75,173.8 47.5,161.3 40,141.3 C35,126.3 35,110 33.8,100 Z',
    opacity: 0.3,
    box: [33.8, 100, 67.5, 71],
  },
  {
    id: 3,
    d: 'M106.3,96.3 C85,110 83.8,142.5 112.5,153 C135,160.5 160,148.8 176.3,135 C156.3,146.3 136.3,148.8 121.3,138.8 C105,128 98.8,112.5 106.3,96.3 Z',
    opacity: 0.7,
    box: [90.6, 96.3, 85.7, 59],
  },
  {
    id: 4,
    d: 'M149.5,106.3 C160,93.8 186.3,92 196.5,110 C201.3,126.3 190,137 162.5,145 C135,157.5 111.3,172.5 107.5,195 C106.3,212.5 117.5,227.5 131.3,237.5 C122.5,226.3 124.5,212.5 125.8,197.5 C128,178 142.5,165.5 163.8,154.5 C183.8,143.8 177.5,122.5 165.5,110.5 C160,106.8 155,105.5 149.5,106.3 Z',
    opacity: 0.95,
    box: [107.4, 96.7, 90.2, 140.8],
  },
];
