<script lang="ts">
	/**
	 * Reusable hover chip for users, spaces, and organizations.
	 * Wraps a bits-ui LinkPreview (hover card primitive) and fetches
	 * the public entity detail on first open so follower/member counts and
	 * descriptions are available without over-fetching in the feed list.
	 */
	import { LinkPreview } from 'bits-ui';
	import { Users, Loader2 } from 'lucide-svelte';
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
	}

	let {
		type,
		id,
		slug,
		name,
		avatar,
		description,
		count,
		isFollowing = false
	}: Props = $props();

	let open = $state(false);
	let loading = $state(false);
	let fetched = $state(false);
	// svelte-ignore state_referenced_locally
	let detailDescription = $state<string | null | undefined>(description);
	// svelte-ignore state_referenced_locally
	let detailCount = $state<number | undefined>(count);

	const href = $derived(
		type === 'user' ? `/u/${slug}` : type === 'space' ? `/spaces/${slug}` : `/org/${slug}`
	);
	const followType: FollowableType = $derived(type === 'user' ? 'user' : type === 'space' ? 'space' : 'organization');
	const followSlug = $derived(type === 'space' ? slug : undefined);
	const entitySlug = $derived(type === 'space' ? slug : type === 'organization' ? slug : undefined);
	const countLabel = $derived(
		type === 'space'
			? t('entity.members', 'members')
			: t('entity.followers', 'followers')
	);

	async function loadDetail() {
		if (fetched || loading) return;
		loading = true;
		const path = type === 'user' ? `/users/${slug}` : type === 'space' ? `/spaces/${slug}` : `/organizations/${slug}`;
		const res = await api.get<unknown>(path);
		if (!res.error && res.data && typeof res.data === 'object') {
			const d = res.data as Record<string, unknown>;
			// Try to extract description/bio and count from the public detail shape.
			if (typeof d.bio === 'string' && d.bio) detailDescription = d.bio;
			else if (typeof d.description === 'string' && d.description) detailDescription = d.description;
			else if (typeof d.tagline === 'string' && d.tagline) detailDescription = d.tagline;

			if (typeof d.follower_count === 'number') detailCount = d.follower_count;
			else if (typeof d.member_count === 'number') detailCount = d.member_count;
			else if (type === 'space' && d.space && typeof d.space === 'object') {
				const s = d.space as Record<string, unknown>;
				if (typeof s.member_count === 'number') detailCount = s.member_count;
				if (typeof s.description === 'string' && s.description) detailDescription = s.description;
			}
		}
		fetched = true;
		loading = false;
	}

	function onOpenChange(next: boolean) {
		open = next;
		if (next) loadDetail();
	}

	function stopPropagation(e: Event) {
		e.stopPropagation();
	}
</script>

<LinkPreview.Root {open} onOpenChange={onOpenChange} openDelay={350} closeDelay={200}>
	<LinkPreview.Trigger
		{href}
		class="inline-flex max-w-full items-center gap-1.5 rounded-full bg-[var(--color-surface-subtle)] px-2 py-1 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface)] hover:text-[var(--color-text)]"
		onclick={stopPropagation}
	>
		{#if avatar}
			<img src={avatar} alt="" class="h-4 w-4 rounded-full object-cover" />
		{:else}
			<span class="grid h-4 w-4 place-items-center rounded-full bg-[var(--color-primary)] text-[8px] font-semibold text-[var(--color-primary-contrast)]">
				{name.slice(0, 1).toUpperCase()}
			</span>
		{/if}
		<span class="truncate">{name}</span>
	</LinkPreview.Trigger>
	<LinkPreview.Content
		side="top"
		align="start"
		sideOffset={8}
		class="z-50 w-72 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-lg"
	>
		<div class="flex items-start gap-3">
			{#if avatar}
				<img src={avatar} alt="" class="h-10 w-10 rounded-full object-cover" />
			{:else}
				<span class="grid h-10 w-10 place-items-center rounded-full bg-[var(--color-primary)] text-sm font-semibold text-[var(--color-primary-contrast)]">
					{name.slice(0, 2).toUpperCase()}
				</span>
			{/if}
			<div class="min-w-0 flex-1">
				<p class="truncate text-sm font-semibold text-[var(--color-text)]">{name}</p>
				<p class="truncate text-xs text-[var(--color-muted)]">{type === 'user' ? `@${slug}` : slug}</p>
			</div>
		</div>

		{#if detailDescription}
			<p class="mt-2 line-clamp-3 text-xs text-[var(--color-text-subtle)]">{detailDescription}</p>
		{/if}

		<div class="mt-3 flex items-center justify-between gap-2">
			<div class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
				<Users size={13} />
				{#if loading}
					<Loader2 size={12} class="animate-spin" />
				{:else if detailCount != null}
					<span>{detailCount.toLocaleString()} {countLabel}</span>
				{:else}
					<span>—</span>
				{/if}
			</div>
			<FollowButton
				entityType={followType}
				entityId={id}
				entitySlug={followType === 'space' ? slug : undefined}
				initiallyFollowing={isFollowing}
				compact
			/>
		</div>

		<button
			type="button"
			class="mt-3 w-full rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-border)]"
			onclick={(e) => {
				e.stopPropagation();
				goto(href);
			}}
		>
			{t('entity.view_profile', 'View profile')}
		</button>
	</LinkPreview.Content>
</LinkPreview.Root>
