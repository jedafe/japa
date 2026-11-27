<script lang="ts">
	/**
	 * SpaceCard — the §05 hub-redesign directory space card (the mockup's
	 * subspace card): cover image with a members badge, name + verified
	 * badge, one-line description, keyword tag pills, stat rows (active
	 * discussions / upcoming events), and a bottom row pairing the
	 * latest-member avatar stack with the caller's join action.
	 *
	 * Used by the /spaces directory grid and the space Subspaces tab —
	 * both read the same GET /spaces row shape (header_url, keywords,
	 * member_avatars, upcoming_event_count, discussion_count).
	 *
	 * Ref: japaconnect-engineering/sections/05-geo-first-space-system.md
	 */
	import type { Snippet } from 'svelte';
	import { MessageSquare, Calendar, Users } from 'lucide-svelte';
	import VerifiedBadge from '#lib/components/ui/VerifiedBadge.svelte';
	import EntityHoverChip from '#lib/components/ui/EntityHoverChip.svelte';
	import { t } from '#lib/i18n/index.js';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';
	import { formatMetric } from '#lib/components/hub/charts/format.js';
	import type { Space } from '#lib/types/index.js';

	interface Props {
		space: Space;
		/** Space-type display name (e.g. "Community") — shown beside the
		 *  title when set. */
		typeName?: string;
		/** Bottom-right action — SpaceJoinButton snippet from the caller. */
		actions?: Snippet;
	}

	let { space, typeName = '', actions }: Props = $props();

	const href = $derived(`/spaces/${space.slug}`);
	const membersLabel = $derived(
		t('meta.members', '{n} members').replace('{n}', formatMetric(space.member_count))
	);
	const tagPills = $derived((space.keywords ?? []).slice(0, 3));
	const avatars = $derived((space.member_avatars ?? []).filter((a) => a && a.length > 0));
	const avatarOverflow = $derived(Math.max(0, space.member_count - avatars.length));

	// Deterministic cover tint — same hue derivation as DirectoryCard so a
	// space without a header image still gets a stable branded gradient.
	const hue = $derived.by(() => {
		let h = 0;
		for (let i = 0; i < space.id.length; i++) h = (h * 31 + space.id.charCodeAt(i)) % 360;
		return h;
	});
	const coverFallbackStyle = $derived(
		`background-image: linear-gradient(135deg, hsl(${hue} 65% 55% / 0.45), hsl(${hue} 65% 40% / 0.2))`
	);
</script>

<article
	class="flex flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-shadow hover:shadow-md"
>
	<!-- Cover — header_url with a members-count badge; tinted gradient
	     fallback when the space has no header image. -->
	<a {href} class="relative block h-32 shrink-0 overflow-hidden" tabindex="-1" aria-hidden="true">
		{#if space.header_url}
			<img
				src={remoteImageUrl(space.header_url)}
				alt=""
				loading="lazy"
				decoding="async"
				class="h-full w-full object-cover"
			/>
		{:else}
			<div class="h-full w-full" style={coverFallbackStyle}></div>
		{/if}
		<span
			class="absolute right-2 top-2 rounded-full bg-white/90 px-2.5 py-0.5 text-[11px] font-medium text-[var(--color-text)] shadow-sm backdrop-blur-sm"
		>
			{membersLabel}
		</span>
	</a>

	<div class="flex flex-1 flex-col gap-2 p-3">
		<!-- Title + verified badge (+ optional type name). -->
		{#snippet titleRow()}
			<span class="flex min-w-0 items-center gap-1 text-sm font-semibold text-[var(--color-text)]">
				<span class="truncate">{space.name}</span>
				<VerifiedBadge
					verified={!!space.is_primary}
					size={15}
				/>
			</span>
			{#if typeName}
				<span class="block truncate text-xs text-[var(--color-muted)]">{typeName}</span>
			{/if}
		{/snippet}
		{#if space.id}
			<EntityHoverChip
				type="space"
				id={space.id}
				slug={space.slug}
				name={space.name}
				avatar={space.logo_url}
				description={space.description}
				count={space.member_count}
				verified={!!space.is_primary}
				class="flex min-w-0 items-center gap-3 text-start"
			>
				{#snippet trigger()}
					{@render titleRow()}
				{/snippet}
			</EntityHoverChip>
		{:else}
			<a {href} class="flex min-w-0 items-center gap-3">
				{@render titleRow()}
			</a>
		{/if}

		{#if space.description}
			<p class="line-clamp-2 text-xs text-[var(--color-text-subtle)]">{space.description}</p>
		{/if}

		<!-- Keyword tag pills — decorative chips (the card itself links to
		     the space; pill links would nest anchors). -->
		{#if tagPills.length > 0}
			<div class="flex flex-wrap gap-1.5">
				{#each tagPills as kw (kw)}
					<span
						class="rounded-full bg-[var(--color-primary)]/10 px-2.5 py-0.5 text-[11px] font-medium text-[var(--color-primary)]"
					>
						{kw}
					</span>
				{/each}
			</div>
		{/if}

		<!-- Stat rows — label left, count right. The events row renders only
		     when the space actually has upcoming entries. -->
		<div class="mt-auto flex flex-col gap-1 text-xs">
			<p class="flex items-center justify-between text-[var(--color-muted)]">
				<span class="inline-flex items-center gap-1.5">
					<MessageSquare size={12} /> {t('space.card_active_discussions', 'Active discussions')}
				</span>
				<span class="font-semibold text-[var(--color-text)]">{space.discussion_count ?? 0}</span>
			</p>
			{#if (space.upcoming_event_count ?? 0) > 0}
				<p class="flex items-center justify-between text-[var(--color-muted)]">
					<span class="inline-flex items-center gap-1.5">
						<Calendar size={12} /> {t('space.events.upcoming', 'Upcoming events')}
					</span>
					<span class="font-semibold text-[var(--color-text)]">{space.upcoming_event_count}</span>
				</p>
			{/if}
		</div>

		<!-- Bottom row — latest-member avatar stack (+N overflow) left,
		     join action right. -->
		<div class="flex items-center justify-between gap-2 pt-1">
			<div class="flex min-w-0 items-center">
				{#if avatars.length > 0}
					<span class="flex -space-x-2 rtl:space-x-reverse">
						{#each avatars as url, i (i)}
							<img
								src={remoteImageUrl(url)}
								alt=""
								loading="lazy"
								decoding="async"
								class="h-7 w-7 rounded-full border-2 border-[var(--color-surface)] object-cover"
							/>
						{/each}
					</span>
				{:else if space.member_count > 0}
					<span class="grid h-7 w-7 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)]">
						<Users size={13} />
					</span>
				{/if}
				{#if avatarOverflow > 0 && avatars.length > 0}
					<span class="ms-1.5 text-[11px] font-medium text-[var(--color-muted)]">+{avatarOverflow}</span>
				{/if}
			</div>
			{#if actions}
				<div class="shrink-0">
					{@render actions()}
				</div>
			{/if}
		</div>
	</div>
</article>
