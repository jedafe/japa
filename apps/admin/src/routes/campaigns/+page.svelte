<script lang="ts">
	import CampaignManager from '$lib/components/campaigns/campaign-manager.svelte';
	import { t } from '$lib/stores/i18n.svelte';
</script>

<svelte:head>
	<title>{t('campaigns.title', 'Content Campaigns')} — JapaConnect Admin</title>
</svelte:head>

<CampaignManager />
