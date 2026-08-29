import { activeTheme, resetAppearance } from '../stores/theme';

function randomHostId(): string {
  const bytes = new Uint32Array(4);
  crypto.getRandomValues(bytes);
  return `typhon-theme-guard-${Array.from(bytes, (n) => n.toString(16)).join('')}`;
}

export function mountThemeGuard(): () => void {
  const host = document.createElement('div');
  host.id = randomHostId();

  let pinning = false;
  function pin() {
    pinning = true;
    host.style.setProperty('display', 'block', 'important');
    host.style.setProperty('position', 'fixed', 'important');
    host.style.setProperty('right', '1.6rem', 'important');
    host.style.setProperty('bottom', '1.6rem', 'important');
    host.style.setProperty('z-index', '2147483647', 'important');
    host.style.setProperty('pointer-events', 'none', 'important');
    queueMicrotask(() => {
      pinning = false;
    });
  }
  pin();

  const observer = new MutationObserver(() => {
    if (!pinning) pin();
  });
  observer.observe(host, { attributes: true, attributeFilter: ['style'] });

  const shadow = host.attachShadow({ mode: 'open' });
  const style = document.createElement('style');
  style.textContent = `
    button {
      all: initial;
      pointer-events: auto;
      display: block;
      font-family: system-ui, sans-serif;
      font-size: 13px;
      color: #fff;
      background: #d96969;
      border-radius: 8px;
      padding: 8px 14px;
      cursor: pointer;
      box-shadow: 0 8px 28px rgba(0, 0, 0, 0.45);
    }
    button:hover {
      background: #e07d7d;
    }
  `;

  const button = document.createElement('button');
  button.type = 'button';
  button.textContent = 'Сбросить оформление';
  button.style.display = 'none';
  button.addEventListener('click', () => {
    resetAppearance();
  });

  shadow.appendChild(style);
  shadow.appendChild(button);
  document.body.appendChild(host);

  const unsubscribe = activeTheme.subscribe((theme) => {
    const visible = Boolean(theme) && (!theme!.builtIn || Boolean(theme!.css));
    button.style.display = visible ? 'block' : 'none';
  });

  return () => {
    unsubscribe();
    observer.disconnect();
    host.remove();
  };
}
