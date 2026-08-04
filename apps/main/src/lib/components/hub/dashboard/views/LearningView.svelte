<script lang="ts">
	import { t } from '$lib/i18n';
	import { Chart, Svg, Bars, Axis } from 'layerchart';
	import DashboardPanel from '$lib/components/hub/DashboardPanel.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { GraduationCap } from 'lucide-svelte';

	interface Enrollment {
		id: string;
		title?: string;
		track_slug?: string;
		progress?: number;
		thumbnail_url?: string;
	}

	let { data }: { data: unknown } = $props();
	const enrollments = $derived((data as Enrollment[] | null) ?? []);

	// Phase 18.16 — progress chart series. Each bar represents one enrollment's
	// completion percentage. Ref: ADR-059, ADR-060
	const progressSeries = $derived(
		enrollments
			.slice(0, 10)
			.map((en, i) => ({
				x: i,
				y: Math.min(100, Math.round(en.progress ?? 0)),
				label: en.title ?? en.id
			}))
	);

	// Aggregate stats for the summary header.
	const avgProgress = $derived(
		enrollments.length > 0
			? Math.round(enrollments.reduce((sum, e) => sum + (e.progress ?? 0), 0) / enrollments.length)
			: 0
	);
	const completedCount = $derived(enrollments.filter((e) => (e.progress ?? 0) >= 100).length);
</script>

<section class="space-y-4">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.learning')}</h1>
	</header>

	{#if enrollments.length > 0}
		<!-- Phase 18.16 — progress summary cards -->
		<div class="grid grid-cols-3 gap-3">
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
				<p class="text-xs text-[var(--color-muted)]">{t('dashboard.learning.enrolled', 'Enrolled')}</p>
				<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{enrollments.length}</p>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
				<p class="text-xs text-[var(--color-muted)]">{t('dashboard.learning.avg_progress', 'Avg progress')}</p>
				<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{avgProgress}%</p>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
				<p class="text-xs text-[var(--color-muted)]">{t('dashboard.learning.completed', 'Completed')}</p>
				<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{completedCount}</p>
			</div>
		</div>

		<!-- Phase 18.16 — progress chart (LayerChart BarChart) -->
		<DashboardPanel titleKey="dashboard.learning.progress_chart">
			<div class="h-48">
				<Chart data={progressSeries} x="x" y="y">
					<Svg>
						<Axis placement="left" ticks={4} format={(v: number) => `${v}%`} />
						<Bars
							fill="var(--color-primary)"
							stroke="var(--color-surface)"
							strokeWidth={1}
						/>
					</Svg>
				</Chart>
			</div>
		</DashboardPanel>
	{/if}

	<DashboardPanel titleKey="dashboard.learning.continue">
		{#if enrollments.length === 0}
			<EmptyState icon={GraduationCap} title={t('dashboard.learning.empty_title')} body={t('dashboard.learning.empty_body')} />
		{:else}
			<ul class="space-y-3">
				{#each enrollments as en (en.id)}
					<li class="flex items-center gap-4 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
						{#if en.thumbnail_url}
							<img src={en.thumbnail_url} alt="" class="h-12 w-12 rounded object-cover" />
						{/if}
						<div class="min-w-0 flex-1">
							<p class="truncate text-sm font-medium text-[var(--color-text)]">{en.title ?? en.id}</p>
							<div class="mt-1 h-2 overflow-hidden rounded-full bg-[var(--color-border)]">
								<div class="h-full bg-[var(--color-primary)]" style="width: {Math.min(100, Math.round(en.progress ?? 0))}%"></div>
							</div>
							<p class="mt-1 text-xs text-[var(--color-muted)]">{Math.round(en.progress ?? 0)}%</p>
						</div>
						<a
							href="https://learn.japaconnect.com/tracks/{en.track_slug ?? en.id}"
							class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-white"
						>
							{t('dashboard.learning.resume')}
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</DashboardPanel>
</section>
