<script lang="ts">
	/**
	 * Generic FollowButton — works for any followable entity (user, org,
	 * space, tag). Calls POST/DELETE on the appropriate follow
	 * endpoint based on the `entityType` prop.
	 *
	 * Ref: japaconnect-engineering/sections/07-organizations.md (org follow)
	 *      japaconnect-engineering/sections/08-spaces.md (space follow)
	 */
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { UserPlus, UserCheck, Loader2 } from 'lucide-svelte';
	import { toggleFollow, type FollowableType } from '$lib/api/hub';
	import { requireAuthOrRedirect } from '$lib/auth/guest';

	interface Props {
		entityType: FollowableType;
		entityId: string;
		/** Slug for spaces/tags/orgs (follow endpoints use slug, not ID). */
		entitySlug?: string;
		initiallyFollowing?: boolean;
		/** Compact mode — smaller padding for card overlays. */
		compact?: boolean;
		/** Optional callback invoked with the authoritative following state after a server response. */
		onChange?: (following: boolean) => void;
	}

	let {
		entityType,
		entityId,
		entitySlug,
		initiallyFollowing = false,
		compact = false,
		onChange
	}: Props = $props();

	let following = $state(untrack(() => initiallyFollowing));
	let pending = $state(false);

	async function toggle() {
		if (pending) return;
		// Guests can't follow — redirect to login instead of hitting a 401.
		if (!(await requireAuthOrRedirect())) return;
		const wasFollowing = following;
		following = !wasFollowing; // optimistic
		pending = true;

		const res = await toggleFollow(entityType, entityId, entitySlug, wasFollowing);

		// H2: read the response body for the authoritative following state.
		// On success, trust the server's value (it may differ from the
		// optimistic toggle — e.g. idempotent unfollow returns false).
		// On error, roll back to the previous state.
		if (res.error) {
			following = wasFollowing;
		} else if (res.data && typeof res.data.following === 'boolean') {
			following = res.data.following;
			onChange?.(following);
		}
		pending = false;
	}

	const sizeClass = $derived(compact ? 'px-3 py-1 text-xs' : 'px-4 py-1.5 text-sm');
</script>

<button
	type="button"
	onclick={toggle}
	disabled={pending}
	aria-busy={pending}
	class="flex shrink-0 items-center gap-1.5 rounded-full font-medium transition-colors disabled:opacity-50 {sizeClass} {following
		? 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
		: 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'}"
>
	{#if pending}
		<Loader2 size={compact ? 12 : 16} class="animate-spin" />
	{:else if following}
		<UserCheck size={compact ? 12 : 16} />
		{t('profile.following_label', 'Following')}
	{:else}
		<UserPlus size={compact ? 12 : 16} />
		{t('profile.follow', 'Follow')}
	{/if}
</button>
