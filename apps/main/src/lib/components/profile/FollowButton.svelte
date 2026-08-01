<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { UserPlus, UserCheck } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { requireAuthOrRedirect } from '$lib/auth/guest';

	interface Props {
		userId: string;
		initiallyFollowing?: boolean;
	}
	let { userId, initiallyFollowing = false }: Props = $props();

	let following = $state(untrack(() => initiallyFollowing));
	let pending = $state(false);

	async function toggle() {
		if (pending) return;
		// Guests can't follow — redirect to login instead of hitting a 401.
		if (!(await requireAuthOrRedirect())) return;
		const wasFollowing = following;
		following = !wasFollowing; // optimistic
		pending = true;

		const res = wasFollowing
			? await api.delete(`/users/${userId}/follow`)
			: await api.post(`/users/${userId}/follow`);

		if (res.error) following = wasFollowing; // rollback
		pending = false;
	}
</script>

<button
	type="button"
	onclick={toggle}
	disabled={pending}
	aria-busy={pending}
	class="flex shrink-0 items-center gap-1.5 rounded-full px-4 py-1.5 text-sm font-medium transition-colors disabled:opacity-50 {following
		? 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
		: 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'}"
>
	{#if following}
		<UserCheck size={16} />
		{t('profile.following_label', 'Following')}
	{:else}
		<UserPlus size={16} />
		{t('profile.follow', 'Follow')}
	{/if}
</button>
