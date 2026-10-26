import { get } from 'svelte/store';
import { activeTheme, endTrial, keepTrial, resetAppearance, themeTrial } from '../stores/theme';
import { locale, msg } from '../i18n';
import type { Theme } from '../services/theme';

const guardAfterChangeMs = 8000;

function randomHostId(): string {
  const bytes = new Uint32Array(4);
  crypto.getRandomValues(bytes);
  return `typhon-theme-guard-${Array.from(bytes, (n) => n.toString(16)).join('')}`;
}

// The store starts empty and gets the saved theme at startup; applying that
// theme is not a change, so the guard only counts changes after it.
export function guardStep(
  seen: string | undefined,
  theme: Theme | null,
): { seen: string | undefined; action: 'show' | 'hide' | 'none' } {
  if (!theme) return { seen, action: 'none' };
  const risky = !theme.builtIn || Boolean(theme.css);
  const key = `${theme.id}:${theme.css ?? ''}`;
  if (!risky) return { seen: key, action: 'hide' };
  return { seen: key, action: seen !== undefined && key !== seen ? 'show' : 'none' };
}

export function mountThemeGuard(): () => void {
  const host = document.createElement('div');
  host.id = randomHostId();

  // Bottom centre, not a corner: a theme may put the sidebar on either side,
  // and a corner would then cover its account block.
  let pinning = false;
  function pin() {
    pinning = true;
    host.style.setProperty('display', 'block', 'important');
    host.style.setProperty('position', 'fixed', 'important');
    host.style.setProperty('left', '50%', 'important');
    host.style.setProperty('transform', 'translateX(-50%)', 'important');
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
    .trial {
      all: initial;
      pointer-events: auto;
      display: flex;
      align-items: center;
      gap: 10px;
      font-family: system-ui, sans-serif;
      font-size: 13px;
      color: #fff;
      background: #1d2430;
      border: 1px solid #3a4658;
      border-radius: 10px;
      padding: 8px 8px 8px 14px;
      box-shadow: 0 8px 28px rgba(0, 0, 0, 0.45);
    }
    .trial button.keep {
      background: #3a4658;
    }
    .trial button.keep:hover {
      background: #4a586c;
    }
  `;

  const trialBar = document.createElement('div');
  trialBar.className = 'trial';
  trialBar.style.display = 'none';
  const trialText = document.createElement('span');
  const keepButton = document.createElement('button');
  keepButton.type = 'button';
  keepButton.className = 'keep';
  keepButton.addEventListener('click', () => keepTrial());
  const revertButton = document.createElement('button');
  revertButton.type = 'button';
  revertButton.addEventListener('click', () => endTrial());
  trialBar.append(trialText, keepButton, revertButton);

  const button = document.createElement('button');
  button.type = 'button';
  button.textContent = msg('ui.resetAppearance');
  button.style.display = 'none';
  button.addEventListener('click', () => {
    resetAppearance();
  });

  shadow.appendChild(style);
  shadow.appendChild(trialBar);
  shadow.appendChild(button);
  document.body.appendChild(host);

  let tick: ReturnType<typeof setInterval> | undefined;
  function renderTrial() {
    const trial = get(themeTrial);
    if (!trial) {
      trialBar.style.display = 'none';
      if (tick) clearInterval(tick);
      tick = undefined;
      return;
    }
    trialBar.style.display = 'flex';
    button.style.display = 'none';
    const seconds = Math.max(0, Math.ceil((trial.endsAt - Date.now()) / 1000));
    trialText.textContent = trial.kept
      ? msg('settings.themeTryKeptActive', { name: trial.name })
      : msg('settings.themeTryActive', { name: trial.name, seconds });
    keepButton.style.display = trial.kept ? 'none' : 'block';
    keepButton.textContent = msg('settings.themeTryKeep');
    revertButton.textContent = msg('settings.themeTryRevert');
    if (!trial.kept && !tick) tick = setInterval(renderTrial, 250);
    if (trial.kept && tick) {
      clearInterval(tick);
      tick = undefined;
    }
  }
  const unsubscribeTrial = themeTrial.subscribe(renderTrial);

  // Escape belongs to an open dialog or menu first; it ends only a timed trial,
  // the one that exists to get the user out of CSS that hides the interface.
  function onKey(event: KeyboardEvent) {
    if (event.key !== 'Escape') return;
    const trial = get(themeTrial);
    if (!trial || trial.kept) return;
    if (document.querySelector('[role="dialog"], [role="menu"], [role="listbox"]')) return;
    endTrial();
  }
  window.addEventListener('keydown', onKey);

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
    const step = guardStep(seen, theme);
    seen = step.seen;
    if (step.action === 'show' && !get(themeTrial)) show(guardAfterChangeMs);
    else if (step.action === 'hide') button.style.display = 'none';
  });

  const unsubscribeLocale = locale.subscribe(() => {
    button.textContent = msg('ui.resetAppearance');
    button.title = msg('ui.resetAppearanceHint');
    renderTrial();
  });

  return () => {
    unsubscribe();
    unsubscribeLocale();
    unsubscribeTrial();
    if (tick) clearInterval(tick);
    window.removeEventListener('keydown', onKey);
    if (hideTimer) clearTimeout(hideTimer);
    observer.disconnect();
    host.remove();
  };
}
