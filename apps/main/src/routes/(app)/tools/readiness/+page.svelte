<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Gauge, Lock, CheckCircle2, ArrowRight } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type {
		AssessmentForm,
		AssessmentResult
	} from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tools.readiness', 'Readiness Assessment — JapaConnect'));
	const description = $derived(
		t('tools.readiness_desc', 'Score your japa readiness across key dimensions.')
	);

	const form = $state<AssessmentForm | null>(data.form);
	const disabled = $state(data.disabled);
	const authed = $state(data.authed);

	// Form state.
	let answers = $state<Record<string, string>>({});
	let submitting = $state(false);
	let submitError = $state<string | null>(null);
	let result = $state<AssessmentResult | null>(null);

	const fields = $derived(form?.fields ?? []);
	const answeredCount = $derived(Object.keys(answers).filter((k) => answers[k]).length);
	const progressPct = $derived(fields.length > 0 ? Math.round((answeredCount / fields.length) * 100) : 0);

	const scorePct = $derived(
		result && form ? Math.round((result.total_score / Math.max(form.scoring_config.max_score, 1)) * 100) : 0
	);

	async function submit() {
		if (!authed) return;
		submitting = true;
		submitError = null;
		const res = await api.post<AssessmentResult>('/assessment/submit', { answers });
		submitting = false;
		if (res.error) {
			submitError = res.error.message;
			return;
		}
		result = res.data;
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
										<a href="/blog/{step.content_slug}" class="mt-1 inline-flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline">
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
					{t('assessment.anon_hint', 'Sign in to submit your answers and save your score.')}
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
				{#each fields as field (field.id)}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
						<label class="block text-sm font-medium text-[var(--color-text)]" for={field.id}>
							{field.label}
							{#if field.is_required}<span class="text-[var(--color-primary)]">*</span>{/if}
						</label>
						{#if field.field_type === 'select' || field.field_type === 'radio'}
							<div class="mt-2 space-y-1">
								{#each Object.keys(field.config.option_scores) as opt (opt)}
									<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
										<input
											type="radio"
											name={field.id}
											value={opt}
											checked={answers[field.id] === opt}
											onchange={() => (answers = { ...answers, [field.id]: opt })}
											class="accent-[var(--color-primary)]"
										/>
										{opt}
									</label>
								{/each}
							</div>
						{:else}
							<input
								id={field.id}
								type="text"
								value={answers[field.id] ?? ''}
								oninput={(e) => (answers = { ...answers, [field.id]: (e.target as HTMLInputElement).value })}
								class="mt-2 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
							/>
						{/if}
					</div>
				{/each}

				{#if submitError}
					<p class="text-sm text-red-500">{submitError}</p>
				{/if}

				<button
					type="submit"
					disabled={!authed || submitting || answeredCount < fields.length}
					class="flex w-full items-center justify-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
				>
					{#if submitting}
						{t('common.submitting', 'Submitting…')}
					{:else}
						<CheckCircle2 size={16} /> {form.submit_label || t('assessment.submit', 'Submit')}
					{/if}
				</button>
			</form>
		</div>
	{/if}
</div>
