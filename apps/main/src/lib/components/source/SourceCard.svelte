<script lang="ts">
	/**
	 * SourceCard — directory card for /sources.
	 *
	 * Mirrors the EntityHoverChip content for sources but renders it as a
	 * standalone card with no hover preview. Shows the source logo, name,
	 * domain, description, follower/post counts, and trust level.
	 */
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import { Users, FileText, Shield } from 'lucide-svelte';
	import { FollowButton } from '$lib/components/ui';
	import { remoteImageUrl } from '$lib/utils/remoteImage';
	import type { Source } from '$lib/types/sources';

	interface Props {
		source: Source;
	}

	let { source }: Props = $props();

	// Seed with the SSR value — the previous $state(0)+$effect pair rendered
	// "0 followers" server-side then jumped to the real count on hydration.
	let followerCount = $state(untrack(() => source.follower_count));

	$effect(() => {
		followerCount = source.follower_count;
	});

	const tintStyle = $derived.by(() => {
		const seed = source.slug || source.id;
		let h = 0;
		for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) % 360;
		return `background-image: linear-gradient(135deg, hsl(${h} 65% 55% / 0.3), hsl(${h} 65% 45% / 0.15))`;
	});

	const initials = $derived(source.name.slice(0, 2).toUpperCase());
	const trustLabel = $derived(`${t('sources.trust', 'Trust')} ${source.trust_level}`);

	// Third-party logo URLs can fail to load — some origins send
	// Cross-Origin-Resource-Policy: same-origin (CORB/CORB-blocked), others
	// simply 404. Fall back to the initials badge instead of a broken img.
	let logoFailed = $state(false);
</script>

<div
	class="flex flex-col gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-shadow hover:shadow-md"
>
	<div class="flex items-start justify-between gap-3">
		<a href="/sources/{source.slug}" class="flex min-w-0 flex-1 items-center gap-3">
			{#if source.logo_url && !logoFailed}
				<img
					src={remoteImageUrl(source.logo_url)}
					alt=""
					loading="lazy"
					decoding="async"
					referrerpolicy="no-referrer"
					onerror={() => (logoFailed = true)}
					class="h-12 w-12 shrink-0 rounded-full bg-[var(--color-surface)] object-cover"
				/>
			{:else}
				<span
					class="grid h-12 w-12 shrink-0 place-items-center rounded-full bg-[var(--color-primary)] text-sm font-semibold text-[var(--color-primary-contrast)]"
					style={tintStyle}
				>
					{initials}
				</span>
			{/if}
			<div class="min-w-0">
				<p class="truncate text-sm font-semibold text-[var(--color-text)]">{source.name}</p>
				{#if source.domain}
					<p class="truncate text-xs text-[var(--color-muted)]">{source.domain}</p>
				{/if}
			</div>
		</a>

		<FollowButton
			entityType="source"
			entityId={source.id}
			entitySlug={source.slug}
			initiallyFollowing={source.is_following ?? false}
			compact
			onFollowerCountChange={(n) => (followerCount = n)}
		/>
	</div>

	{#if source.description}
		<p class="line-clamp-3 text-xs text-[var(--color-text-subtle)]">{source.description}</p>
	{/if}

	<div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--color-muted)]">
		<span class="inline-flex items-center gap-1">
			<Users size={12} />
			<span class="font-semibold text-[var(--color-text)]">{followerCount.toLocaleString()}</span>
			{t('sources.followers', 'followers')}
		</span>
		<span class="inline-flex items-center gap-1">
			<FileText size={12} />
			<span class="font-semibold text-[var(--color-text)]">{source.post_count.toLocaleString()}</span>
			{t('sources.posts', 'posts')}
		</span>
		<span class="inline-flex items-center gap-1">
			<Shield size={12} />
			<span class="font-semibold text-[var(--color-text)]">{trustLabel}</span>
		</span>
	</div>

	<button
		type="button"
		onclick={() => goto(`/sources/${source.slug}`)}
		class="mt-auto w-full rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-border)]"
	>
		{t('sources.view', 'View source')}
	</button>
</div>
