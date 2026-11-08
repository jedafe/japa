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
	import { t } from '#lib/i18n/index.js';
	import { UserPlus, UserCheck, Loader2 } from 'lucide-svelte';
	import { toggleFollow, type FollowableType } from '#lib/api/hub.js';
	import { requireAuthOrRedirect } from '#lib/auth/guest.js';

	interface Props {
		entityType: FollowableType;
		entityId: string;
		/** Slug for spaces/tags/orgs (follow endpoints use slug, not ID). */
		entitySlug?: string;
		initiallyFollowing?: boolean;
		/** Compact mode — smaller padding for card overlays. */
		compact?: boolean;
		/** Icon-only circular button — for tight card footers where the label
		 *  moves to aria-label/title (e.g. the japa-stage member card). */
		iconOnly?: boolean;
		/** Optional callback invoked with the authoritative following state after a server response. */
		onChange?: (following: boolean) => void;
		/** Optional callback invoked with the updated follower count (source follows include this). */
		onFollowerCountChange?: (count: number) => void;
		/** Optional label overrides (e.g. "Connect" on the profile hero). */
		followLabel?: string;
		followingLabel?: string;
	}

	let {
		entityType,
		entityId,
		entitySlug,
		initiallyFollowing = false,
		compact = false,
	iconOnly = false,
		onChange,
		onFollowerCountChange,
		followLabel,
		followingLabel
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
		// Source follows use `is_following` and also return `follower_count`.
		if (res.error) {
			following = wasFollowing;
		} else if (res.data) {
			if (typeof res.data.is_following === 'boolean') {
				following = res.data.is_following;
			} else if (typeof res.data.following === 'boolean') {
				following = res.data.following;
			}
			onChange?.(following);
			if (typeof res.data.follower_count === 'number') {
				onFollowerCountChange?.(res.data.follower_count);
			}
		}
		pending = false;
	}

	const sizeClass = $derived(compact ? 'px-3 py-1 text-xs' : 'px-4 py-1.5 text-sm');
	const iconSize = $derived(compact ? 12 : 16);
	const label = $derived(
		following
			? (followingLabel ?? t('profile.following_label', 'Following'))
			: (followLabel ?? t('profile.follow', 'Follow'))
	);
</script>

{#if iconOnly}
	<button
		type="button"
		onclick={toggle}
		disabled={pending}
		aria-busy={pending}
		aria-label={label}
		title={label}
		class="grid h-8 w-8 shrink-0 place-items-center rounded-full border transition-colors disabled:opacity-50 {following
			? 'border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
			: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary-text)]'}"
	>
		{#if pending}
			<Loader2 size={iconSize} class="animate-spin" />
		{:else if following}
			<UserCheck size={iconSize} />
		{:else}
			<UserPlus size={iconSize} />
		{/if}
	</button>
{:else}
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
			<Loader2 size={iconSize} class="animate-spin" />
		{:else if following}
			<UserCheck size={iconSize} />
			{label}
		{:else}
			<UserPlus size={iconSize} />
			{label}
		{/if}
	</button>
{/if}
