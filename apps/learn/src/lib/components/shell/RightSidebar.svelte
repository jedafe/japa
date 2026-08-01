<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Search, Sun, Moon, Languages } from 'lucide-svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { t, i18n } from '$lib/i18n';
	import { switchLocale } from '$lib/appOrigins';
	import SearchAutocomplete from './SearchAutocomplete.svelte';
	import UserDropdown from './UserDropdown.svelte';
	import ContinueLearningCard from './ContinueLearningCard.svelte';
	import SidebarWidgetRenderer from './SidebarWidgetRenderer.svelte';
	import type { SessionUser } from '$lib/types';

	interface SidebarWidget {
		id: string;
		widget_type: string;
		label: string;
		props: Record<string, unknown>;
		sort_order: number;
	}

	interface Props {
		user: SessionUser | null;
		/** True on the Japa University host → creator-focused copy (ADR-063). */
		isUniversity?: boolean;
		/** Route-specific widgets rendered above the global defaults. */
		sidebar?: Snippet;
		/** Sidebar widgets from the Sidebar Management System (Section 59, ADR-066).
		 *  Resolved via /sidebars/resolve and rendered by SidebarWidgetRenderer. */
		rightSidebarWidgets?: unknown[];
	}

	let { user, isUniversity = false, sidebar, rightSidebarWidgets = [] }: Props = $props();

	// Normalize incoming widgets to the SidebarWidget shape.
	const widgets = $derived(
		(rightSidebarWidgets as SidebarWidget[]).filter((w) => w && typeof w.widget_type === 'string')
	);

	// Only show the language toggle when more than one language is enabled.
	// The enabled set is resolved server-side from i18n.enabled_languages; for
	// now Learn always offers en+fr, so the toggle is always shown.
	let showLanguageToggle = $derived(true);
</script>

<aside class="flex h-full flex-col gap-4 overflow-y-auto px-4 py-3">
	<!-- Sticky top bar: search + avatar -->
	<div
		class="sticky top-0 z-10 -mx-4 flex items-center gap-2 bg-[var(--color-bg)]/90 px-4 py-2 backdrop-blur"
	>
		<SearchAutocomplete />

		<button
			type="button"
			onclick={() => theme.toggle()}
			class="grid h-9 w-9 shrink-0 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={theme.current === 'dark' ? t('theme.toggle_light') : t('theme.toggle_dark')}
		>
			{#if theme.current === 'dark'}<Sun size={18} />{:else}<Moon size={18} />{/if}
		</button>

		{#if showLanguageToggle}
			<button
				type="button"
				onclick={() => switchLocale(i18n.locale === 'fr' ? 'en' : 'fr')}
				class="grid h-9 w-9 shrink-0 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
				aria-label={i18n.locale === 'fr' ? t('locale.switch_to_en') : t('locale.switch_to_fr')}
				title={i18n.locale === 'fr' ? 'English' : 'Français'}
			>
				<Languages size={18} />
			</button>
		{/if}

		<UserDropdown {user} />
	</div>

	<!-- Route-specific widgets -->
	{#if sidebar}
		{@render sidebar()}
	{/if}

	<!-- Global widget: continue-learning (CSR, session-guarded) -->
	{#if user}
		<ContinueLearningCard {user} {isUniversity} />
	{/if}

	<!-- Admin-managed sidebar widgets (Section 59, ADR-066).
	     Rendered below the route-specific snippet and ContinueLearningCard. -->
	{#each widgets as widget (widget.id)}
		<SidebarWidgetRenderer {widget} />
	{/each}
</aside>
