<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Search, Sun, Moon, Languages } from 'lucide-svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { t, i18n } from '$lib/i18n';
	import SearchAutocomplete from './SearchAutocomplete.svelte';
	import UserDropdown from './UserDropdown.svelte';
	import ContinueLearningCard from './ContinueLearningCard.svelte';
	import type { SessionUser } from '$lib/types';

	interface Props {
		user: SessionUser | null;
		/** True on the Japa University host → creator-focused copy (ADR-063). */
		isUniversity?: boolean;
		/** Route-specific widgets rendered above the global defaults. */
		sidebar?: Snippet;
	}

	let { user, isUniversity = false, sidebar }: Props = $props();

	// Only show the language toggle when more than one language is enabled.
	// The enabled set is resolved server-side from i18n.enabled_languages; for
	// now Learn always offers en+fr, so the toggle is always shown.
	let showLanguageToggle = $derived(true);

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

	<!-- Global widget: continue-learning (CSR, session-guarded) -->
	{#if user}
		<ContinueLearningCard {user} {isUniversity} />
	{/if}
</aside>
