<script lang="ts">
	// TrackCard — Learn's ONE variant-driven card (ADR-060, Ghost-style).
	// Thumbnail, title, LevelBadge, duration, instructor, rating, enrolled count,
	// and an enroll/continue/locked action. Never fork per level/type.
	//
	// The action state is CSR (enrollment status is user-specific — ADR-060 SSR/CSR
	// boundary). The card receives the public track data via props; the parent page
	// passes the enrollment state down after fetching it client-side.
	import { t } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import { formatDuration, formatCount, formatPriceKobo, isFreeForUser } from '$lib/format';
	import { Star, Users, Clock, PlayCircle, CheckCircle } from 'lucide-svelte';
	import LevelBadge from './LevelBadge.svelte';
	import type { TrackSummary, SessionUser } from '$lib/types';

	interface Props {
		track: TrackSummary;
		/** Current user — drives Pro pricing + enrollment actions. */
		user?: SessionUser | null;
		/** Enrollment state for this track (CSR-resolved by parent). */
		enrolled?: boolean;
		/** Progress pct if enrolled (0–100). */
		progressPct?: number;
	}
	let {
		track,
		user = null,
		enrolled = false,
		progressPct = 0
	}: Props = $props();

	const href = $derived(`/tracks/${track.slug}`);

	// Price badge — always shown on the unified University surface.
	const priceLabel = $derived(() => {
		if (isFreeForUser(track.price_kobo, user?.is_pro)) {
			return user?.is_pro ? t('learn.included_pro') : t('learn.free');
		}
		return formatPriceKobo(track.price_kobo);
	});
</script>

<a
	{href}
	class="group flex flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-shadow hover:shadow-md"
>
	<!-- Thumbnail -->
	<div class="relative aspect-video w-full overflow-hidden bg-[var(--color-surface-subtle)]">
		{#if track.thumbnail_url}
			<img
				src={track.thumbnail_url}
				alt=""
				loading="lazy"
				class="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
			/>
		{:else}
			<div class="flex h-full w-full items-center justify-center text-[var(--color-muted)]">
				<PlayCircle size={32} />
			</div>
		{/if}
		<!-- Level badge (top-left) -->
		<div class="absolute left-3 top-3">
			<LevelBadge level={track.level} />
		</div>
		<!-- Price badge (top-right) -->
		{#if priceLabel()}
			<span
				class="absolute right-3 top-3 rounded-full bg-[var(--color-surface)]/90 px-2.5 py-0.5 text-xs font-semibold text-[var(--color-text)] backdrop-blur"
			>
				{priceLabel()}
			</span>
		{/if}
	</div>

	<!-- Body -->
	<div class="flex flex-1 flex-col gap-2 p-4">
		<h3 class="line-clamp-2 text-base font-semibold text-[var(--color-text)]">{track.title}</h3>
		{#if track.subtitle}
			<p class="line-clamp-2 text-sm text-[var(--color-muted)]">{track.subtitle}</p>
		{/if}

		<!-- Instructor -->
		{#if track.instructor_name}
			<p class="text-xs text-[var(--color-text-subtle)]">
				{#if track.instructor_id}
					<span
						role="link"
						tabindex="0"
						class="cursor-pointer hover:text-[var(--color-primary)] hover:underline"
						onclick={(e) => { e.stopPropagation(); e.preventDefault(); goto(`/instructors/${track.instructor_id}`); }}
						onkeydown={(e) => { if (e.key === 'Enter') { e.stopPropagation(); e.preventDefault(); goto(`/instructors/${track.instructor_id}`); } }}
					>{track.instructor_name}</span>
				{:else}
					{track.instructor_name}
				{/if}
			</p>
		{/if}

		<!-- Stats row -->
		<div class="mt-auto flex flex-wrap items-center gap-x-4 gap-y-1 pt-2 text-xs text-[var(--color-muted)]">
			{#if track.duration_label ?? track.duration_iso}
				<span class="inline-flex items-center gap-1">
					<Clock size={12} />
					{track.duration_label ?? formatDuration(track.duration_iso)}
				</span>
			{/if}
			{#if track.rating_avg != null && track.rating_count}
				<span class="inline-flex items-center gap-1">
					<Star size={12} class="text-[var(--level-intermediate)]" />
					{track.rating_avg.toFixed(1)} ({formatCount(track.rating_count)})
				</span>
			{/if}
			{#if track.enrolled_count}
				<span class="inline-flex items-center gap-1">
					<Users size={12} />
					{formatCount(track.enrolled_count)}
				</span>
			{/if}
		</div>

		<!-- Action state (CSR-driven) -->
		<div class="pt-2">
			{#if enrolled}
				<div class="flex items-center gap-2 text-sm">
					{#if progressPct >= 100}
						<span class="inline-flex items-center gap-1 font-medium text-[var(--color-progress)]">
							<CheckCircle size={16} /> {t('learn.completed')}
						</span>
					{:else}
						<span class="inline-flex items-center gap-1 font-medium text-[var(--color-primary)]">
							<PlayCircle size={16} /> {t('learn.continue')}
						</span>
						<!-- Progress bar -->
						<div class="flex-1">
							<div class="h-1 w-full overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
								<div
									class="h-full rounded-full bg-[var(--color-progress)]"
									style="width: {progressPct}%"
								></div>
							</div>
						</div>
					{/if}
				</div>
			{:else}
				<span
					class="inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary)]"
				>
					{t('learn.enroll')}
				</span>
			{/if}
		</div>
	</div>
</a>
