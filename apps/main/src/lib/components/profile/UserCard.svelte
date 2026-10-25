<script lang="ts">
	/**
	 * UserCard — People-directory card on /users. Profile-preview layout
	 * inspired by the profile mockup: pastel header, large centered avatar,
	 * identity block, tier pill, stats row, and a full-width profile action.
	 *
	 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md
	 */
	import { page } from '$app/state';
	import { ArrowRight, BadgeCheck, GraduationCap, MapPin } from 'lucide-svelte';
	import { t, i18n } from '$lib/i18n';
	import FollowButton from '$lib/components/ui/FollowButton.svelte';
	import { remoteImageUrl } from '$lib/utils/remoteImage';
	import type { PublicUser } from '$lib/types';

	let { user }: { user: PublicUser } = $props();

	const hue = $derived.by(() => {
		let h = 0;
		for (let i = 0; i < user.id.length; i++) h = (h * 31 + user.id.charCodeAt(i)) % 360;
		return h;
	});
	const tintStyle = $derived(
		`background-image: linear-gradient(135deg, hsl(${hue} 65% 55% / 0.30), hsl(${hue} 65% 45% / 0.12))`
	);
	const initials = $derived(
		(user.display_name?.charAt(0) ?? user.username?.charAt(0) ?? '?').toUpperCase()
	);
	const joinDate = $derived(
		new Date(user.created_at).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'short' })
	);
	const tierLabel = $derived(t(`users.profile_type_${user.profile_type}`, user.profile_type));
	// §07 verification badge — the badge reflects the granted flag, not the
	// tier (a business/pro account is not automatically verified).
	const verified = $derived(Boolean(user.verified));
	const isOwn = $derived(page.data.user?.id === user.id);
</script>

<div
	class="flex flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-shadow hover:shadow-md"
>
	<!-- Header — user cover or soft tint with decorative shapes -->
	<div class="relative h-24">
		{#if user.cover_url}
			<img
				src={remoteImageUrl(user.cover_url)}
				alt=""
				loading="lazy"
				class="absolute inset-0 h-full w-full object-cover"
			/>
		{:else}
			<div class="absolute inset-0" style={tintStyle}></div>
		{/if}
		<div
			class="pointer-events-none absolute -end-6 -top-8 h-24 w-24 rounded-full bg-[var(--color-surface)]/25"
		></div>
		<div
			class="pointer-events-none absolute -start-8 bottom-0 h-20 w-20 rounded-full bg-[var(--color-surface)]/15"
		></div>

		{#if !isOwn}
			<div class="absolute start-3 top-3">
				<FollowButton entityType="user" entityId={user.id} compact />
			</div>
		{/if}
		{#if verified}
			<span
				class="absolute end-3 top-3 grid h-7 w-7 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)]"
				title={tierLabel}
			>
				<BadgeCheck size={16} />
			</span>
		{/if}
	</div>

	<!-- Avatar overlapping the header — relative so it paints above the
	     position:relative cover block it overlaps via -mt-12. -->
	<div class="relative -mt-12 flex justify-center px-4">
		{#if user.avatar_url}
			<img
				src={remoteImageUrl(user.avatar_url)}
				alt={user.display_name ?? user.username}
				loading="lazy"
				class="h-24 w-24 rounded-full border-4 border-[var(--color-surface)] object-cover"
			/>
		{:else}
			<span
				class="grid h-24 w-24 place-items-center rounded-full border-4 border-[var(--color-surface)] text-2xl font-bold text-[var(--color-text-subtle)]"
				style={tintStyle}
			>
				{initials}
			</span>
		{/if}
	</div>

	<div class="flex flex-1 flex-col items-center px-5 pb-5 pt-2 text-center">
		<a href="/u/{user.username}" class="group min-w-0">
			<span class="block truncate text-base font-bold text-[var(--color-text)] group-hover:underline">
				{user.display_name ?? user.username}
			</span>
			<span class="block truncate text-xs text-[var(--color-muted)]">@{user.username}</span>
		</a>

		{#if user.location}
			<span
				class="mt-1.5 inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary)]"
			>
				<MapPin size={12} />{user.location}
			</span>
		{/if}
		{#if user.bio}
			<p class="mt-1.5 line-clamp-2 text-xs text-[var(--color-text-subtle)]">{user.bio}</p>
		{/if}

		<span
			class="mt-2.5 inline-flex items-center gap-1.5 rounded-full bg-[var(--color-surface-subtle)] px-3 py-1 text-[11px] font-medium text-[var(--color-text-subtle)]"
		>
			<GraduationCap size={12} />
			{tierLabel} · {t('profile.joined', 'Joined')} {joinDate}
		</span>

		<div
			class="mt-4 flex w-full divide-x divide-[var(--color-border)] border-t border-[var(--color-border)] pt-3"
		>
			<div class="flex-1">
				<p class="text-lg font-bold text-[var(--color-primary)]">{user.follower_count}</p>
				<p class="text-[11px] text-[var(--color-muted)]">
					{t('profile.followers_label', 'Followers')}
				</p>
			</div>
			<div class="flex-1">
				<p class="text-lg font-bold text-[var(--color-primary)]">{user.following_count}</p>
				<p class="text-[11px] text-[var(--color-muted)]">
					{t('profile.following_label', 'Following')}
				</p>
			</div>
			{#if user.points != null}
				<div class="flex-1">
					<p class="text-lg font-bold text-[var(--color-primary)]">{user.points}</p>
					<p class="text-[11px] text-[var(--color-muted)]">
						{t('profile.points_label', 'Points')}
					</p>
				</div>
			{/if}
		</div>

		<a
			href="/u/{user.username}"
			class="mt-4 flex w-full items-center justify-center gap-2 rounded-xl bg-[var(--color-primary)] px-4 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
		>
			{t('entity.view_profile', 'View profile')}
			<span class="grid h-6 w-6 place-items-center rounded-full bg-white/20">
				<ArrowRight class="rtl:-scale-x-100" size={14} />
			</span>
		</a>
	</div>
</div>
