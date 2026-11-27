<script lang="ts">
	/**
	 * Space hub header — rounded cover card with a dark bottom fade. Per the
	 * mockup the ENTIRE identity block lives inside the cover: circular
	 * logo at bottom-left, white name + verified badge, location/type
	 * meta, a globe visibility line, and a compact bottom row where the
	 * stats strip (bold number over label, vertical dividers) sits left
	 * and the action pills (Follow, Invite, Wiki) sit right.
	 *
	 * Ref: .jeos/sections/05-geo-first-space-system.md
	 *      (Amendment addendum 2026-11-27: Space Hub Redesign)
	 */
	import { t } from '#lib/i18n/index.js';
	import {
		Bell, BellOff, BookOpen, Check, Flag, Globe, Loader2, UserPlus
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
		/** Meta line under the name — "Delta State, Nigeria". */
		locationLine?: string;
		/** Space-type display name ("/space-types" name for space_type_id). */
		typeName?: string;
		/** Stats strip rendered inside the cover bottom — caller picks the
		 *  per-class set (primary: followers/discussions/events/guides;
		 *  custom: members/discussions/posts/followers). */
		stats?: Stat[];
		authed: boolean;
		following: boolean;
		followPending?: boolean;
		muted?: boolean;
		mutePending?: boolean;
		/** Direct link to the destination wiki (primary spaces with a
		 *  wiki_type_configs row — same gate as the Wiki tab). */
		showWikiLink?: boolean;
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
		mutePending = false, showWikiLink = false, canInvite = false,
		onFollow, onMute, onReport, onInvite
	}: Props = $props();

	function fmt(n: number): string {
		return n >= 1000
			? `${(n / 1000).toFixed(n >= 10000 ? 0 : 1).replace(/\.0$/, '')}k`
			: String(n);
	}
</script>

<header class="overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]" use:imgLightbox>
	<!-- Cover — the mockup keeps the whole identity block inside the image:
	     logo + name + meta sit over the dark fade, and the stats strip
	     shares the bottom row with the action pills (stats left, actions
	     right) so the cover stays compact. -->
	<div class="relative h-48 overflow-hidden sm:h-52">
		{#if headerUrl}
			<img src={headerUrl} alt="" class="h-full w-full object-cover" />
		{:else}
			<div
				class="h-full w-full"
				style="background: linear-gradient(135deg, color-mix(in srgb, var(--color-primary) 30%, transparent) 0%, color-mix(in srgb, var(--color-primary) 8%, var(--color-surface-subtle)) 100%);"
			></div>
		{/if}
		<div class="pointer-events-none absolute inset-0 bg-gradient-to-t from-black/75 via-black/35 to-black/10"></div>

		<!-- Identity row — logo, name/meta. -->
		<div class="absolute inset-x-0 bottom-0 flex flex-col gap-3 px-4 pb-3.5 sm:px-5">
			<div class="flex items-end gap-3 sm:gap-4">
				<!-- Circular logo — falls back to the first letter. -->
				{#if logoUrl}
					<img
						src={logoUrl}
						alt={space.name}
						class="h-20 w-20 shrink-0 rounded-full bg-[var(--color-surface)] object-cover shadow-lg sm:h-24 sm:w-24"
					/>
				{:else}
					<span
						class="grid h-20 w-20 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-2xl font-bold text-[var(--color-primary-text)] shadow-lg sm:h-24 sm:w-24"
					>
						{space.name.charAt(0).toUpperCase()}
					</span>
				{/if}

				<div class="min-w-0 flex-1 pb-0.5 text-white">
					<h1 class="flex items-center gap-1.5 text-2xl font-bold drop-shadow-sm sm:text-3xl">
						<span class="truncate">{space.name}</span>
						{#if space.is_primary}
							<VerifiedBadge verified={true} type="platform" size={20} />
						{/if}
					</h1>
					<p class="mt-0.5 truncate text-sm text-white/90">
						{#if locationLine}{locationLine}{/if}{#if locationLine && typeName} · {/if}{#if typeName}{typeName}{/if}
					</p>
					<p class="mt-0.5 flex items-center gap-1 text-xs text-white/80">
						<Globe size={12} class="shrink-0" />
						<span class="capitalize">{t('space.visibility_label', '{visibility} space').replace('{visibility}', space.visibility)}</span>
					</p>
				</div>
			</div>

			<!-- Bottom row — stats strip (left) shares the row with the
			     action pills (right). Stats: bold number over small label,
			     thin vertical dividers (mockup). -->
			<div class="flex flex-wrap items-center gap-x-3 gap-y-2">
				{#if stats.length > 0}
					<div class="flex min-w-0 flex-1 items-center overflow-x-auto text-white">
						{#each stats as stat, i (stat.label)}
							<div class="shrink-0 px-4 first:pl-0 sm:px-6 {i > 0 ? 'border-l border-white/30' : ''}">
								<span class="block text-lg font-bold leading-tight sm:text-xl">{fmt(stat.value)}</span>
								<span class="block text-xs text-white/75">{stat.label}</span>
							</div>
						{/each}
					</div>
				{:else}
					<div class="flex-1"></div>
				{/if}

				<div class="flex shrink-0 items-center justify-end gap-2">
					{#if authed}
						<button
							type="button"
							onclick={onMute}
							disabled={mutePending}
							aria-busy={mutePending}
							title={muted ? t('space.unmute', 'Unmute') : t('space.mute', 'Mute')}
							class="grid h-9 w-9 shrink-0 place-items-center rounded-full border border-white/40 bg-white/10 text-white backdrop-blur-sm transition-colors hover:bg-white/20 disabled:opacity-50"
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
							class="grid h-9 w-9 shrink-0 place-items-center rounded-full border border-white/40 bg-white/10 text-white backdrop-blur-sm transition-colors hover:bg-white/20"
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
						class="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] shadow-md transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{#if followPending}
							<Loader2 size={16} class="animate-spin" />
						{:else if following}
							<Check size={16} />
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
							class="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-white/60 bg-white/10 px-4 py-2 text-sm font-medium text-white backdrop-blur-sm transition-colors hover:bg-white/20"
						>
							<UserPlus size={16} /> {t('space.invite.action', 'Invite')}
						</button>
					{/if}
					{#if showWikiLink}
						<a
							href="/wiki/{space.slug}"
							class="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-white/60 bg-white/10 px-4 py-2 text-sm font-medium text-white backdrop-blur-sm transition-colors hover:bg-white/20"
						>
							<BookOpen size={16} /> {t('space.wiki', 'Wiki')}
						</a>
					{/if}
				</div>
			</div>
		</div>
	</div>
</header>
