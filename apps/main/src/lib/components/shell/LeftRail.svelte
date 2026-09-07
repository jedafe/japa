<script lang="ts">
	// LeftRail — a dedicated 160×600 ad column for the left sidebar position
	// (Section 59, ADR-066; Section 25 ad placement). It renders only
	// admin-managed `ad_slot` widgets and is mounted by AppShell solely while
	// the side nav is collapsed. Other left-position widget types are ignored
	// — the rail is intentionally ad-only.
	import AdSlot from '$lib/components/ui/AdSlot.svelte';
	import { t } from '$lib/i18n';

	interface Props {
		widgets?: unknown[];
	}

	let { widgets = [] }: Props = $props();

	interface AdWidget {
		id: string;
		widget_type: string;
		props?: { slot?: string };
	}

	// Only ad_slot widgets belong in this rail — anything else an admin
	// assigns to a left sidebar resolves but does not render here.
	const adSlots = $derived(
		(widgets as AdWidget[]).filter((w) => w && w.widget_type === 'ad_slot')
	);
</script>

{#each adSlots as widget (widget.id)}
	<aside class="left-rail" aria-label={t('a11y.sidebar', 'Sidebar')}>
		<AdSlot slot={widget.props?.slot ?? 'left_rail'} format="skyscraper" />
	</aside>
{/each}

<style>
	.left-rail {
		position: sticky;
		top: 0;
		height: 100vh;
		display: flex;
		align-items: center;
		justify-content: center;
		width: var(--left-rail-width);
	}
</style>
