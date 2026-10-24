const HEX = /^#([\da-f]{3}|[\da-f]{6})$/i;
const FUNCTION = /^(rgb|hsl)a?\(\s*([^()]+?)\s*\)$/i;

function hex(r: number, g: number, b: number): string {
  return '#' + [r, g, b].map((v) => Math.round(Math.max(0, Math.min(255, v))).toString(16).padStart(2, '0')).join('');
}

function channel(part: string, scale: number): number | null {
  if (!/^[+-]?(?:\d+\.?\d*|\.\d+)(%|deg)?$/.test(part)) return null;
  const n = parseFloat(part);
  return part.endsWith('%') ? (n / 100) * scale : n;
}

function hslToHex(h: number, s: number, l: number): string {
  const a = s * Math.min(l, 1 - l);
  const at = (n: number) => {
    const k = (n + h / 30) % 12;
    return l - a * Math.max(-1, Math.min(k - 3, 9 - k, 1));
  };
  return hex(at(0) * 255, at(8) * 255, at(4) * 255);
}

function parseFunction(kind: string, body: string): string | null {
  const parts = body.split(/[\s,/]+/).filter(Boolean);
  if (parts.length < 3 || parts.length > 4) return null;
  if (parts[3] !== undefined) {
    const alpha = channel(parts[3], 1);
    if (alpha === null || alpha < 1) return null;
  }
  if (kind === 'rgb') {
    const [r, g, b] = parts.slice(0, 3).map((p) => channel(p, 255));
    return r == null || g == null || b == null ? null : hex(r, g, b);
  }
  const h = channel(parts[0], 360);
  const s = channel(parts[1], 1);
  const l = channel(parts[2], 1);
  if (h === null || s === null || l === null || !parts[1].endsWith('%') || !parts[2].endsWith('%')) return null;
  return hslToHex(((h % 360) + 360) % 360, Math.max(0, Math.min(1, s)), Math.max(0, Math.min(1, l)));
}

function throughCanvas(value: string): string | null {
  if (typeof document === 'undefined') return null;
  const ctx = document.createElement('canvas').getContext('2d');
  if (!ctx) return null;
  ctx.fillStyle = '#000000';
  ctx.fillStyle = value;
  const first = ctx.fillStyle;
  ctx.fillStyle = '#ffffff';
  ctx.fillStyle = value;
  const second = ctx.fillStyle;
  return first === second && HEX.test(first) ? first : null;
}

export function normalizeColor(value: string): string | null {
  const text = value.trim();
  if (text === '') return null;
  const short = HEX.exec(text);
  if (short) {
    const digits = short[1].length === 3 ? [...short[1]].map((c) => c + c).join('') : short[1];
    return '#' + digits.toLowerCase();
  }
  const fn = FUNCTION.exec(text);
  if (fn) {
    const parsed = parseFunction(fn[1].toLowerCase(), fn[2]);
    if (parsed) return parsed;
  }
  return throughCanvas(text);
}
