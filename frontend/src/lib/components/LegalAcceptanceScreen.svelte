<script lang="ts">
  import { TriangleAlert } from '@lucide/svelte';
  import { settings } from '../stores/settings';
  import {
    currentLegalVersion,
    legalVersionFailed,
    loadLegalVersion,
    respondLegalAcceptance,
  } from '../stores/legalAcceptance';
  import { errorCode, locale, msg } from '../i18n';
  import type { MessageKey } from '../i18n';
  import Button from './Button.svelte';
  import LegalDocumentModal from './LegalDocumentModal.svelte';

  const REASONS: Record<string, MessageKey> = {
    'settings.legal_acceptance_save_failed': 'modals.legalAcceptSaveFailed',
  };

  const points: MessageKey[] = [
    'modals.legalAcceptPoint1',
    'modals.legalAcceptPoint2',
    'modals.legalAcceptPoint3',
    'modals.legalAcceptPoint4',
  ];

  let saving = $state(false);
  let error = $state('');
  let docOpen = $state(false);
  let docId = $state<string | null>(null);
  let docTitle = $state('');

  const updated = $derived.by(() => {
    const accepted = $settings?.legalAcceptedVersion ?? '';
    const version = $currentLegalVersion;
    if (accepted === '' || version === null || accepted === version) return '';
    const date = new Date(`${version}T00:00:00`);
    if (Number.isNaN(date.getTime())) return version;
    return date.toLocaleDateString($locale, { year: 'numeric', month: 'long', day: 'numeric' });
  });

  function openDoc(id: string, title: string) {
    docId = id;
    docTitle = title;
    docOpen = true;
  }

  async function accept() {
    saving = true;
    error = '';
    try {
      await respondLegalAcceptance();
    } catch (err) {
      const key = REASONS[errorCode(err)];
      error = msg(key ?? 'modals.legalAcceptSaveFallback');
    } finally {
      saving = false;
    }
  }
</script>

<!--
  Deliberately not a Modal: Modal closes on Escape, on a backdrop click and on
  its own X, and this screen stays up until the acceptance is stored. There is
  no decline button; the only way out besides accepting is closing the app.
-->
<div class="screen" role="dialog" aria-modal="true" aria-labelledby="legal-title">
  <div class="card">
    <div class="head">
      <h3 id="legal-title">{msg('modals.legalAcceptTitle')}</h3>
      {#if updated}
        <p class="updated">{msg('modals.legalAcceptUpdated', { date: updated })}</p>
      {/if}
    </div>

    <div class="body">
      {#if $legalVersionFailed}
        <p class="error">
          <TriangleAlert size="1.5rem" strokeWidth={1.8} />
          {msg('modals.legalAcceptVersionFailed')}
        </p>
      {:else}
        <p class="text">{msg('modals.legalAcceptIntro')}</p>
        <ol class="points">
          {#each points as key (key)}
            <li>{msg(key)}</li>
          {/each}
        </ol>
        <div class="links">
          <Button variant="ghost" onclick={() => openDoc('terms', msg('modals.legalAcceptTermsLink'))}>
            {msg('modals.legalAcceptTermsLink')}
          </Button>
          <Button variant="ghost" onclick={() => openDoc('privacy', msg('modals.legalAcceptPrivacyLink'))}>
            {msg('modals.legalAcceptPrivacyLink')}
          </Button>
        </div>
      {/if}

      {#if error}
        <p class="error">
          <TriangleAlert size="1.5rem" strokeWidth={1.8} />
          {error}
        </p>
      {/if}
    </div>

    <div class="foot">
      {#if $legalVersionFailed}
        <Button size="lg" variant="primary" onclick={loadLegalVersion}>{msg('modals.legalAcceptRetry')}</Button>
      {:else}
        <Button size="lg" variant="primary" disabled={saving || $currentLegalVersion === null} onclick={accept}>
          {saving ? msg('modals.legalAcceptSaving') : msg('modals.legalAcceptButton')}
        </Button>
      {/if}
    </div>
  </div>
</div>

<LegalDocumentModal bind:open={docOpen} documentId={docId} title={docTitle} />

<style>
  .screen {
    position: fixed;
    inset: 0;
    z-index: 100;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(4, 6, 10, 0.62);
    animation: fade var(--dur-panel) var(--ease);
  }

  .card {
    width: 52rem;
    max-width: calc(100vw - 4.8rem);
    max-height: calc(100vh - 8rem);
    display: flex;
    flex-direction: column;
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-xl);
    box-shadow: var(--shadow-modal);
    animation: rise var(--dur-panel) var(--ease);
  }

  .head {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    padding: 1.8rem 2.4rem 0;
  }

  h3 {
    font-size: var(--font-lg);
    font-weight: 600;
  }

  .updated {
    font-size: var(--font-sm);
    color: var(--accent);
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    padding: 1.6rem 2.4rem 2.4rem;
    overflow-y: auto;
  }

  .text {
    font-size: var(--font-md);
    line-height: 1.6;
    color: var(--text-2);
  }

  .points {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding-left: 2rem;
    font-size: var(--font-md);
    line-height: 1.6;
    color: var(--text-2);
  }

  .links {
    display: flex;
    flex-wrap: wrap;
    gap: 0.8rem;
  }

  .foot {
    display: flex;
    justify-content: flex-end;
    gap: 0.8rem;
    padding: 1.4rem 2.4rem;
    border-top: 1px solid var(--border);
  }

  .error {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    font-size: var(--font-sm);
    color: var(--danger);
  }

  @keyframes fade {
    from {
      opacity: 0;
    }
    to {
      opacity: 1;
    }
  }

  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(0.4rem);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
