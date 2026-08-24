const maxMaskedChars = 10;

export function maskEmail(email: string): string {
  const value = email.trim();
  if (!value) return '';

  const at = value.lastIndexOf('@');
  if (at <= 0 || at === value.length - 1) return stars(value.length);

  const local = value.slice(0, at);
  const domain = value.slice(at + 1);
  return `${local.slice(0, 1)}${stars(local.length - 1)}@${domain}`;
}

function stars(count: number): string {
  return '*'.repeat(Math.min(Math.max(count, 1), maxMaskedChars));
}
