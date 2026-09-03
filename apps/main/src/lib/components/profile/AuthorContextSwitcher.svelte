<script lang="ts">
	// Ref: docs/superpowers/specs/2026-09-03-unified-user-tiers-and-professional-profile-removal-design.md
	// Author context is now always the user account; Professional Profiles were removed.
	// This component is retained as a compatibility shim for any existing callers.
	import { t } from '$lib/i18n';
	import { User } from 'lucide-svelte';

	interface Props {
		/** Kept for API compatibility; never called because only the user context exists. */
		onSelected?: (profileId: string | null) => void;
	}

	let { onSelected }: Props = $props();

	function selectSelf() {
		onSelected?.(null);
	}
</script>

<button
	type="button"
	onclick={selectSelf}
	class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)]"
>
	<User class="h-4 w-4" />
	<span>{t('profiles.author_switcher.yourself', 'Yourself')}</span>
</button>
