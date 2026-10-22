<script lang="ts">
  import { msg } from '../../lib/i18n';

  let { vars }: { vars: string } = $props();
</script>

<div class="preview" style={vars} role="img" aria-label={msg('settings.themePreviewAria')}>
  <div class="topbar">
    <span class="brand">Typhon</span>
    <div class="field topbar-field">{msg('settings.themePreviewInput')}</div>
    <span class="link">{msg('settings.themePreviewLink')}</span>
  </div>

  <div class="body">
    <div class="side">
      <span class="side-item active">{msg('settings.themePreviewSidebarLibrary')}</span>
      <span class="side-item">{msg('settings.themePreviewSidebarCatalog')}</span>
      <span class="side-item hovered">{msg('settings.themePreviewHover')}</span>
      <span class="side-item pressed">{msg('settings.themePreviewPressed')}</span>
    </div>

    <div class="main">
      <h2 class="page-title">{msg('settings.themePreviewPageTitle')}</h2>

      <div class="grid">
        <div class="card">
          <h3 class="card-heading">{msg('settings.themePreviewCardHeading')}</h3>
          <div class="game">
            <div class="cover"></div>
            <div class="info">
              <span class="title">{msg('settings.themePreviewGameTitle')}</span>
              <span class="sub">{msg('settings.themePreviewGameSub')}</span>
              <span class="muted">{msg('settings.themePreviewGameMuted')}</span>
              <div class="buttons">
                <span class="btn primary">{msg('settings.themePreviewPlay')}</span>
                <span class="btn primary hover">{msg('settings.themePreviewHover')}</span>
                <span class="btn secondary">{msg('settings.themePreviewSecondary')}</span>
              </div>
            </div>
          </div>
          <div class="badges">
            <span class="badge success">{msg('settings.themePreviewSuccess')}</span>
            <span class="badge warning">{msg('settings.themePreviewWarning')}</span>
            <span class="badge danger">{msg('settings.themePreviewDanger')}</span>
          </div>
        </div>

        <div class="card">
          <h3 class="card-heading">{msg('settings.themePreviewDownload')}</h3>
          <div class="progress"><div class="progress-fill"></div></div>
          <span class="muted">{msg('settings.themePreviewGameMuted')}</span>
          <div class="field">{msg('settings.themePreviewInput')}</div>
          <div class="field focused">{msg('settings.themePreviewFocus')}</div>
          <div class="checks">
            <span class="check">
              <span class="toggle"><span class="knob"></span></span>
              {msg('settings.themePreviewToggle')}
            </span>
            <span class="check">
              <span class="box"></span>
              {msg('settings.themePreviewCheckbox')}
            </span>
          </div>
          <span class="muted">{msg('settings.themePreviewControls')}</span>
          <div class="controls">
            <span class="ctl sm">{msg('settings.themePreviewControlSm')}</span>
            <span class="ctl md">{msg('settings.themePreviewControlMd')}</span>
            <span class="ctl lg">{msg('settings.themePreviewControlLg')}</span>
          </div>
        </div>

        <div class="card layer">
          <div class="toast">
            <span class="toast-title">{msg('settings.themePreviewToast')}</span>
            <span class="sub">{msg('settings.themePreviewToastText')}</span>
          </div>
          <div class="dialog">
            <h3 class="dialog-heading">{msg('settings.themePreviewDialog')}</h3>
            <span class="sub">{msg('settings.themePreviewDialogText')}</span>
            <div class="buttons">
              <span class="btn secondary">{msg('settings.themePreviewCancel')}</span>
              <span class="btn primary">{msg('settings.themePreviewConfirm')}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</div>

