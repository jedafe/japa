<script lang="ts">
	/**
	 * Collapsed space-hub composer (§05 hub-redesign) — the "Share something
	 * with {space}…" card. Clicking opens the compose sheet in discussion
	 * mode with the space pre-scoped (compose.presetSpace); Poll opens it
	 * the same way (the poll builder lives inside the composer); Photo and
	 * Article route to /create with the space pre-attached.
	 */
	import { t } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import { BarChart3, Image, Newspaper } from 'lucide-svelte';
	import { compose } from '#lib/components/shell/compose.svelte.js';
	import { requireAuthOrRedirect } from '#lib/auth/guest.js';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';
	import type { Space } from '#lib/types/index.js';

	interface Props {
		space: Space;
	}
	let { space }: Props = $props();

	const avatar = $derived((page.data.user as { avatar_url?: string } | null)?.avatar_url ?? '');
	const initials = $derived(
		((page.data.user as { display_name?: string; username?: string } | null)?.display_name ??
			(page.data.user as { username?: string } | null)?.username ??
			'?'
		).charAt(0).toUpperCase()
	);

	async function openComposer() {
		if (!(await requireAuthOrRedirect())) return;
		compose.openDiscussion(false, { spaceId: space.id, spaceName: space.name });
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
			{t('space.composer.placeholder', 'Share something with {space}…').replace('{space}', space.name)}
		</button>
	</div>
	<div class="mt-2 flex items-center divide-x divide-[var(--color-border)] border-t border-[var(--color-border)] pt-2">
		<button
			type="button"
			onclick={openComposer}
			class="flex flex-1 items-center justify-center gap-1.5 rounded-[var(--radius)] py-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<BarChart3 size={16} class="text-[var(--color-primary-text)]" /> {t('space.composer.poll', 'Poll')}
		</button>
		<a
			href="/create?type=blog_post&space={space.slug}"
			class="flex flex-1 items-center justify-center gap-1.5 rounded-[var(--radius)] py-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<Image size={16} class="text-[var(--color-primary-text)]" /> {t('space.composer.photo', 'Photo')}
		</a>
		<a
			href="/create?type=blog_post&space={space.slug}"
			class="flex flex-1 items-center justify-center gap-1.5 rounded-[var(--radius)] py-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<Newspaper size={16} class="text-[var(--color-primary-text)]" /> {t('space.composer.article', 'Article')}
		</a>
	</div>
</div>
