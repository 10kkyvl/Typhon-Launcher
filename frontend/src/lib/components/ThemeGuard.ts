import { activeTheme, resetAppearance } from '../stores/theme';
import { locale, msg } from '../i18n';

const guardAfterChangeMs = 8000;
const guardAfterHotkeyMs = 15000;

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
  button.textContent = msg('ui.resetAppearance');
  button.style.display = 'none';
  button.addEventListener('click', () => {
    resetAppearance();
  });

  shadow.appendChild(style);
  shadow.appendChild(button);
  document.body.appendChild(host);

  let hideTimer: ReturnType<typeof setTimeout> | undefined;
  function show(ms: number) {
    button.style.display = 'block';
    if (hideTimer) clearTimeout(hideTimer);
    hideTimer = setTimeout(() => {
      button.style.display = 'none';
    }, ms);
  }

  let seen: string | undefined;
  const unsubscribe = activeTheme.subscribe((theme) => {
    const risky = Boolean(theme) && (!theme!.builtIn || Boolean(theme!.css));
    const key = theme ? `${theme.id}:${theme.css ?? ''}` : '';
    const changed = seen !== undefined && key !== seen;
    seen = key;
    if (risky && changed) show(guardAfterChangeMs);
    else if (!risky) button.style.display = 'none';
  });

  const onKey = (event: KeyboardEvent) => {
    if (event.ctrlKey && event.altKey && !event.shiftKey && event.code === 'KeyR') {
      event.preventDefault();
      show(guardAfterHotkeyMs);
    }
  };
  window.addEventListener('keydown', onKey);

  const unsubscribeLocale = locale.subscribe(() => {
    button.textContent = msg('ui.resetAppearance');
    button.title = msg('ui.resetAppearanceHint');
  });

  return () => {
    unsubscribe();
    unsubscribeLocale();
    window.removeEventListener('keydown', onKey);
    if (hideTimer) clearTimeout(hideTimer);
    observer.disconnect();
    host.remove();
  };
}
