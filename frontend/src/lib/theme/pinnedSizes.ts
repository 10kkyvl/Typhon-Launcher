// Big Picture and the in-game overlay are drawn for the size scale the main window had before it
// took the redesign's density: they set these values on their own root, whatever the theme says.
export const pinnedSizes: Readonly<Record<string, string>> = {
  '--radius-xs': '0.44rem',
  '--space-1': '0.44rem',
  '--space-2': '0.88rem',
  '--space-3': '1.32rem',
  '--space-4': '1.76rem',
  '--space-5': '2.2rem',
  '--space-6': '2.64rem',
  '--space-8': '3.52rem',
  '--space-10': '4.4rem',
  '--space-12': '5.28rem',
  '--page-x': '3.96rem',
  '--page-max': '255.2rem',
  '--prose-max': '83.6rem',
  '--sidebar-w': '25.52rem',
  '--topbar-h': '6.16rem',
  '--font-xs': '1.43rem',
  '--font-sm': '1.54rem',
  '--font-md': '1.65rem',
  '--font-lg': '1.87rem',
  '--font-xl': '2.09rem',
  '--font-title': '3.52rem',
  '--font-hero': 'clamp(3.96rem, 2.6vw, 4.84rem)',
  '--tracking-title': '-0.024em',
  '--tracking-heading': '-0.012em',
  '--control-sm': '3.52rem',
  '--control-md': '4.18rem',
  '--control-lg': '4.84rem',
};

export const pinnedSizesStyle = Object.entries(pinnedSizes)
  .map(([name, value]) => `${name}: ${value}`)
  .join('; ');
