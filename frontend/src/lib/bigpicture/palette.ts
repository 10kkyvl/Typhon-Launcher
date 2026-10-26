export const bigPicturePalette: Readonly<Record<string, string>> = {
  '--bg': '#090d17',
  '--bg-sidebar': '#090d17',
  '--surface': '#0f1522',
  '--surface-2': '#131a29',
  '--surface-3': '#1a2233',
  '--surface-4': '#222b3f',
  '--border': 'rgba(255, 255, 255, 0.07)',
  '--border-strong': 'rgba(255, 255, 255, 0.12)',
  '--hover': 'rgba(255, 255, 255, 0.045)',
  '--hover-strong': 'rgba(255, 255, 255, 0.075)',
  '--text': '#eef2f6',
  '--text-2': '#9aa5b8',
  '--text-3': '#647089',
};

export const bigPictureStyle = Object.entries(bigPicturePalette)
  .map(([name, value]) => `${name}: ${value}`)
  .join('; ');