<style>
  .preview {
    display: flex;
    flex-direction: column;
    min-height: 32rem;
    overflow: hidden;
    container-type: inline-size;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    color: var(--text);
    font-family: var(--font-sans);
    font-size: var(--font-sm);
    user-select: none;
    pointer-events: none;
  }

  .preview span,
  .preview h2,
  .preview h3,
  .preview .field {
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .preview h2,
  .preview h3 {
    margin: 0;
  }

  .topbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-4);
    min-height: var(--control-md);
    padding: var(--space-2) var(--space-4);
    background: var(--surface);
    border-bottom: 1px solid var(--border);
  }

  .brand {
    font-size: var(--font-md);
    font-weight: 700;
  }

  .topbar-field {
    flex: 1 1 16rem;
    max-width: 32rem;
  }

  .link {
    color: var(--accent-text);
    font-size: var(--font-sm);
  }

  .body {
    display: flex;
    flex: 1 1 auto;
    min-width: 0;
  }

  .side {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    flex: 0 0 16rem;
    min-width: 0;
    padding: var(--space-4) var(--space-3);
    background: var(--bg-sidebar);
    border-right: 1px solid var(--border);
  }

  .side-item {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-sm);
    color: var(--text-2);
    font-size: var(--font-sm);
  }

  .side-item.active {
    background: var(--accent-subtle);
    color: var(--accent-text);
  }

  .side-item.hovered {
    background: var(--hover);
    color: var(--text);
  }

  .side-item.pressed {
    background: var(--hover-strong);
    color: var(--text);
  }

  .main {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    flex: 1 1 auto;
    min-width: 0;
    padding: var(--space-6);
  }

  .page-title {
    font-size: var(--font-title);
    font-weight: 700;
    letter-spacing: var(--tracking-title);
    line-height: 1.1;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(26rem, 1fr));
    gap: var(--space-5);
    align-items: start;
  }

  .card {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    min-width: 0;
    padding: var(--space-4);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .card-heading {
    font-size: var(--font-lg);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
  }

  .game {
    display: flex;
    gap: var(--space-4);
  }

  .cover {
    flex: 0 0 8rem;
    min-height: 11rem;
    background: var(--surface-3);
    border-radius: var(--radius-md);
  }

  .info {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .title {
    color: var(--text);
    font-size: var(--font-md);
    font-weight: 600;
  }

  .sub {
    color: var(--text-2);
    font-size: var(--font-xs);
  }

  .muted {
    color: var(--text-3);
    font-size: var(--font-xs);
  }

  .buttons {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }

  .btn {
    padding: var(--space-1) var(--space-3);
    border-radius: var(--radius-md);
    font-size: var(--font-xs);
    font-weight: 500;
  }

  .btn.primary {
    background: var(--accent);
    color: var(--accent-on, #fff);
  }

  .btn.primary.hover {
    background: var(--accent-hover);
  }

  .btn.secondary {
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    color: var(--text);
  }

  .field {
    padding: var(--space-2) var(--space-3);
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    color: var(--text-3);
    font-size: var(--font-xs);
  }

  .field.focused {
    border-color: var(--accent);
    box-shadow: 0 0 0 var(--space-1) var(--accent-ring);
    color: var(--text);
  }

  .badges {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .badge {
    padding: var(--space-1) var(--space-2);
    border-radius: var(--radius-xs);
    font-size: var(--font-xs);
    font-weight: 500;
  }

  .badge.success {
    background: var(--success-subtle);
    color: var(--success);
  }

  .badge.warning {
    background: var(--warning-subtle);
    color: var(--warning);
  }

  .badge.danger {
    background: var(--danger-subtle);
    color: var(--danger);
  }

  .progress {
    height: var(--space-2);
    background: var(--surface-3);
    border-radius: var(--radius-xs);
    overflow: hidden;
  }

  .progress-fill {
    width: 64%;
    height: 100%;
    background: var(--accent);
  }

  .checks {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-4);
  }

  .check {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-2);
    font-size: var(--font-xs);
  }

  .toggle {
    display: inline-flex;
    align-items: center;
    justify-content: flex-end;
    flex: 0 0 auto;
    width: calc(var(--space-5) * 1.8);
    height: var(--space-5);
    padding: var(--space-1);
    background: var(--accent);
    border-radius: var(--space-5);
  }

  .knob {
    width: calc(var(--space-5) - var(--space-2));
    height: calc(var(--space-5) - var(--space-2));
    background: var(--accent-on, #fff);
    border-radius: 50%;
  }

  .box {
    flex: 0 0 auto;
    width: var(--space-4);
    height: var(--space-4);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-xs);
  }

  .controls {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .ctl {
    display: inline-flex;
    align-items: center;
    padding: 0 var(--space-3);
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    font-size: var(--font-xs);
  }

  .ctl.sm {
    height: var(--control-sm);
  }

  .ctl.md {
    height: var(--control-md);
  }

  .ctl.lg {
    height: var(--control-lg);
  }

  .layer {
    grid-column: 1 / -1;
    flex-wrap: wrap;
    gap: var(--space-5);
  }

  .layer > * {
    flex: 1 1 26rem;
  }

  .toast {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding: var(--space-3) var(--space-4);
    background: var(--surface-4);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
  }

  .toast-title {
    font-size: var(--font-sm);
    font-weight: 600;
  }

  .dialog {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-5);
    background: var(--surface);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-xl);
    box-shadow: var(--shadow-modal);
  }

  .dialog-heading {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
  }

  @container (max-width: 48rem) {
    .body {
      flex-direction: column;
    }

    .side {
      flex: 0 0 auto;
      flex-direction: row;
      flex-wrap: wrap;
      border-right: 0;
      border-bottom: 1px solid var(--border);
    }
  }
</style>
