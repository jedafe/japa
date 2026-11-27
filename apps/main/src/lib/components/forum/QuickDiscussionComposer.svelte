<script lang="ts">
	/**
	 * Collapsed quick-discussion card (mockup) — avatar + "Share
	 * something…" input that opens the compose sheet in discussion mode.
	 * The bottom row mirrors the space-hub composer: Poll opens the
	 * composer (the poll builder lives inside it), Photo and Attach route
	 * to /create, and the teal Post pill opens the composer too — a
	 * collapsed card has no body to submit yet.
	 *
	 * Replaces the old "New discussion" PageAction on /thread.
	 */
	import { t } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import { BarChart3, Image, Paperclip } from 'lucide-svelte';
	import { compose } from '#lib/components/shell/compose.svelte.js';
	import { requireAuthOrRedirect } from '#lib/auth/guest.js';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';

	const avatar = $derived((page.data.user as { avatar_url?: string } | null)?.avatar_url ?? '');
	const initials = $derived(
		((page.data.user as { display_name?: string; username?: string } | null)?.display_name ??
			(page.data.user as { username?: string } | null)?.username ??
			'?'
		).charAt(0).toUpperCase()
	);

	async function openComposer() {
		if (!(await requireAuthOrRedirect())) return;
		compose.openDiscussion();
	}
</script>

<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
	<div class="flex items-center gap-3">
		{#if avatar}
			<img src={remoteImageUrl(avatar)} alt="" class="h-9 w-9 shrink-0 rounded-full object-cover" />
		{:else}
			<span class="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">
				{initials}
			</span>
		{/if}
		<button
			type="button"
			onclick={openComposer}
			class="min-w-0 flex-1 rounded-full border border-[var(--color-border)] bg-[var(--color-bg)] px-4 py-2 text-start text-sm text-[var(--color-muted)] transition-colors hover:border-[var(--color-primary)] hover:text-[var(--color-text-subtle)]"
		>
			{t('forum.composer.placeholder', 'Share something with the community…')}
		</button>
	</div>
	<div class="mt-2 flex items-center gap-1 border-t border-[var(--color-border)] pt-2">
		<a
			href="/create?type=blog_post"
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] px-3 py-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<Image size={16} class="text-[var(--color-primary-text)]" /> {t('forum.composer.photo', 'Photo')}
		</a>
		<button
			type="button"
			onclick={openComposer}
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] px-3 py-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<BarChart3 size={16} class="text-[var(--color-primary-text)]" /> {t('forum.composer.poll', 'Poll')}
		</button>
		<button
			type="button"
			onclick={openComposer}
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] px-3 py-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<Paperclip size={16} class="text-[var(--color-primary-text)]" /> {t('forum.composer.attach', 'Attach')}
		</button>
		<button
			type="button"
			onclick={openComposer}
			class="ms-auto inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-5 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
		>
			{t('forum.composer.post', 'Post')}
		</button>
	</div>
</div>
