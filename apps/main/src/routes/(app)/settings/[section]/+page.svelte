<script lang="ts">
	import { resolveSection } from '$lib/components/hub/settings/settingsSections';
	let { data } = $props();
	const meta = $derived(resolveSection(data.section));
	const SectionComponent = $derived(meta?.component ?? null);
</script>

{#if SectionComponent}
	<SectionComponent data={data.sectionData} />
{/if}
