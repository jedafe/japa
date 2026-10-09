<script lang="ts">
	import { resolveSection } from '$lib/components/hub/settings/settingsSections';
	import { t } from '$lib/i18n';
	let { data } = $props();
	const meta = $derived(resolveSection(data.section));
	const SectionComponent = $derived(meta?.component ?? null);
	const title = $derived(meta ? t(meta.labelKey) : '');
</script>

<svelte:head>
	<title>{title} — {t('settings.nav.label')}</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

{#if SectionComponent}
	<SectionComponent data={data.sectionData} />
{/if}
