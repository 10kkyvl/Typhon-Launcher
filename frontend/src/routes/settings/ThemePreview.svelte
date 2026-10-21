<script lang="ts">
  import { msg } from '../../lib/i18n';

  let { vars }: { vars: string } = $props();
</script>

<div class="preview" style={vars} role="img" aria-label={msg('settings.themePreviewAria')}>
  <div class="side">
    <span class="side-item active">{msg('settings.themePreviewSidebarLibrary')}</span>
    <span class="side-item">{msg('settings.themePreviewSidebarCatalog')}</span>
  </div>
  <div class="main">
    <div class="card">
      <div class="cover"></div>
      <div class="info">
        <span class="title">{msg('settings.themePreviewGameTitle')}</span>
        <span class="sub">{msg('settings.themePreviewGameSub')}</span>
        <span class="muted">{msg('settings.themePreviewGameMuted')}</span>
        <div class="buttons">
          <span class="btn primary">{msg('settings.themePreviewPlay')}</span>
          <span class="btn secondary">{msg('settings.themePreviewSecondary')}</span>
        </div>
      </div>
    </div>
    <div class="field">{msg('settings.themePreviewInput')}</div>
    <div class="badges">
      <span class="badge success">{msg('settings.themePreviewSuccess')}</span>
      <span class="badge warning">{msg('settings.themePreviewWarning')}</span>
      <span class="badge danger">{msg('settings.themePreviewDanger')}</span>
    </div>
  </div>
</div>

<style>
  .preview {
    display: flex;
    min-height: 24rem;
    overflow: hidden;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    color: var(--text);
    font-family: var(--font-sans);
    font-size: var(--font-sm);
    user-select: none;
    pointer-events: none;
  }

  .side {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    flex: 0 0 9rem;
    padding: var(--space-3) var(--space-2);
    background: var(--bg-sidebar);
    border-right: 1px solid var(--border);
  }

  .side-item {
    padding: var(--space-1) var(--space-2);
    border-radius: var(--radius-sm);
    color: var(--text-2);
    font-size: var(--font-xs);
  }

  .side-item.active {
    background: var(--accent-subtle);
    color: var(--accent-text);
  }

  .main {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    flex: 1 1 auto;
    min-width: 0;
    padding: var(--space-3);
  }

  .card {
    display: flex;
    gap: var(--space-3);
    padding: var(--space-3);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .cover {
    flex: 0 0 5rem;
    min-height: 7rem;
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
    margin-top: var(--space-1);
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
</style>
