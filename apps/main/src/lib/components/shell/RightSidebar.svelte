<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { Search, Sun, Moon, Languages } from 'lucide-svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { t, i18n } from '$lib/i18n';
	import SearchAutocomplete from './SearchAutocomplete.svelte';
	import UserDropdown from './UserDropdown.svelte';
	import { AdSlot } from '$lib/components/ui';
	import SidebarBlocks from './SidebarBlocks.svelte';
	import type { SessionUser, TagSuggestion, SpaceSuggestion } from '$lib/types';

	interface Props {
		user: SessionUser | null;
		trendingTags?: TagSuggestion[];
		suggestedSpaces?: SpaceSuggestion[];
		/** Comma-separated list of enabled language codes from i18n.enabled_languages. */
		enabledLanguages?: string;
		/** Route-specific widgets rendered above the global defaults. */
		sidebar?: Snippet;
		/** Page Builder blocks for this sidebar (sitewide, from admin). */
		rightSidebarBlocks?: unknown[];
	}

	let { user, trendingTags = [], suggestedSpaces = [], enabledLanguages = 'en,fr', sidebar, rightSidebarBlocks = [] }: Props = $props();

	// Only show the language toggle when more than one language is enabled.
	// Ref: conventions/system-settings-catalog.md (i18n.enabled_languages)
	let showLanguageToggle = $derived(enabledLanguages.split(',').filter(Boolean).length > 1);

	// ADR-060: Dashboard is private — no ad slots on /dashboard/* routes.
	let isDashboard = $derived(page.url.pathname.startsWith('/dashboard'));

	/** Current path without the leading slash, for the /fr /en redirect targets. */
	function currentPath(): string {
		if (typeof window === 'undefined') return '';
		return window.location.pathname.slice(1);
	}
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
			<a
				href={i18n.locale === 'fr' ? `/en/${currentPath()}` : `/fr/${currentPath()}`}
				class="grid h-9 w-9 shrink-0 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
				aria-label={i18n.locale === 'fr' ? t('locale.switch_to_en') : t('locale.switch_to_fr')}
				title={i18n.locale === 'fr' ? 'English' : 'Français'}
			>
				<Languages size={18} />
			</a>
		{/if}

		<UserDropdown {user} />
	</div>

	<!-- Route-specific widgets -->
	{#if sidebar}
		{@render sidebar()}
	{/if}

	<!-- Page Builder sidebar blocks (managed from admin /page-builder/sidebars) -->
	{#if rightSidebarBlocks.length > 0}
		<SidebarBlocks blocks={rightSidebarBlocks} />
	{/if}

	<!-- Phase 24 — sidebar_top ad slot (above global widgets). Suppressed on
	dashboard routes per ADR-060 (dashboard is private, no ad slots). -->
	{#if !isDashboard}
		<AdSlot slot="sidebar_top" format="square" />
	{/if}

	<!-- Global widget: trending tags -->
	{#if trendingTags.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('sidebar.trending_tags')}</h2>
			<ul class="flex flex-wrap gap-2">
				{#each trendingTags as tag (tag.id)}
					<li>
						<a
							href={`/tags/${tag.slug}`}
							class="inline-block rounded-full bg-[var(--color-surface-subtle)] px-3 py-1 text-xs text-[var(--color-text-subtle)] hover:text-[var(--color-primary)]"
							>#{tag.name}</a
						>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	<!-- Global widget: suggested spaces -->
	{#if suggestedSpaces.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('sidebar.suggested_spaces')}</h2>
			<ul class="space-y-2">
				{#each suggestedSpaces as space (space.id)}
					<li>
						<a href={`/spaces/${space.slug}`} class="flex items-center gap-2 rounded-[var(--radius)] p-1.5 hover:bg-[var(--color-surface-subtle)]">
							<span class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">{space.name.charAt(0).toUpperCase()}</span>
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm text-[var(--color-text)]">{space.name}</span>
								{#if space.member_count != null}
									<span class="block text-xs text-[var(--color-muted)]">{t('meta.members', '{n} members').replace('{n}', String(space.member_count))}</span>
								{/if}
							</span>
						</a>
					</li>
				{/each}
			</ul>
		</section>
	{/if}

	<!-- Phase 24 — sidebar_bottom ad slot (after global widgets). Suppressed on
	dashboard routes per ADR-060. -->
	{#if !isDashboard}
		<AdSlot slot="sidebar_bottom" format="banner" />
	{/if}
</aside>
