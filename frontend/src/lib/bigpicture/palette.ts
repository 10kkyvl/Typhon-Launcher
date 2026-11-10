export const bigPicturePalette: Readonly<Record<string, string>> = {
  '--bg': '#0e0f11',
  '--bg-sidebar': '#0b0c0e',
  '--surface': '#16181b',
  '--surface-2': '#1e2024',
  '--surface-3': '#25282c',
  '--surface-4': '#2c2f34',
  '--border': 'rgba(255, 255, 255, 0.07)',
  '--border-strong': 'rgba(255, 255, 255, 0.13)',
  '--hover': 'rgba(255, 255, 255, 0.045)',
  '--hover-strong': 'rgba(255, 255, 255, 0.08)',
  '--text': '#edeef0',
  '--text-2': '#b6b9bf',
  '--text-3': '#93979e',
};

export const bigPictureStyle = Object.entries(bigPicturePalette)
  .map(([name, value]) => `${name}: ${value}`)
  .join('; ');
