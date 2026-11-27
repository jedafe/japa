<script lang="ts">
	/**
	 * Space hub header — rounded cover card with a dark bottom fade, circular
	 * overlapping logo, name + meta line, an overlaid stats strip, and the
	 * action cluster (Follow/Join, Invite, Post, Mute, Report).
	 *
	 * Ref: .jeos/sections/05-geo-first-space-system.md
	 *      (Amendment addendum 2026-11-27: Space Hub Redesign)
	 */
	import { t } from '#lib/i18n/index.js';
	import {
		Bell, BellOff, BookOpen, Flag, Loader2, PenSquare, UserCheck, UserPlus
	} from 'lucide-svelte';
	import VerifiedBadge from '#lib/components/ui/VerifiedBadge.svelte';
	import { imgLightbox } from '#lib/lightbox.svelte.js';
	import type { Space } from '#lib/types/index.js';

	interface Stat {
		label: string;
		value: number;
	}

	interface Props {
		space: Space;
		/** Resolved imgproxy logo URL (country spaces already resolve to the
		 *  bundled flag artwork upstream). */
		logoUrl?: string;
		headerUrl?: string;
		/** Meta line under the name — "Lagos · Education community". */
		locationLine?: string;
		/** Space-type display name ("/space-types" name for space_type_id). */
		typeName?: string;
		/** Stats strip rendered over the cover bottom — caller picks the
		 *  per-class set (primary: followers/discussions/events/guides;
		 *  custom: members/discussions/posts/followers). */
		stats?: Stat[];
		authed: boolean;
		following: boolean;
		followPending?: boolean;
		muted?: boolean;
		mutePending?: boolean;
		/** Follow-only primary spaces hide Join — the follow button is the
		 *  main action for every class. */
		showWikiLink?: boolean;
		/** Composer gate — posting CTAs are hidden on geo spaces. */
		canPost?: boolean;
		/** Invite button is a custom-space feature (§05 hub-redesign). */
		canInvite?: boolean;
		onFollow?: () => void;
		onMute?: () => void;
		onReport?: () => void;
		onInvite?: () => void;
	}
	let {
		space, logoUrl = '', headerUrl = '', locationLine = '', typeName = '',
		stats = [], authed, following, followPending = false, muted = false,
		mutePending = false, showWikiLink = false, canPost = false,
		canInvite = false, onFollow, onMute, onReport, onInvite
	}: Props = $props();

	function fmt(n: number): string {
		return n >= 1000
			? `${(n / 1000).toFixed(n >= 10000 ? 0 : 1).replace(/\.0$/, '')}k`
			: String(n);
	}
</script>

