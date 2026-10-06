<script lang="ts">
	import { t } from '$lib/i18n';
	import { formatDuration } from '$lib/format';
	import { PlayCircle, Flame } from 'lucide-svelte';
	import type { Enrollment, SessionUser } from '$lib/types';
	import { enrollmentsStore } from '$lib/stores/enrollments.svelte';

	interface Props {
		user: SessionUser | null;
	}
	let { user }: Props = $props();

	// Use the shared academy_enrollments store so all widgets stay in sync after
	// mutations (enroll, mark-complete). The store deduplicates fetches —
	// multiple widgets mounting at the same time share a single request.
	$effect(() => {
		if (user) {
			enrollmentsStore.fetch();
		}
	});

	const academy_enrollments = $derived(enrollmentsStore.academy_enrollments ?? []);
	const loading = $derived(enrollmentsStore.loading && academy_enrollments.length === 0);

	// The most recently active enrollment is the "continue" target.
	const continueItem = $derived(
		academy_enrollments
			.slice()
			.sort((a, b) => (b.last_activity_at ?? '').localeCompare(a.last_activity_at ?? ''))
			.find((e) => e.progress_pct < 100) ?? academy_enrollments[0]
	);
</script>

{#if loading}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="jc-skeleton mb-3 h-4 w-24 rounded"></div>
		<div class="jc-skeleton h-16 w-full rounded"></div>
	</section>
{:else if continueItem}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">
			{t('academy.continue_learning')}
		</h2>
		<a
			href="/tracks/{continueItem.track_slug}"
			class="block rounded-[var(--radius)] p-2 hover:bg-[var(--color-surface-subtle)]"
		>
			{#if continueItem.thumbnail_url}
				<img
					src={continueItem.thumbnail_url}
					alt={continueItem.track_title}
					class="mb-2 h-20 w-full rounded-[var(--radius)] object-cover"
				/>
			{/if}
			<p class="truncate text-sm font-medium text-[var(--color-text)]">{continueItem.track_title}</p>
			<p class="mt-1 text-xs text-[var(--color-muted)]">
				{continueItem.lessons_completed ?? 0}/{continueItem.lessons_total ?? 0} {t('academy.lessons')} · {continueItem.progress_pct}%
			</p>
			<!-- Progress bar -->
			<div class="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
				<div
					class="h-full rounded-full bg-[var(--color-progress)]"
					style="width: {continueItem.progress_pct}%"
				></div>
			</div>
		</a>
		{#if continueItem.next_lesson}
			<a
				href="/tracks/{continueItem.track_slug}/{continueItem.next_lesson.module_slug}/{continueItem.next_lesson.slug}"
				class="mt-3 flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm text-[var(--color-primary)] hover:bg-[var(--color-surface-subtle)]"
			>
				<PlayCircle size={16} />
				<span class="truncate">{t('academy.up_next')}: {continueItem.next_lesson.title}</span>
			</a>
		{/if}
	</section>
{:else if user}
	<!-- No academy_enrollments yet — show a streak placeholder + explore CTA -->
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
			<Flame size={16} class="text-[var(--level-intermediate)]" />
			{t('academy.streak')}
		</h2>
		<p class="text-sm text-[var(--color-muted)]">{t('academy.no_enrollments')}</p>
		<a
			href="/"
			class="mt-3 inline-block rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
		>
			{t('academy.explore')}
		</a>
	</section>
{/if}
