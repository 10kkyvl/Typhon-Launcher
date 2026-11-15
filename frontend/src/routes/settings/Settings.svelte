<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import './settings-layout.css';
  import { untrack } from 'svelte';
  import CloudConsentModal from '../../lib/components/CloudConsentModal.svelte';
  import CloudDisableModal from '../../lib/components/CloudDisableModal.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import LegalDocumentModal from '../../lib/components/LegalDocumentModal.svelte';
  import LibrarySetupModal from '../../lib/components/LibrarySetupModal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { msg } from '../../lib/i18n';
  import SentDataModal from '../../lib/components/SentDataModal.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import ReleaseNotesList from '../../lib/components/ReleaseNotesList.svelte';
  import SourcesNoticeModal from '../../lib/components/SourcesNoticeModal.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import { tabPanel } from '../../lib/components/tabs';
  import AboutTab from './AboutTab.svelte';
  import AppearanceTab from './AppearanceTab.svelte';
  import DownloadsTab from './DownloadsTab.svelte';
  import GeneralTab from './GeneralTab.svelte';
  import { createSettingsPage } from './settingsPage.svelte';
  import { releaseNotesHistory, selfUpdateStatus } from '../../lib/stores/selfupdate';
  import { settings } from '../../lib/stores/settings';

  let { tab: initialTab }: { tab?: string } = $props();

  function tabOptions() {
    return [
      { id: 'general', label: msg('settings.generalTab') },
      { id: 'downloads', label: msg('settings.downloadsTab') },
      { id: 'appearance', label: msg('settings.appearanceTab') },
      { id: 'about', label: msg('settings.aboutTab') },
    ];
  }

  const tabs = $derived(tabOptions());

  let tab = $state('general');

  $effect(() => {
    const next = initialTab;
    untrack(() => {
      if (next && tabs.some((t) => t.id === next)) tab = next;
    });
  });

  const page = createSettingsPage();

  const current = $derived($settings);

  // Ids of the row labels that name the fields next to them, shared by the tabs.
  const uid = $props.id();
</script>

<Page>
  <PageHeader title={msg('settings.pageTitle')} />

  <div class="tabs-wrap">
    <Tabs {tabs} idPrefix={uid} label={msg('settings.pageTitle')} divider={false} bind:value={tab} />
  </div>

  <div class="settings-page">
  {#key tab}
  <div class="tab-body" {...tabPanel(uid, tab)}>
  {#if tab === 'general'}
    <GeneralTab {page} {uid} />
  {:else if tab === 'downloads'}
    <DownloadsTab {page} {uid} />
  {:else if tab === 'appearance'}
    <AppearanceTab />
  {:else if tab === 'about'}
    <AboutTab {page} />
  {/if}
  </div>
  {/key}

  </div>

  <LibrarySetupModal
    bind:open={page.librarySetupOpen}
    title={current?.libraryPath ? msg('settings.generalLibrarySetupTitleChange') : msg('settings.generalLibrarySetupTitleNew')}
    note={current?.libraryPath
      ? msg('settings.generalLibrarySetupChangeNote')
      : ''}
  />

  <CloudConsentModal
    bind:open={page.cloudConsentOpen}
    busy={page.cloudBusy}
    onenable={page.enableCloudSaves}
    onpolicy={() => page.openLegalDoc({ id: 'privacy', title: msg('settings.generalPrivacyPolicyLinkLabel') })}
  />

  <CloudDisableModal
    bind:open={page.cloudDisableOpen}
    busy={page.cloudBusy}
    ondelete={() => page.disableCloudSaves(true)}
    onkeep={() => page.disableCloudSaves(false)}
  />

  <LegalDocumentModal bind:open={page.legalOpen} documentId={page.legalActiveId} title={page.legalActiveTitle} />

  <SourcesNoticeModal bind:open={page.sourcesNoticeReviewOpen} mode="review" />

  <SentDataModal bind:open={page.sentDataOpen} />

  {#if page.pending}
    <ConfirmModal prompt={page.pending.prompt} onconfirm={page.pending.run} onclose={page.closePending} />
  {/if}

  <Modal bind:open={page.historyOpen} title={msg('settings.aboutHistoryLabel')} width="104rem">
    <ReleaseNotesList notes={$releaseNotesHistory} currentVersion={$selfUpdateStatus.currentVersion} />
  </Modal>
</Page>

<style>
  /* Stuck over the scrolled settings, the tabs are an island like the top bar's. */
  .tabs-wrap {
    position: sticky;
    top: 0;
    z-index: var(--z-sticky);
    margin: var(--space-2) 0 var(--space-6);
    padding: 0 var(--space-4);
    background: var(--island-fill);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .tab-body {
    animation: rise-in var(--dur-panel) var(--ease) backwards;
  }
</style>
