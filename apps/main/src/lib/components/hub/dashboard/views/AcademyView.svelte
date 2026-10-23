<script lang="ts">
	import { t, i18n } from '$lib/i18n';
	import { Chart, Bars, Tooltip } from 'layerchart';
	import DashboardPanel from '$lib/components/hub/DashboardPanel.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { GraduationCap } from 'lucide-svelte';
	import { academyUrl } from '$lib/appOrigins';
	import type { AcademyEnrollment, LessonNavRef } from '$lib/types';

	let { data }: { data: unknown } = $props();
	const academy_enrollments = $derived((data as AcademyEnrollment[] | null) ?? []);

	// Phase 18.16 — progress chart series. Each bar represents one enrollment's
	// completion percentage. `x` is the URL-safe track slug for the band scale;
	// `title` is the user-facing track title used in the tooltip.
	const progressSeries = $derived(
		academy_enrollments
			.slice(0, 10)
			.map((en) => ({
				x: en.track_slug,
				y: Math.min(100, Math.round(en.progress_pct ?? 0)),
				title: en.track_title ?? en.track_slug ?? en.id
			}))
	);

	// Aggregate stats for the summary header.
	const avgProgress = $derived(
		academy_enrollments.length > 0
			? Math.round(academy_enrollments.reduce((sum, e) => sum + (e.progress_pct ?? 0), 0) / academy_enrollments.length)
			: 0
	);
	const completedCount = $derived(academy_enrollments.filter((e) => (e.progress_pct ?? 0) >= 100).length);

	/** Resume path: next lesson when known, otherwise the track landing page.
	 *  Slugs are URL-encoded so bad data (e.g. spaces in track_slug) does not
	 *  produce an invalid cross-app href. */
	function resumePath(en: AcademyEnrollment): string {
		const trackSlug = encodeURIComponent(en.track_slug);
		if (en.next_lesson) {
			const nl = en.next_lesson as LessonNavRef;
			const moduleSlug = encodeURIComponent(nl.module_slug);
			const lessonSlug = encodeURIComponent(nl.slug);
			return `/tracks/${trackSlug}/${moduleSlug}/${lessonSlug}`;
		}
		return `/tracks/${trackSlug}`;
	}

	// academyUrl throws when PUBLIC_JC_ACADEMY_ORIGIN is unset — guard so a
	// missing env doesn't crash the render (falls back to '#').
	function safeAcademyUrl(path: string): string {
		try { return academyUrl(path); } catch { return '#'; }
	}

	/** Label for the resume CTA: "Start" for untouched first lessons,
	 *  "Resume" when in progress, "View track" when there is no next lesson. */
	function resumeLabel(en: AcademyEnrollment): string {
		if (!en.next_lesson) return t('dashboard.academy.view_track', 'View track');
		if ((en.progress_pct ?? 0) === 0) return t('dashboard.academy.start', 'Start');
		return t('dashboard.academy.resume', 'Resume');
	}

	// Format an ISO 8601 timestamp as a short, locale-aware date.
	function formatDate(iso: string | undefined | null): string {
		if (!iso) return '—';
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return '—';
		return d.toLocaleDateString(i18n.locale, { year: 'numeric', month: 'short', day: 'numeric' });
	}

	/** Activity line: "Last active {date}" if last_activity_at exists,
	 *  otherwise "Enrolled {date}". */
	function activityLine(en: AcademyEnrollment): string {
		const date = en.last_activity_at ?? en.enrolled_at;
		if (en.last_activity_at) {
			return t('dashboard.academy.last_active', 'Last active {date}', { date: formatDate(date) });
		}
		return t('dashboard.academy.enrolled_on', 'Enrolled {date}', { date: formatDate(date) });
	}
</script>

