<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Gauge, Lock, CheckCircle2, ArrowRight, TrendingUp } from 'lucide-svelte';
	import { LineChart } from 'layerchart';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type {
		AssessmentForm,
		AssessmentResult
	} from '$lib/types';
	import type { PageData } from './$types';

	// AssessmentHistoryEntry is exported from +page.server.ts and inferred by
	// $types via PageData, but we redeclare it here for direct use in
	// component state typing.
	interface AssessmentHistoryEntry {
		id: string;
		total_score: number;
		threshold_label?: string;
		submission_id?: string;
		created_at: string;
	}

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tools.readiness', 'Readiness Assessment — JapaConnect'));
	const description = $derived(
		t('tools.readiness_desc', 'Score your japa readiness across key dimensions.')
	);

	const form = $derived<AssessmentForm | null>(data.form);
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);
	const history = $derived<AssessmentHistoryEntry[]>(data.history ?? []);

	// Form state — answers keyed by field.key (matches field_values contract).
	let answers = $state<Record<string, string | string[] | number>>({});
	let submitting = $state(false);
	let submitError = $state<string | null>(null);
	let result = $state<AssessmentResult | null>(null);

	const fields = $derived(form?.fields ?? []);
	const answeredCount = $derived(
		Object.keys(answers).filter((k) => {
			const v = answers[k];
			if (Array.isArray(v)) return v.length > 0;
			return v !== undefined && v !== null && v !== '';
		}).length
	);
	const progressPct = $derived(fields.length > 0 ? Math.round((answeredCount / fields.length) * 100) : 0);

	const scorePct = $derived(
		result && form ? Math.round((result.total_score / Math.max(form.scoring_config.max_score, 1)) * 100) : 0
	);

	// Phase 33.02 — score history series for the LineChart. Each point is a
	// past submission's percentage score over time. The backend history
	// endpoint returns created_at (not submitted_at) and omits max_score, so
	// the percentage is derived from the form's scoring_config.max_score.
	const maxScore = $derived(form?.scoring_config.max_score ?? 1);
	const historySeries = $derived(
		history
			.slice()
			.reverse()
			.map((h) => ({
				date: new Date(h.created_at),
				score: Math.round((h.total_score / Math.max(maxScore, 1)) * 100),
				label: h.created_at
			}))
	);

	// Submit is async (Ref: §71): POST returns 202 with {submission_id,
	// status:"queued"}; the client polls GET /assessment/result/by-submission
	// until the worker writes the result (200) or the retry budget is spent.
	let polling = $state(false);

	async function submit() {
		if (!form) return;
		submitting = true;
		submitError = null;
		result = null;
		// Backend expects field_values (Ref: §71 handler), not answers.
		const res = await api.post<{ submission_id: string; status: string }>('/assessment/submit', {
			field_values: answers
		});
		if (res.error) {
			submitting = false;
			submitError = res.error.message;
			return;
		}
		const submissionId = res.data?.submission_id;
		if (!submissionId) {
			submitting = false;
			submitError = t('assessment.submit_error', 'Submission failed: no submission id returned.');
			return;
		}
		// Poll for the scored result.
		submitting = false;
		polling = true;
		try {
			result = await pollForResult(submissionId);
		} catch (e) {
			submitError = e instanceof Error ? e.message : t('assessment.scoring_timeout', 'Scoring is taking too long. Please try again.');
		} finally {
			polling = false;
		}
	}

	async function pollForResult(submissionId: string): Promise<AssessmentResult> {
		const maxAttempts = 20;
		const delayMs = 1000;
		for (let attempt = 0; attempt < maxAttempts; attempt++) {
			await new Promise((r) => setTimeout(r, delayMs));
			const res = await api.get<AssessmentResult | { status: string }>(
				`/assessment/result/by-submission/${submissionId}`
			);
			if (res.error) {
				throw new Error(res.error.message);
			}
			// 200 with a scored result has total_score; 202 pending has {status:"pending"}.
			const payload = res.data as AssessmentResult | { status: string };
			if (payload && typeof payload === 'object' && 'total_score' in payload) {
				return payload as AssessmentResult;
			}
		}
		throw new Error(t('assessment.scoring_timeout', 'Scoring is taking too long. Please try again.'));
	}

	function resetResult() {
		result = null;
		answers = {};
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Gauge size={20} /> {t('tools.readiness', 'Readiness Assessment')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else if !form}
		<div class="p-4">
			<LoadingSkeleton variant="lines" />
		</div>
	{:else if result}
		<!-- Results view -->
		<div class="p-4">
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-base font-medium text-[var(--color-text)]">
					{t('assessment.results', 'Your results')}
				</h2>
				<button
					type="button"
					onclick={resetResult}
					class="text-xs text-[var(--color-primary)] hover:underline"
				>
					{t('assessment.retake', 'Retake assessment')}
				</button>
			</div>

			<!-- Score gauge -->
			<div class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<div class="flex items-center gap-4">
					<div class="relative grid h-20 w-20 place-items-center">
						<svg class="h-20 w-20 -rotate-90" viewBox="0 0 80 80">
							<circle cx="40" cy="40" r="34" fill="none" stroke="var(--color-surface-subtle)" stroke-width="6" />
							<circle
								cx="40" cy="40" r="34" fill="none" stroke="var(--color-primary)" stroke-width="6"
								stroke-dasharray="{2 * Math.PI * 34}"
								stroke-dashoffset="{2 * Math.PI * 34 * (1 - scorePct / 100)}"
								stroke-linecap="round"
								class="transition-all duration-700"
							/>
						</svg>
						<span class="absolute text-lg font-semibold text-[var(--color-text)]">{scorePct}%</span>
					</div>
					<div class="min-w-0 flex-1">
						<span class="block text-sm font-medium text-[var(--color-text)]">
							{result.threshold_label}
						</span>
						<span class="mt-1 block text-xs text-[var(--color-muted)]">
							{t('assessment.score', 'Score')}: {result.total_score} / {form.scoring_config.max_score}
						</span>
					</div>
				</div>
			</div>

			<!-- Dimension breakdown -->
			{#if Object.keys(result.dimension_scores).length > 0}
				<div class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h3 class="mb-3 text-sm font-medium text-[var(--color-text)]">
						{t('assessment.dimensions', 'Dimension breakdown')}
					</h3>
					<div class="space-y-2">
						{#each Object.entries(result.dimension_scores) as [dim, score] (dim)}
							<div>
								<div class="flex items-center justify-between text-xs text-[var(--color-muted)]">
									<span class="capitalize">{dim.replace(/_/g, ' ')}</span>
									<span>{score}</span>
								</div>
								<div class="mt-1 h-1.5 overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
									<div
										class="h-full bg-[var(--color-primary)] transition-all"
										style="width: {Math.min(100, Math.round((score / form.scoring_config.max_score) * 100))}%"
									></div>
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/if}

			<!-- Roadmap -->
			{#if result.roadmap.length > 0}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h3 class="mb-3 text-sm font-medium text-[var(--color-text)]">
						{t('assessment.roadmap', 'Recommended next steps')}
					</h3>
					<ol class="space-y-3">
						{#each result.roadmap as step, i (step.title)}
							<li class="flex items-start gap-3">
								<span class="grid h-6 w-6 shrink-0 place-items-center rounded-full bg-[var(--color-primary)] text-xs font-medium text-white">
									{i + 1}
								</span>
								<span class="min-w-0 flex-1">
									<span class="block text-sm font-medium text-[var(--color-text)]">{step.title}</span>
									{#if step.description}
										<span class="mt-0.5 block text-xs text-[var(--color-muted)]">{step.description}</span>
									{/if}
									{#if step.content_slug}
										<a href="/{step.content_slug}" class="mt-1 inline-flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline">
											{t('assessment.read_more', 'Read more')} <ArrowRight size={12} />
										</a>
									{/if}
								</span>
							</li>
						{/each}
					</ol>
				</div>
			{/if}
		</div>
	{:else}
		<!-- Form view -->
		<div class="p-4">
			{#if !authed}
				<div class="mb-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs text-[var(--color-muted)]">
					{t('assessment.anon_hint', 'Sign in to save your score to history. Anonymous results are not saved.')}
				</div>
			{/if}

			{#if form.description}
				<p class="mb-4 text-sm text-[var(--color-muted)]">{form.description}</p>
			{/if}

			<!-- Progress bar -->
			<div class="mb-4">
				<div class="flex items-center justify-between text-xs text-[var(--color-muted)]">
					<span>{t('assessment.progress', 'Progress')}</span>
					<span>{answeredCount}/{fields.length}</span>
				</div>
				<div class="mt-1 h-2 overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
					<div class="h-full bg-[var(--color-primary)] transition-all" style="width: {progressPct}%"></div>
				</div>
			</div>

			<form onsubmit={(e) => { e.preventDefault(); submit(); }} class="space-y-4">
				{#each fields as field (field.key)}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
						<label class="block text-sm font-medium text-[var(--color-text)]" for={field.key}>
							{field.label}
							{#if field.is_required}<span class="text-[var(--color-primary)]">*</span>{/if}
						</label>
						{#if (field.field_type === 'select' || field.field_type === 'radio') && field.options.length > 0}
							<div class="mt-2 space-y-1">
								{#each field.options as opt (opt.value)}
									<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
										<input
											type="radio"
											name={field.key}
											value={opt.value}
											checked={answers[field.key] === opt.value}
											onchange={() => (answers = { ...answers, [field.key]: opt.value })}
											class="accent-[var(--color-primary)]"
										/>
										{opt.label}
									</label>
								{/each}
							</div>
						{:else if field.field_type === 'multiselect' && field.options.length > 0}
							<div class="mt-2 space-y-1">
								{#each field.options as opt (opt.value)}
									<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
										<input
											type="checkbox"
											value={opt.value}
											checked={Array.isArray(answers[field.key]) && (answers[field.key] as string[]).includes(opt.value)}
											onchange={(e) => {
												const current = Array.isArray(answers[field.key]) ? answers[field.key] as string[] : [];
												const next = e.currentTarget.checked
													? [...current, opt.value]
													: current.filter((v) => v !== opt.value);
												answers = { ...answers, [field.key]: next };
											}}
											class="accent-[var(--color-primary)]"
										/>
										{opt.label}
									</label>
								{/each}
							</div>
						{:else if field.field_type === 'number' || field.field_type === 'rating'}
							<div class="mt-2 flex items-center gap-3">
								<input
									type="range"
									min="0"
									max="100"
									value={(answers[field.key] as number) ?? 0}
									oninput={(e) => (answers = { ...answers, [field.key]: parseInt(e.currentTarget.value) })}
									class="flex-1 accent-[var(--color-primary)]"
								/>
								<span class="w-8 text-center text-sm font-medium text-[var(--color-text)]">
									{(answers[field.key] as number) ?? 0}
								</span>
							</div>
						{:else}
							<input
								id={field.key}
								type="text"
								placeholder={field.placeholder || ''}
								value={answers[field.key] ?? ''}
								oninput={(e) => (answers = { ...answers, [field.key]: (e.target as HTMLInputElement).value })}
								class="mt-2 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
							/>
						{/if}
					</div>
				{/each}

				{#if submitError}
					<p class="text-sm text-[var(--color-danger)]">{submitError}</p>
				{/if}

				<button
					type="submit"
					disabled={submitting || polling || answeredCount < fields.length}
					class="flex w-full items-center justify-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
				>
					{#if submitting}
						{t('common.submitting', 'Submitting…')}
					{:else if polling}
						{t('assessment.scoring', 'Scoring…')}
					{:else}
						<CheckCircle2 size={16} /> {form.submit_label || t('assessment.submit', 'Submit')}
					{/if}
				</button>
			</form>
		</div>
	{/if}

	<!-- Score history chart — Phase 33.02 -->
	{#if authed && historySeries.length > 0}
		<div class="border-t border-[var(--color-border)] px-4 py-4">
			<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<TrendingUp size={16} />
				{t('assessment.history', 'Score history')}
			</h2>
			<div class="h-48">
				<LineChart
					data={historySeries}
					x="date"
					y="score"
					props={{
						line: { class: 'stroke-[var(--color-primary)]' },
						area: { class: 'fill-[var(--color-primary)]/10' }
					}}
				/>
			</div>
			<p class="mt-2 text-xs text-[var(--color-muted)]">
				{t('assessment.history_count', '{n} past submissions').replace('{n}', String(historySeries.length))}
			</p>
		</div>
	{/if}
</div>
