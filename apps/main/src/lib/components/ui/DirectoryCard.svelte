<script lang="ts">
	/**
	 * DirectoryCard — social-platform style entity card for directory/list
	 * pages (spaces, people, organizations, tags, universities).
	 *
	 * Layout: avatar + title/subtitle header row, optional description,
	 * optional icon meta row, optional bottom action area. No banner/header
	 * image — the avatar fallback uses a deterministic tint derived from
	 * hueSeed so the same entity always gets the same colour.
	 *
	 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md
	 */
	import type { Snippet } from 'svelte';
	import { BadgeCheck } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import { remoteImageUrl } from '$lib/utils/remoteImage';
	import type { IconComponent } from './icons';

	export interface CardMetaItem {
		icon: IconComponent;
		text: string;
	}

	interface Props {
		href: string;
		title: string;
		subtitle?: string;
		description?: string | null;
		/** Resolved image URL for the avatar (avatar_url / logo_url). */
		imageUrl?: string | null;
		/** Fallback text when no image (initials or single letter). */
		fallback?: string;
		/** Fallback icon when no image and no fallback text (e.g. Hash, Building2). */
		icon?: IconComponent;
		/** Circular avatar — people/profiles. Square (rounded) for orgs/spaces. */
		round?: boolean;
		verified?: boolean;
		/** Seed for the deterministic avatar tint — defaults to href. */
		hueSeed?: string;
		meta?: CardMetaItem[];
		/** Optional leading content in the header row (e.g. a select checkbox). */
		leading?: Snippet;
		/** Optional action rendered in the top-right of the header row. */
		headerAction?: Snippet;
		/** Bottom action area — Follow/Join buttons. FollowButton and
		 * SpaceJoinButton are auto-stretched: the primary button flexes and any
		 * icon-only toggle keeps its natural width. */
		actions?: Snippet;
		/** Show a secondary "View profile" link in the bottom action area. */
		viewProfile?: boolean;
		viewProfileLabel?: string;
		/** Optional entity type for the hover chip. When set, the header is
		 * wrapped in an EntityHoverChip (user, space, organization, or source).
		 */
		entityType?: 'user' | 'space' | 'organization' | 'source';
		entityId?: string;
		entitySlug?: string | null;
		entityCount?: number;
		entityIsFollowing?: boolean;
	}

	let {
		href,
		title,
		subtitle = '',
		description = null,
		imageUrl = null,
		fallback = '',
		icon = undefined,
		round = false,
		verified = false,
		hueSeed = '',
		meta = [],
		leading,
		headerAction,
		actions,
		viewProfile = false,
		viewProfileLabel,
		entityType,
		entityId,
		entitySlug,
		entityCount,
		entityIsFollowing
	}: Props = $props();

	const resolvedViewLabel = $derived(viewProfileLabel ?? t('entity.view_profile', 'View profile'));

	const hue = $derived.by(() => {
		const seed = hueSeed || href;
		let h = 0;
		for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) % 360;
		return h;
	});
	const tintStyle = $derived(
		`background-image: linear-gradient(135deg, hsl(${hue} 65% 55% / 0.3), hsl(${hue} 65% 45% / 0.15))`
	);
	const FallbackIcon = $derived(icon);
</script>

<div
	class="flex flex-col gap-2 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-shadow hover:shadow-md"
>
	<div class="flex items-start gap-3">
		{@render leading?.()}
		{#if entityType && entityId}
			<EntityHoverChip
				type={entityType}
				id={entityId}
				slug={entitySlug}
				name={title}
				avatar={imageUrl}
				description={description}
				count={entityCount}
				isFollowing={entityIsFollowing}
				class="flex min-w-0 flex-1 items-center gap-3 text-start"
			>
				{#snippet trigger()}
					{@render headerBody()}
				{/snippet}
			</EntityHoverChip>
		{:else}
			<a {href} class="flex min-w-0 flex-1 items-center gap-3">
				{@render headerBody()}
			</a>
		{/if}
		{#if headerAction}
			<div class="ms-auto flex items-start">
				{@render headerAction()}
			</div>
		{/if}
	</div>

	{#snippet headerBody()}
		{#if imageUrl}
			<img
				src={remoteImageUrl(imageUrl)}
				alt=""
				loading="lazy"
				decoding="async"
				class="h-10 w-10 shrink-0 bg-[var(--color-surface)] object-cover {round ? 'rounded-full' : 'rounded-[var(--radius)]'}"
			/>
		{:else}
			<span
				class="grid h-10 w-10 shrink-0 place-items-center font-semibold leading-none text-[var(--color-text-subtle)] {round ? 'rounded-full' : 'rounded-[var(--radius)]'} {fallback.length > 1 ? 'px-0.5 text-[10px]' : 'text-sm'}"
				style={tintStyle}
			>
				{#if FallbackIcon}
					<FallbackIcon size={18} />
				{:else}
					{fallback}
				{/if}
			</span>
		{/if}
		<span class="min-w-0 flex-1">
			<span class="flex items-center gap-1 text-sm font-semibold text-[var(--color-text)]">
				<span class="truncate">{title}</span>
				{#if verified}
					<BadgeCheck size={15} class="shrink-0 text-[var(--color-primary)]" aria-label={t('organizations.verified', 'Verified')} />
				{/if}
			</span>
			{#if subtitle}
				<span class="block truncate text-xs text-[var(--color-muted)]">{subtitle}</span>
			{/if}
		</span>
	{/snippet}

	{#if description}
		<p class="line-clamp-3 text-xs text-[var(--color-text-subtle)]">{description}</p>
	{/if}

	{#if meta.length > 0}
		<p class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--color-muted)]">
			{#each meta as item}
				{@const MetaIcon = item.icon}
				<span class="inline-flex items-center gap-1"><MetaIcon size={12} />{item.text}</span>
			{/each}
		</p>
	{/if}

	{#if viewProfile || actions}
		<div class="mt-auto flex flex-col gap-2 pt-1">
			{#if actions}
				<div class="[&>button]:w-full [&>div]:w-full [&>div>button:first-child]:flex-1 [&_button]:justify-center">
					{@render actions()}
				</div>
			{/if}
			{#if viewProfile}
				<button
					type="button"
					onclick={() => goto(href)}
					class="w-full rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-border)]"
				>
					{resolvedViewLabel}
				</button>
			{/if}
		</div>
	{/if}
</div>
