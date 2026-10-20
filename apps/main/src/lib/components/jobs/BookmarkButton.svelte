<script lang="ts">
	import { Bookmark } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { untrack } from 'svelte';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import { getViewerState, updateViewerState } from '$lib/api/viewerState';
	import { page } from '$app/state';

	interface Props {
		entityType: string;
		entityId: string;
		bookmarked: boolean;
		count?: number;
	}

	let {
		entityType,
		entityId,
		bookmarked: initial = false,
		count
	}: Props = $props();

	let isOn = $state(untrack(() => initial));
	let currentCount = $state(untrack(() => count));
	let loading = $state(false);

	// Hydrate real bookmark state — callers pass the SSR guess (usually
	// false); without this a saved job shows as unsaved and can't be
	// un-bookmarked. getViewerState batches all cards into one request.
	// Guests skip the call — /me/content-states is auth-gated (401 spam).
	$effect(() => {
		if (!page.data?.isAuthed) return;
		if (entityType === 'content' || entityType === 'blog_post') {
			void getViewerState(entityId).then((s) => (isOn = s.bookmarked));
		}
	});

	async function toggle() {
		if (loading) return;
		// Guests can't bookmark — redirect to login instead of hitting a 401.
		if (!(await requireAuthOrRedirect())) return;
		loading = true;
		const wasOn = isOn;
		isOn = !wasOn;
		if (currentCount !== undefined) currentCount += wasOn ? -1 : 1;

		try {
			// Ref: japaconnect-engineering/sections/14-content-type-system.md
			// Content/blog_post bookmarks use the entity-scoped routes
			// POST/DELETE /content/{id}/bookmark (api-routes-catalog L272-273).
			// Forum thread bookmarks use POST /forum/bookmarks + DELETE
			// /forum/bookmarks/{bookmark_id} — a different flow that requires
			// the bookmark row ID, so it is handled by the forum UI directly,
			// not this generic button.
			if (entityType === 'content' || entityType === 'blog_post') {
				// The api client never throws — it returns { error } — so
				// check the result and revert the optimistic toggle on any
				// failure (previously failures silently stuck "on").
				const res = wasOn
					? await api.delete(`/content/${entityId}/bookmark`)
					: await api.post(`/content/${entityId}/bookmark`, {});
				if (res.error) throw new Error(res.error.message);
				updateViewerState(entityId, { bookmarked: !wasOn });
			} else {
				throw new Error(`BookmarkButton does not support entity_type "${entityType}"`);
			}
		} catch {
			// Revert on failure.
			isOn = wasOn;
			if (currentCount !== undefined) currentCount += wasOn ? 1 : -1;
		} finally {
			loading = false;
		}
	}
</script>

<button
	type="button"
	onclick={toggle}
	disabled={loading}
	class="inline-flex items-center gap-1.5 rounded-[var(--radius)] px-2 py-1 text-sm text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
	aria-pressed={isOn}
	aria-label={t('bookmarks.toggle', 'Bookmark')}
>
	<Bookmark
		size={16}
		class={isOn ? 'fill-[var(--color-primary)] text-[var(--color-primary)]' : ''}
	/>
	{#if currentCount !== undefined}
		<span class="tabular-nums">{currentCount}</span>
	{/if}
</button>