<section class="space-y-4">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.academy')}</h1>
	</header>

	{#if academy_enrollments.length > 0}
		<!-- Phase 18.16 — progress summary cards -->
		<div class="grid grid-cols-3 gap-3">
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
				<p class="text-xs text-[var(--color-muted)]">{t('dashboard.academy.enrolled', 'Enrolled')}</p>
				<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{academy_enrollments.length}</p>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
				<p class="text-xs text-[var(--color-muted)]">{t('dashboard.academy.avg_progress', 'Avg progress')}</p>
				<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{avgProgress}%</p>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
				<p class="text-xs text-[var(--color-muted)]">{t('dashboard.academy.completed', 'Completed')}</p>
				<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{completedCount}</p>
			</div>
		</div>

		<!-- Phase 18.16 — progress chart (LayerChart BarChart).
			 Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
			 /me/enrollments returns a per-track progress snapshot, so the
			 chart uses one bar per track (x=track_slug, y=progress_pct). -->
		<DashboardPanel titleKey="dashboard.academy.progress_chart">
			<div class="h-48">
				<Chart
					data={progressSeries}
					x="x"
					y="y"
					bandPadding={0.3}
					padding={{ left: 48 }}
					axis="y"
					grid
					rule
					tooltipContext={{ mode: 'band' }}
					props={{
						yAxis: { ticks: 4, format: (v: number) => `${v}%` }
					}}
				>
					{#snippet tooltip({ context })}
						{@const d = context.tooltip.data as { x: string; y: number; title: string } | null}
						{#if d}
							<Tooltip.Root>
								<Tooltip.Header value={d.title} />
								<Tooltip.List>
									<Tooltip.Item
										label={t('dashboard.academy.progress', 'Progress')}
										value={d.y}
										format={(v: number) => `${v}%`}
										color="var(--chart-1, var(--color-primary))"
									/>
								</Tooltip.List>
							</Tooltip.Root>
						{/if}
					{/snippet}
					{#snippet marks({ context })}
						<Bars
							fill="var(--chart-1, var(--color-primary))"
							stroke="var(--color-surface)"
							strokeWidth={1}
							radius={4}
						/>
					{/snippet}
				</Chart>
			</div>
		</DashboardPanel>
	{/if}

	<DashboardPanel titleKey="dashboard.academy.continue">
		{#if academy_enrollments.length === 0}
			<EmptyState icon={GraduationCap} title={t('dashboard.academy.empty_title')} body={t('dashboard.academy.empty_body')} />
		{:else}
			<ul class="space-y-3">
				{#each academy_enrollments as en (en.id)}
					<li class="flex items-center gap-4 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
						{#if en.thumbnail_url}
							<img src={en.thumbnail_url} alt="" class="h-12 w-12 rounded object-cover" />
						{:else}
							<span class="grid h-12 w-12 shrink-0 place-items-center rounded bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
								<GraduationCap size={20} />
							</span>
						{/if}
						<div class="min-w-0 flex-1">
							<p class="truncate text-sm font-medium text-[var(--color-text)]">{en.track_title}</p>
							<div class="mt-1 h-2 overflow-hidden rounded-full bg-[var(--color-border)]">
								<div class="h-full bg-[var(--color-primary)]" style="width: {Math.min(100, Math.round(en.progress_pct ?? 0))}%"></div>
							</div>
							<p class="mt-1 text-xs text-[var(--color-muted)]">
								{Math.round(en.progress_pct ?? 0)}%
								{#if (en.lessons_total ?? 0) > 0}
									· {en.lessons_completed ?? 0}/{en.lessons_total ?? 0} {t('academy.lessons', 'lessons')}
								{/if}
							</p>
							<div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-[var(--color-muted)]">
								{#if en.level}
									<span class="rounded-full bg-[var(--color-surface-subtle)] px-1.5 py-0.5 capitalize">{en.level}</span>
								{/if}
								{#if en.access_type}
									<span class="rounded-full bg-[var(--color-surface-subtle)] px-1.5 py-0.5 capitalize">{en.access_type}</span>
								{/if}
								<span class="ms-auto">{activityLine(en)}</span>
							</div>
						</div>
						<a
							href={safeAcademyUrl(resumePath(en))}
							class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)]"
						>
							{resumeLabel(en)}
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</DashboardPanel>
</section>
