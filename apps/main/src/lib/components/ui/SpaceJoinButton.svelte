<script lang="ts">
	/**
	 * SpaceJoinButton — Join/Leave toggle for a space, with an independent
	 * Follow/Unfollow control. Joining auto-follows the space (BL-010 mirror:
	 * the follow service auto-joins on follow; here we do the reverse for the
	 * spaces directory CTA). The user may unfollow a joined space without
	 * leaving it.
	 *
	 * Ref: japaconnect-engineering/sections/05-geo-first-space-system.md (join endpoint)
	 *      japaconnect-engineering/sections/31-follow-system.md (follow endpoint)
	 */
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { LogIn, LogOut, UserPlus, UserCheck, Loader2 } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { requireAuthOrRedirect } from '$lib/auth/guest';

	interface Props {
		entityId: string;
		/** Slug for the space — join/follow endpoints are keyed by slug. */
		entitySlug: string;
		initiallyJoined?: boolean;
		/** Whether the caller already has a pending join request awaiting
		 * admin approval (request-only spaces) — is_pending from the API. */
		initiallyPending?: boolean;
		initiallyFollowing?: boolean;
		/** Compact mode — smaller padding for card overlays. */
		compact?: boolean;
		/** §05 amendment — primary (platform-run) spaces have no Join CTA:
		 *  Follow is the only action (they're auto-joined / have no owner). */
		followOnly?: boolean;
	}

	let {
		entityId,
		entitySlug,
		initiallyJoined = false,
		initiallyPending = false,
		initiallyFollowing = false,
		compact = false,
		followOnly = false
	}: Props = $props();

	let joined = $state(untrack(() => initiallyJoined));
	let requestPending = $state(untrack(() => initiallyPending));
	let following = $state(untrack(() => initiallyFollowing));
	let joinPending = $state(false);
	let followPending = $state(false);

	// Re-sync on client-side navigation between entities — the component
	// persists across [slug] param changes, so without this the previous
	// entity's join/follow state bleeds into the new page's buttons.
	$effect(() => {
		entityId;
		joined = initiallyJoined;
		requestPending = initiallyPending;
		following = initiallyFollowing;
		joinPending = false;
		followPending = false;
	});

	/** POST /spaces/{slug}/join toggles membership (join ↔ leave). A
	 * pending request is cancelled via DELETE /spaces/{slug}/leave —
	 * the pending row is user-owned and deletable (BUG-034). */
	async function toggleJoin() {
		if (joinPending) return;
		if (!(await requireAuthOrRedirect())) return;
		if (requestPending) {
			joinPending = true;
			const res = await api.delete<{ left?: boolean }>(`/spaces/${entitySlug}/leave`);
			if (!res.error) requestPending = false;
			joinPending = false;
			return;
		}
		const wasJoined = joined;
		joined = !wasJoined; // optimistic
		joinPending = true;

		const res = await api.post<{ joined?: boolean; pending?: boolean }>(
			`/spaces/${entitySlug}/join`
		);

		if (res.error) {
			joined = wasJoined; // roll back
		} else if (res.data) {
			if (typeof res.data.joined === 'boolean') joined = res.data.joined;
			// A pending join request (request-perm space) is not a real join —
			// surface the Pending state so the button doesn't silently flip
			// back to "Join" (audit fix).
			if (res.data.pending) {
				joined = false;
				requestPending = true;
			}
		}

		// Auto-follow on a successful join (best-effort — a follow failure
		// does not roll back the join). Only follow when transitioning to
		// joined and not already following.
		if (joined && !following && !res.data?.pending) {
			await doFollow(true);
		}
		joinPending = false;
	}

	/** POST /spaces/{slug}/follow toggles follow. Pass `want=true` to force
	 *  follow (used by auto-follow on join); omit for a normal toggle. */
	async function doFollow(want = false) {
		if (followPending) return;
		if (!(await requireAuthOrRedirect())) return;
		const wasFollowing = following;
		const next = want ? true : !wasFollowing;
		if (next === wasFollowing) return;
		following = next; // optimistic
		followPending = true;

		const path = `/spaces/${entitySlug}/follow`;
		const res = next
			? await api.post<{ following: boolean }>(path)
			: await api.delete<{ following: boolean }>(path);

		if (res.error) {
			following = wasFollowing; // roll back
		} else if (res.data && typeof res.data.following === 'boolean') {
			following = res.data.following;
		}
		followPending = false;
	}

	const sizeClass = $derived(compact ? 'px-3 py-1 text-xs' : 'px-4 py-1.5 text-sm');
	const iconSize = $derived(compact ? 12 : 16);
</script>

<div class="flex shrink-0 items-center gap-1.5">
	<!-- Join / Joined (membership toggle) — hidden on primary spaces. -->
	{#if !followOnly}
	<button
		type="button"
		onclick={toggleJoin}
		disabled={joinPending || requestPending}
		aria-busy={joinPending}
		class="flex items-center gap-1.5 rounded-full font-medium transition-colors disabled:opacity-50 {sizeClass} {joined || requestPending
			? 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
			: 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'}"
	>
		{#if joinPending}
			<Loader2 size={iconSize} class="animate-spin" />
		{:else if joined}
			<LogOut class="rtl:-scale-x-100" size={iconSize} />
			{t('spaces.joined_label', 'Joined')}
		{:else if requestPending}
			<LogIn class="rtl:-scale-x-100" size={iconSize} />
			{t('spaces.pending_label', 'Pending')}
		{:else}
			<LogIn class="rtl:-scale-x-100" size={iconSize} />
			{t('spaces.join', 'Join')}
		{/if}
	</button>
	{/if}

	<!-- Follow / Following — shown independently of membership so the
	     user can follow without joining or unfollow while staying joined.
	     In followOnly mode it carries a text label (it's the sole CTA). -->
	<button
			type="button"
			onclick={() => doFollow(false)}
			disabled={followPending}
			aria-busy={followPending}
			title={following
				? t('spaces.unfollow_title', 'Unfollow this space')
				: t('spaces.follow_title', 'Follow this space')}
			class="flex items-center gap-1 rounded-full border border-[var(--color-border)] font-medium transition-colors disabled:opacity-50 {sizeClass} {following
				? 'text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
				: 'text-[var(--color-primary)] hover:border-[var(--color-primary)]/40'}"
		>
			{#if followPending}
				<Loader2 size={iconSize} class="animate-spin" />
			{:else if following}
				<UserCheck size={iconSize} />
			{:else}
				<UserPlus size={iconSize} />
			{/if}
			{#if followOnly}
				{following ? t('profile.following_label', 'Following') : t('profile.follow', 'Follow')}
			{/if}
		</button>
</div>
