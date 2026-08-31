<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { untrack } from 'svelte';
	import { Gauge, Lock, CheckCircle2, ArrowRight, TrendingUp, X } from 'lucide-svelte';
	import { LineChart } from 'layerchart';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { AssessmentForm, AssessmentResult, AssessmentField } from '$lib/types';
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

	// Local history so we can prepend new results after a submit without
	// waiting for a full page reload.
	let history = $state<AssessmentHistoryEntry[]>(untrack(() => data.history ?? []));
	let answers = $state<Record<string, string | string[] | number>>({});
	let currentStep = $state(0);
	let submitting = $state(false);
	let submitError = $state<string | null>(null);
	let result = $state<AssessmentResult | null>(null);
	let loadingResult = $state(false);
	let showRegisterPrompt = $state(false);
	let pollTimer: ReturnType<typeof setTimeout> | null = null;

	// Load a shared/past result from the URL ?result= query param.
	$effect(() => {
		const resultId = page.url.searchParams.get('result');
		if (resultId && !result && !loadingResult) {
			loadingResult = true;
			api.get<AssessmentResult>(`/assessment/result/${resultId}`).then((res) => {
				loadingResult = false;
				if (!res.error && res.data) result = res.data;
			});
		}
	});

	const fields = $derived(form?.fields ?? []);
	const requiredFields = $derived(fields.filter((f) => f.is_required));

	/**
	 * Group the flat field list by dimension for the multi-step UX. Dimensions
	 * are ordered by the position of their first field. Fields without a
	 * dimension fall into a final "general" step.
	 */
	const steps = $derived.by(() => {
		if (!form) return [] as { name: string; fields: AssessmentField[] }[];
		const order: string[] = [];
		const map = new Map<string, AssessmentField[]>();
		for (const f of form.fields) {
			const dim = f.config?.dimension || 'general';
			if (!map.has(dim)) {
				map.set(dim, []);
				order.push(dim);
			}
			map.get(dim)!.push(f);
		}
		return order.map((name) => ({ name, fields: map.get(name)! }));
	});

	const totalSteps = $derived(steps.length);
	const currentStepData = $derived(steps[currentStep] ?? null);

	function isFieldAnswered(f: AssessmentField): boolean {
		const a = answers[f.key];
		if (a === undefined || a === null) return false;
		if (Array.isArray(a)) return a.length > 0;
		if (typeof a === 'string') return a.trim().length > 0;
		return true;
	}

	function stepComplete(): boolean {
		if (!currentStepData) return false;
		return currentStepData.fields.every((f) => !f.is_required || isFieldAnswered(f));
	}

	const requiredAnsweredCount = $derived(
		requiredFields.filter((f) => isFieldAnswered(f)).length
	);
	const progressPct = $derived(
		requiredFields.length > 0
			? Math.round((requiredAnsweredCount / requiredFields.length) * 100)
			: 0
	);

	function setAnswer(key: string, value: string | string[] | number) {
		answers = { ...answers, [key]: value };
	}

	function buildFieldValues(): Record<string, unknown> {
		const out: Record<string, unknown> = {};
		for (const f of form?.fields ?? []) {
			const a = answers[f.key];
			if (a !== undefined && a !== null) out[f.key] = a;
		}
		return out;
	}

	function nextStep() {
		if (currentStep < totalSteps - 1) currentStep++;
	}

	function prevStep() {
		if (currentStep > 0) currentStep--;
	}

	function resetResult() {
		result = null;
		answers = {};
		currentStep = 0;
		showRegisterPrompt = false;
		const url = new URL(page.url);
		url.searchParams.delete('result');
		replaceState(url, {});
	}

	async function submit() {
		if (!form) return;
		submitting = true;
		submitError = null;
		result = null;
		const res = await api.post<{ submission_id: string; status: string }>('/assessment/submit', {
			field_values: buildFieldValues()
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
		submitting = false;
		try {
			result = await pollForResult(submissionId);
		} catch (e) {
			submitError =
				e instanceof Error
					? e.message
					: t('assessment.scoring_timeout', 'Scoring is taking too long. Please try again.');
		}
	}

	async function pollForResult(submissionId: string): Promise<AssessmentResult> {
		const maxAttempts = 30;
		const delayMs = 1000;
		for (let attempt = 0; attempt < maxAttempts; attempt++) {
			await new Promise<void>((resolve) => {
				pollTimer = setTimeout(resolve, delayMs);
			});
			const res = await api.get<AssessmentResult | { status: string }>(
				`/assessment/result/by-submission/${submissionId}`
			);
			if (res.error) throw new Error(res.error.message);
			const payload = res.data as AssessmentResult | { status: string };
			if (payload && typeof payload === 'object' && 'total_score' in payload) {
				return payload as AssessmentResult;
			}
		}
		throw new Error(t('assessment.scoring_timeout', 'Scoring is taking too long. Please try again.'));
	}

	// Clear the polling timer on destroy so a mid-poll navigation does not
	// fire a pending setTimeout on an unmounted component (§4.3).
	$effect(() => {
		return () => {
			if (pollTimer) {
				clearTimeout(pollTimer);
				pollTimer = null;
			}
		};
	});

	// After a result is scored, persist the result ID in the URL and update
	// local state: authenticated users get the new entry prepended to history;
	// anonymous users are shown a registration prompt (§71).
	$effect(() => {
		if (!result) return;
		const url = new URL(page.url);
		url.searchParams.set('result', result.id);
		replaceState(url, {});

		if (authed) {
			const exists = history.some((h) => h.id === result!.id);
			if (!exists) {
				const newItem: AssessmentHistoryEntry = {
					id: result.id,
					total_score: result.total_score,
					threshold_label: result.threshold_label,
					submission_id: result.submission_id,
					created_at: result.created_at
				};
				history = [newItem, ...history];
			}
		} else {
			showRegisterPrompt = true;
		}
	});

	async function viewHistoryResult(item: AssessmentHistoryEntry) {
		loadingResult = true;
		const res = await api.get<AssessmentResult>(`/assessment/result/${item.id}`);
		loadingResult = false;
		if (!res.error && res.data) {
			result = res.data;
		}
	}

	function resolveThresholdColor(color?: string | null): string {
		if (!color) return 'var(--color-primary)';
		if (color.startsWith('#')) return color;
		if (color.startsWith('chart-')) return `var(--${color})`;
		return color;
	}

	const resultThreshold = $derived.by(() => {
		if (!result || !form) return null;
		const label = result.threshold_label;
		return form.scoring_config.thresholds.find((th) => th.label === label) ?? null;
	});

	const gaugeColor = $derived.by(() => {
		return resolveThresholdColor(resultThreshold?.color);
	});

	const scorePct = $derived(
		result && form
			? Math.round((result.total_score / Math.max(form.scoring_config.max_score, 1)) * 100)
			: 0
	);

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

	const nextReturnPath = $derived(
		encodeURIComponent(page.url.pathname + (page.url.searchParams.toString() ? `?${page.url.searchParams.toString()}` : ''))
	);
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
							<circle
								cx="40" cy="40" r="34"
								fill="none"
								stroke="var(--color-surface-subtle)"
								stroke-width="6"
							/>
							<circle
								cx="40" cy="40" r="34"
								fill="none"
								stroke={gaugeColor}
								stroke-width="6"
								stroke-dasharray={2 * Math.PI * 34}
								stroke-dashoffset={2 * Math.PI * 34 * (1 - scorePct / 100)}
								stroke-linecap="round"
								class="transition-all duration-700"
							/>
						</svg>
						<span class="absolute text-lg font-semibold text-[var(--color-text)]">{scorePct}%</span>
					</div>
					<div class="min-w-0 flex-1">
						<span class="block text-sm font-medium" style="color: {gaugeColor};">
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
										style="width: {Math.min(100, Math.round((score / Math.max(maxScore, 1)) * 100))}%"
									></div>
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/if}

			<!-- Roadmap -->
			{#if result.roadmap.length > 0}
				<div class="mb-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h3 class="mb-3 text-sm font-medium text-[var(--color-text)]">
						{t('assessment.roadmap', 'Recommended next steps')}
					</h3>
					<ol class="space-y-3">
						{#each result.roadmap as step, i (i)}
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

			<!-- Anonymous registration prompt -->
			{#if !authed && showRegisterPrompt}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<div class="flex items-start justify-between gap-3">
						<div>
							<h3 class="text-sm font-medium text-[var(--color-text)]">
								{t('assessment.save_results_title', 'Save your results')}
							</h3>
							<p class="mt-1 text-xs text-[var(--color-muted)]">
								{t('assessment.save_results_desc', 'Create a free account to save your results, track your progress over time, and get your personalized roadmap.')}
							</p>
							<div class="mt-3 flex items-center gap-2">
								<a
									href="/signup?next={nextReturnPath}"
									class="inline-flex items-center gap-1 rounded-lg bg-[var(--color-primary)] px-3 py-2 text-xs font-medium text-white hover:opacity-90"
								>
									{t('assessment.create_account', 'Create free account')}
								</a>
								<a
									href="/login?next={nextReturnPath}"
									class="inline-flex items-center gap-1 rounded-lg border border-[var(--color-border)] px-3 py-2 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
								>
									{t('assessment.sign_in', 'Sign in')}
								</a>
							</div>
						</div>
						<button
							type="button"
							onclick={() => (showRegisterPrompt = false)}
							class="text-[var(--color-muted)] hover:text-[var(--color-text)]"
							aria-label={t('common.close', 'Close')}
						>
							<X size={16} />
						</button>
					</div>
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
					<span>{requiredAnsweredCount}/{requiredFields.length}</span>
				</div>
				<div class="mt-1 h-2 overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
					<div class="h-full bg-[var(--color-primary)] transition-all" style="width: {progressPct}%"></div>
				</div>
			</div>

			{#if totalSteps > 1}
				<div class="mb-4 flex items-center gap-1">
					{#each steps as _, i (i)}
						<div
							class="h-1.5 flex-1 rounded-full transition-colors"
							style="background: {i <= currentStep ? 'var(--color-primary)' : 'var(--color-surface-subtle)'}"
						></div>
					{/each}
				</div>
				<p class="mb-4 text-xs text-[var(--color-muted)]">
					{t('assessment.section_progress', 'Section {current} of {total}').replace('{current}', String(currentStep + 1)).replace('{total}', String(totalSteps))}
					<span class="ml-2 capitalize">{currentStepData?.name}</span>
				</p>
			{/if}

			{#if currentStepData}
				<div class="space-y-4">
					{#each currentStepData.fields as field (field.key)}
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
												onchange={() => setAnswer(field.key, opt.value)}
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
													setAnswer(field.key, next);
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
										oninput={(e) => setAnswer(field.key, parseInt(e.currentTarget.value))}
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
									value={(answers[field.key] as string) ?? ''}
									oninput={(e) => setAnswer(field.key, (e.target as HTMLInputElement).value)}
									class="mt-2 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
								/>
							{/if}
						</div>
					{/each}
				</div>
			{/if}

			{#if submitError}
				<p class="mt-4 text-sm text-[var(--color-danger)]">{submitError}</p>
			{/if}

			<div class="mt-6 flex items-center justify-between">
				<button
					type="button"
					onclick={prevStep}
					disabled={currentStep === 0}
					class="inline-flex items-center gap-1.5 rounded-lg px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
				>
					{t('assessment.previous', 'Previous')}
				</button>

				{#if currentStep < totalSteps - 1}
					<button
						type="button"
						onclick={nextStep}
						disabled={!stepComplete()}
						class="inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
					>
						{t('assessment.next', 'Next')}
						<ArrowRight size={16} />
					</button>
				{:else}
					<button
						type="button"
						onclick={submit}
						disabled={submitting || !stepComplete()}
						class="flex items-center justify-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
					>
						{#if submitting}
							{t('common.submitting', 'Submitting…')}
						{:else}
							<CheckCircle2 size={16} /> {form.submit_label || t('assessment.submit', 'Submit')}
						{/if}
					</button>
				{/if}
			</div>
		</div>
	{/if}

	<!-- Score history chart -->
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
			<ul class="mt-3 space-y-1">
				{#each history as item (item.id)}
					<li>
						<button
							type="button"
							onclick={() => viewHistoryResult(item)}
							disabled={loadingResult}
							class="w-full rounded-[var(--radius)] px-2 py-1.5 text-left text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
						>
							<span class="font-medium">{new Date(item.created_at).toLocaleDateString()}</span>
							<span class="ml-2 text-[var(--color-muted)]">
								{Math.round((item.total_score / Math.max(maxScore, 1)) * 100)}%
								{#if item.threshold_label}
									— {item.threshold_label}
								{/if}
							</span>
						</button>
					</li>
				{/each}
			</ul>
		</div>
	{/if}
</div>
