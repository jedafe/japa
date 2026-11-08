<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { resolveSection } from '#lib/components/hub/settings/settingsSections.js';
	import { t } from '#lib/i18n/index.js';
	let { data } = $props();
	const meta = $derived(resolveSection(data.section));
	const SectionComponent = $derived(meta?.component ?? null);
	const title = $derived(meta ? t(meta.labelKey) : '');
</script>

<SeoHead title={`${title} — ${t('settings.nav.label')}`} robots="`noindex, nofollow`" description={t('settings.section_description', '{name} — account settings on {site}.').replace('{name}', title)} />

{#if SectionComponent}
	<SectionComponent data={data.sectionData} />
{/if}
