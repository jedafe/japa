<script lang="ts">
	/**
	 * Reusable hover chip for users, spaces, and organizations.
	 * Wraps a bits-ui LinkPreview (hover card primitive) and fetches
	 * the public entity detail on first open so follower/member counts and
	 * descriptions are available without over-fetching in the feed list.
	 */
	import { LinkPreview } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { BadgeCheck, Calendar, Globe, MapPin } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import FollowButton from '$lib/components/ui/FollowButton.svelte';
	import type { FollowableType } from '$lib/api/hub';

	interface Props {
		type: 'user' | 'space' | 'organization';
		id: string;
		slug: string;
		name: string;
		avatar?: string | null;
		/** Initial description/bio from the feed row; may be overwritten by fetched detail. */
		description?: string | null;
		/** Initial count from the feed row (followers for users/orgs; members for spaces). */
		count?: number;
		/** Viewer already follows this entity. */
		isFollowing?: boolean;
		/** Visual treatment: 'chip' (compact pill) or 'row' (source row with large avatar). */
		variant?: 'chip' | 'row';
		/** Secondary line shown under the name when variant='row'. */
		secondary?: string;
		/** Optional custom trigger content. When provided, the default chip/row markup is skipped. */
		trigger?: Snippet;
	}

	let {
		type,
		id,
		slug,
		name,
		avatar,
		description,
		count,
		isFollowing = false,
		variant = 'chip',
		secondary,
		trigger
	}: Props = $props();

	let open = $state(false);
	let loading = $state(false);
	let fetched = $state(false);
	// svelte-ignore state_referenced_locally
	let detailDescription = $state<string | null | undefined>(description);
	// svelte-ignore state_referenced_locally
	let detailCount = $state<number | undefined>(count);
	let detailName = $state<string | undefined>(undefined);
	let detailAvatar = $state<string | undefined>(undefined);
	let detailLocation = $state<string | undefined>(undefined);
	let detailWebsite = $state<string | undefined>(undefined);
	let detailCreatedAt = $state<string | undefined>(undefined);
	let detailVerified = $state<boolean | undefined>(undefined);
	let detailFollowingCount = $state<number | undefined>(undefined);
	let detailPostsCount = $state<number | undefined>(undefined);
	let detailUsername = $state<string | undefined>(undefined);

	const joinedDate = $derived(
		detailCreatedAt
			? new Date(detailCreatedAt).toLocaleDateString(undefined, { month: 'short', year: 'numeric' })
			: null
	);
	const websiteHref = $derived(
		detailWebsite
			? detailWebsite.startsWith('http')
				? detailWebsite
				: `https://${detailWebsite}`
			: undefined
	);
	const websiteHostname = $derived(
		websiteHref
			? (() => {
					try {
						return new URL(websiteHref).hostname;
					} catch {
						return detailWebsite ?? '';
					}
				})()
			: null
	);
	const avatarSrc = $derived(detailAvatar ?? avatar ?? undefined);
	const displayName = $derived(detailName ?? name);
	const handle = $derived(
		type === 'user' ? (detailUsername ?? slug) : slug
	);
	const stats = $derived(
		type === 'user'
			? [
					{ label: t('entity.stat_following', 'following'), value: detailFollowingCount },
					{ label: t('entity.stat_followers', 'followers'), value: detailCount },
					{ label: t('entity.stat_posts', 'posts'), value: detailPostsCount }
				]
			: type === 'space'
				? [
						{ label: t('entity.stat_members', 'members'), value: detailCount },
						{ label: t('entity.stat_posts', 'posts'), value: detailPostsCount }
					]
				: [
						{ label: t('entity.stat_followers', 'followers'), value: detailCount },
						{ label: t('entity.stat_posts', 'posts'), value: detailPostsCount }
					]
	);
	const visibleStats = $derived(stats.filter((s) => s.value != null));

	const href = $derived(
		type === 'user' ? (handle ? `/u/${handle}` : '') : type === 'space' ? `/spaces/${slug}` : `/org/${slug}`
	);
	const followType: FollowableType = $derived(type === 'user' ? 'user' : type === 'space' ? 'space' : 'organization');

	function pick(obj: Record<string, unknown>, ...keys: string[]) {
		for (const key of keys) {
			const v = obj[key];
			if (v != null) return v;
		}
		return undefined;
	}
	function pickString(obj: Record<string, unknown>, ...keys: string[]) {
		const v = pick(obj, ...keys);
		return typeof v === 'string' && v ? v : undefined;
	}
	function pickNumber(obj: Record<string, unknown>, ...keys: string[]) {
		const v = pick(obj, ...keys);
		return typeof v === 'number' ? v : undefined;
	}
	function pickBool(obj: Record<string, unknown>, ...keys: string[]) {
		const v = pick(obj, ...keys);
		return typeof v === 'boolean' ? v : undefined;
	}

	async function loadDetail() {
		if (fetched || loading) return;
		if (!slug) return;
		loading = true;
		const path = type === 'user' ? `/users/${slug}` : type === 'space' ? `/spaces/${slug}` : `/organizations/${slug}`;
		const res = await api.get<Record<string, unknown>>(path);
		if (!res.error && res.data && typeof res.data === 'object') {
			const d = res.data;
			// Unwrap nested entity payloads (d.space, d.organization, d.user, d.profile).
			const entity = (d.space && typeof d.space === 'object'
				? d.space
				: d.organization && typeof d.organization === 'object'
					? d.organization
					: d.user && typeof d.user === 'object'
						? d.user
						: d) as Record<string, unknown>;
			const profile = (entity.profile && typeof entity.profile === 'object'
				? entity.profile
				: entity) as Record<string, unknown>;

			const displayName =
				pickString(entity, 'name', 'display_name', 'title') ??
				pickString(profile, 'display_name', 'name');
			if (displayName) detailName = displayName;
			const username = pickString(entity, 'username') ?? pickString(profile, 'username');
			if (username) detailUsername = username;
			const avatarUrl =
				pickString(entity, 'avatar_url', 'logo_url', 'logo', 'image_url') ??
				pickString(profile, 'avatar_url', 'image_url');
			if (avatarUrl) detailAvatar = avatarUrl;
			const desc =
				pickString(profile, 'bio', 'description', 'tagline') ??
				pickString(entity, 'bio', 'description', 'tagline');
			if (desc) detailDescription = desc;
			const c =
				pickNumber(entity, 'follower_count', 'member_count', 'followers', 'members') ??
				pickNumber(profile, 'follower_count', 'member_count');
			if (c != null) detailCount = c;
			const loc =
				pickString(entity, 'location', 'country', 'city') ??
				pickString(profile, 'location', 'country', 'city');
			if (loc) detailLocation = loc;
			const site =
				pickString(entity, 'website', 'website_url', 'url') ??
				pickString(profile, 'website', 'website_url', 'url');
			if (site) detailWebsite = site;
			const created =
				pickString(entity, 'created_at', 'createdAt') ??
				pickString(profile, 'created_at', 'createdAt');
			if (created) detailCreatedAt = created;
			const verified =
				pickBool(entity, 'verified', 'is_verified') ??
				pickBool(profile, 'verified', 'is_verified');
			if (verified != null) detailVerified = verified;
			const following =
				pickNumber(entity, 'following_count', 'following', 'follows_count') ??
				pickNumber(profile, 'following_count', 'following', 'follows_count');
			if (following != null) detailFollowingCount = following;
			const posts =
				pickNumber(entity, 'post_count', 'posts_count', 'content_count', 'posts') ??
				pickNumber(profile, 'post_count', 'posts_count', 'content_count', 'posts');
			if (posts != null) detailPostsCount = posts;
		}
		fetched = true;
		loading = false;
	}

	function onOpenChange(next: boolean) {
		open = next;
		if (next) loadDetail();
	}

	function onTriggerClick(e: MouseEvent) {
		e.stopPropagation();
		if (href) goto(href);
	}
