<script lang="ts">
  import { Palette } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Page from '../../lib/components/Page.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { SHOWCASE_LABEL } from '../../lib/navigation/showcase';
  import { navigate } from '../../lib/stores/router';
  import ShowcaseActions from './ShowcaseActions.svelte';
  import ShowcaseFields from './ShowcaseFields.svelte';
  import ShowcaseGames from './ShowcaseGames.svelte';
  import ShowcaseLayers from './ShowcaseLayers.svelte';
  import ShowcaseNotices from './ShowcaseNotices.svelte';
  import ShowcaseStatus from './ShowcaseStatus.svelte';

  /**
   * Every base component of lib/components in its states, in the current theme: for developers, theme
   * authors and later plugin authors. Development builds only (lib/navigation/showcase.ts); the words
   * are the page's own and are not translated. It uses each component as its doc comment says, so it
   * is a usage reference too. A new base component is shown here (showcase.coverage.test.ts).
   */
</script>

<Page>
  <div class="showcase" id="showcase-top">
    <PageHeader title={SHOWCASE_LABEL} subtitle="The base components in their states, in the current theme. Development builds only.">
      {#snippet actions()}
        <Button onclick={() => navigate('settings', { tab: 'appearance' })}><Palette size="1.6rem" />Change the theme</Button>
      {/snippet}
    </PageHeader>
    <ShowcaseActions />
    <ShowcaseFields />
    <ShowcaseStatus />
    <ShowcaseNotices />
    <ShowcaseLayers />
    <ShowcaseGames />
  </div>
</Page>

<style>
  .showcase {
    display: flex;
    flex-direction: column;
    gap: var(--space-6);
    padding-bottom: var(--space-12);
  }
</style>
