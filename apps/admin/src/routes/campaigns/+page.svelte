<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	import CampaignManager from '#lib/components/campaigns/campaign-manager.svelte';
	import { t } from '#lib/stores/i18n.svelte.js';
</script>

<svelte:head>
	<title>{t('campaigns.title', 'Content Campaigns')} — {siteName()} Admin</title>
</svelte:head>

<CampaignManager />