</script>

<LinkPreview.Root {open} onOpenChange={onOpenChange} openDelay={350} closeDelay={200}>
	<LinkPreview.Trigger
		class={variant === 'row'
			? 'flex w-full min-w-0 items-center gap-3 text-left'
			: 'inline-flex max-w-full items-center gap-1.5 rounded-full bg-[var(--color-surface-subtle)] px-2 py-1 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface)] hover:text-[var(--color-text)]'}
		onclick={onTriggerClick}
	>
		{#if trigger}
			{@render trigger()}
		{:else if variant === 'row'}
			{#if avatar}
				<img src={avatar} alt="" class="h-10 w-10 rounded-full object-cover" />
			{:else}
				<span class="grid h-10 w-10 place-items-center rounded-full bg-[var(--color-primary)] text-sm font-semibold text-[var(--color-primary-contrast)]">
					{name.slice(0, 2).toUpperCase()}
				</span>
			{/if}
			<div class="min-w-0 flex-1">
				<p class="truncate text-sm font-semibold text-[var(--color-text)]">{name}</p>
				{#if secondary}
					<p class="truncate text-xs text-[var(--color-muted)]">{secondary}</p>
				{/if}
			</div>
		{:else}
			{#if avatar}
				<img src={avatar} alt="" class="h-4 w-4 rounded-full object-cover" />
			{:else}
				<span class="grid h-4 w-4 place-items-center rounded-full bg-[var(--color-primary)] text-[8px] font-semibold text-[var(--color-primary-contrast)]">
					{name.slice(0, 1).toUpperCase()}
				</span>
			{/if}
			<span class="truncate">{name}</span>
		{/if}
	</LinkPreview.Trigger>
	<LinkPreview.Portal>
		<LinkPreview.Content
			side="bottom"
			align="start"
			sideOffset={8}
			class="z-50 w-80 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-lg"
		>
		<div class="flex items-start gap-3">
			<div class="flex min-w-0 flex-1 items-center gap-3">
				{#if avatarSrc}
					<img src={avatarSrc} alt="" class="h-12 w-12 rounded-full object-cover" />
				{:else}
					<span class="grid h-12 w-12 place-items-center rounded-full bg-[var(--color-primary)] text-sm font-semibold text-[var(--color-primary-contrast)]">
						{displayName.slice(0, 2).toUpperCase()}
					</span>
				{/if}
				<div class="min-w-0">
					<div class="flex items-center gap-1">
						<p class="truncate text-sm font-semibold text-[var(--color-text)]">{displayName}</p>
						{#if detailVerified}
							<BadgeCheck size={14} class="shrink-0 text-[var(--color-primary)]" />
						{/if}
					</div>
					<p class="truncate text-xs text-[var(--color-muted)]">{handle ? (type === 'user' ? `@${handle}` : handle) : ''}</p>
				</div>
			</div>
			<FollowButton
				entityType={followType}
				entityId={id}
				entitySlug={followType === 'space' ? slug : undefined}
				initiallyFollowing={isFollowing}
				compact
			/>
		</div>

		{#if detailDescription}
			<p class="mt-3 line-clamp-3 text-xs text-[var(--color-text-subtle)]">{detailDescription}</p>
		{/if}

		{#if visibleStats.length > 0 || detailLocation || websiteHostname || joinedDate}
			<div class="mt-4 flex flex-col gap-2">
				{#if visibleStats.length > 0}
					<div class="flex items-center gap-4 text-xs text-[var(--color-muted)]">
						{#each visibleStats as stat (stat.label)}
							<span class="flex items-baseline gap-1">
								<span class="font-semibold text-[var(--color-text)]">{stat.value?.toLocaleString()}</span>
								{stat.label}
							</span>
						{/each}
					</div>
				{/if}
				{#if detailLocation || websiteHostname || joinedDate}
					<div class="flex flex-wrap gap-x-3 gap-y-1 text-xs text-[var(--color-muted)]">
						{#if detailLocation}
							<span class="inline-flex items-center gap-1">
								<MapPin size={12} />
								{detailLocation}
							</span>
						{/if}
						{#if websiteHref && websiteHostname}
							<a
								href={websiteHref}
								target="_blank"
								rel="noopener noreferrer"
								class="inline-flex max-w-[10rem] items-center gap-1 truncate hover:text-[var(--color-primary)]"
								onclick={(e) => e.stopPropagation()}
							>
								<Globe size={12} />
								{websiteHostname}
							</a>
						{/if}
						{#if joinedDate}
							<span class="inline-flex items-center gap-1">
								<Calendar size={12} />
								{t('entity.joined', 'Joined {date}', { date: joinedDate })}
							</span>
						{/if}
					</div>
				{/if}
			</div>
		{/if}

		<button
			type="button"
			class="mt-4 w-full rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-border)]"
			onclick={(e) => {
				e.stopPropagation();
				goto(href);
			}}
		>
			{t('entity.view_profile', 'View profile')}
		</button>
		</LinkPreview.Content>
	</LinkPreview.Portal>
</LinkPreview.Root>