<header class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]" use:imgLightbox>
	<!-- Cover — image with a dark bottom fade, or the gradient fallback.
	     Taller than the old strip (mockup): h-44 / sm:h-56. -->
	<div class="relative h-44 overflow-hidden sm:h-56">
		{#if headerUrl}
			<img src={headerUrl} alt="" class="h-full w-full object-cover" />
		{:else}
			<div
				class="h-full w-full"
				style="background: linear-gradient(135deg, color-mix(in srgb, var(--color-primary) 30%, transparent) 0%, color-mix(in srgb, var(--color-primary) 8%, var(--color-surface-subtle)) 100%);"
			></div>
		{/if}
		<div class="pointer-events-none absolute inset-x-0 bottom-0 h-2/3 bg-gradient-to-t from-black/70 via-black/30 to-transparent"></div>

		<!-- Stats strip — overlaid on the cover bottom (mockup). -->
		{#if stats.length > 0}
			<div class="absolute inset-x-0 bottom-0 flex items-center gap-1 px-4 pb-3 text-white sm:gap-2">
				{#each stats as stat, i (stat.label)}
					{#if i > 0}<span class="text-white/50" aria-hidden="true">·</span>{/if}
					<span class="text-xs font-medium sm:text-sm">
						<span class="font-semibold">{fmt(stat.value)}</span>
						<span class="text-white/80">{stat.label}</span>
					</span>
				{/each}
			</div>
		{/if}
	</div>

	<div class="relative px-4 pb-4">
		<div class="-mt-10 flex flex-col gap-3 sm:-mt-12 sm:flex-row sm:items-end sm:gap-4">
			<!-- Circular logo (mockup) — falls back to the first letter. -->
			{#if logoUrl}
				<img
					src={logoUrl}
					alt={space.name}
					class="h-20 w-20 shrink-0 rounded-full border-4 border-[var(--color-surface)] bg-[var(--color-surface)] object-cover shadow-lg sm:h-24 sm:w-24"
				/>
			{:else}
				<span
					class="grid h-20 w-20 shrink-0 place-items-center rounded-full border-4 border-[var(--color-surface)] bg-[var(--color-surface-subtle)] text-2xl font-bold text-[var(--color-primary-text)] shadow-lg sm:h-24 sm:w-24"
				>
					{space.name.charAt(0).toUpperCase()}
				</span>
			{/if}

			<div class="min-w-0 flex-1 pb-1">
				<h1 class="flex items-center gap-1.5 text-xl font-bold text-[var(--color-text)]">
					<span class="truncate">{space.name}</span>
					{#if space.is_primary}
						<VerifiedBadge verified={true} type="platform" size={18} />
					{/if}
				</h1>
				<p class="mt-0.5 truncate text-sm text-[var(--color-muted)]">
					{#if locationLine}{locationLine} · {/if}{#if typeName}{typeName} · {/if}{t('space.visibility_label', '{visibility} space').replace('{visibility}', space.visibility)}
				</p>
				{#if space.description}
					<p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{space.description}</p>
				{/if}
			</div>

			<div class="flex shrink-0 flex-wrap items-center gap-2 pb-1">
				{#if authed}
					<button
						type="button"
						onclick={onMute}
						disabled={mutePending}
						aria-busy={mutePending}
						title={muted ? t('space.unmute', 'Unmute') : t('space.mute', 'Mute')}
						class="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{#if mutePending}
							<Loader2 size={16} class="animate-spin" />
						{:else if muted}
							<BellOff size={16} />
						{:else}
							<Bell size={16} />
						{/if}
					</button>
					<button
						type="button"
						onclick={onReport}
						title={t('space.report', 'Report')}
						class="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<Flag size={16} />
					</button>
				{/if}
				<!-- Follow renders for guests too — the handler redirects to
				     /login before touching the API. -->
				<button
					type="button"
					onclick={onFollow}
					disabled={followPending}
					aria-busy={followPending}
					class="inline-flex shrink-0 items-center gap-1.5 rounded-full px-4 py-1.5 text-sm font-medium transition-colors disabled:opacity-50 {following
						? 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
						: 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'}"
				>
					{#if followPending}
						<Loader2 size={16} class="animate-spin" />
					{:else if following}
						<UserCheck size={16} />
						{t('profile.following_label', 'Following')}
					{:else}
						<UserPlus size={16} />
						{t('profile.follow', 'Follow')}
					{/if}
				</button>
				{#if canInvite}
					<button
						type="button"
						onclick={onInvite}
						class="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-[var(--color-border)] px-4 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<UserPlus size={16} /> {t('space.invite.action', 'Invite')}
					</button>
				{/if}
				{#if showWikiLink}
					<a
						href="/wiki/{space.slug}"
						class="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-[var(--color-border)] px-4 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<BookOpen size={16} /> {t('space.wiki', 'Wiki')}
					</a>
				{/if}
				{#if canPost}
					<a
						href="/create?type=blog_post&space={space.slug}"
						class="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
					>
						<PenSquare size={16} /> {t('space.post', 'Post')}
					</a>
				{/if}
			</div>
		</div>
	</div>
</header>
