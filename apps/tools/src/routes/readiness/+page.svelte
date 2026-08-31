<script lang="ts">
	import { untrack } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import type {
		AssessmentForm,
		AssessmentResult,
		AssessmentHistoryItem,
		AssessmentField,
		AssessmentSubmitResponse,
		AssessmentResultPending
	} from '$lib/types';

	/**
	 * Japa Readiness Assessment tool — §71, ADR-071.
	 * Scored questionnaire built on the §06 Form Builder: a flat list of
	 * custom_fields, each carrying a dimension + option_scores. Scoring runs
	 * server-side in an Asynq task; submit returns a submission_id and the
	 * client polls /result/by-submission/:id until the result is written.
	 * Anonymous users can take the assessment (§71); authenticated users get
	 * persistent history.
	 */
	let { data } = $props();

	let form = $state<AssessmentForm | null>(untrack(() => data.form));
	let result = $state<AssessmentResult | null>(untrack(() => data.result));
	let history = $state<AssessmentHistoryItem[]>(untrack(() => data.history));
	let user = $state(untrack(() => data.user));

	// Form state — answers keyed by field.key (matches field_values contract).
	let answers = $state<Record<string, string | string[] | number>>({});
	let currentStep = $state(0);
	let submitting = $state(false);
	let polling = $state(false);
	let error = $state<string | null>(null);
	let showRegisterPrompt = $state(false);

	// Polling handle — cleared on destroy to avoid a leak (§4.3).
	let pollTimer: ReturnType<typeof setTimeout> | null = null;

	/**
	 * Group the flat field list by dimension for the multi-step UX. Dimensions
	 * are ordered by the position of their first field. Fields without a
	 * dimension fall into a final "General" step.
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

	const seo = $derived(
		buildSeoMeta({
			title: t('readiness.seo_title', 'Japa Readiness Assessment — JapaConnect Tools'),
			description: t(
				'readiness.seo_description',
				'A scored questionnaire across financial, logistical, and personal dimensions. Get a readiness score, threshold label, and a personalized roadmap.'
			),
			path: '/readiness',
			origin: page.url.origin
		})
	);

	function setAnswer(key: string, value: string | string[] | number) {
		answers = { ...answers, [key]: value };
	}

	function isFieldAnswered(f: AssessmentField): boolean {
		const a = answers[f.key];
		if (a === undefined || a === null) return false;
		if (Array.isArray(a)) return a.length > 0;
		if (typeof a === 'string') return a.trim().length > 0;
		return a !== undefined;
	}

	function stepComplete(): boolean {
		if (!currentStepData) return false;
		return currentStepData.fields.every((f) => !f.is_required || isFieldAnswered(f));
	}

	function nextStep() {
		if (currentStep < totalSteps - 1) currentStep++;
	}

	function prevStep() {
		if (currentStep > 0) currentStep--;
	}

	/** Build the field_values map the backend scoring engine expects. */
	function buildFieldValues(): Record<string, unknown> {
		const out: Record<string, unknown> = {};
		for (const f of form?.fields ?? []) {
			const a = answers[f.key];
			if (a !== undefined && a !== null) out[f.key] = a;
		}
		return out;
	}

	async function submitAssessment() {
		if (!form) return;
		submitting = true;
		polling = false;
		error = null;
		const res = await api.post<AssessmentSubmitResponse>('/assessment/submit', {
			field_values: buildFieldValues()
		});
		submitting = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		if (!res.data?.submission_id) {
			error = t('readiness.error_submit', 'Submission failed. Please try again.');
			return;
		}
		// Scoring is async (Asynq) — poll until the result is written.
		await pollForResult(res.data.submission_id);
	}

	/**
	 * Poll GET /assessment/result/by-submission/:id until the worker writes the
	 * result (200) or the retry budget is exhausted. 202 = still pending.
	 */
	async function pollForResult(submissionId: string) {
		polling = true;
		const maxAttempts = 30; // ~45s at 1.5s intervals
		const interval = 1500;
		for (let attempt = 0; attempt < maxAttempts; attempt++) {
			const res = await api.get<AssessmentResult | AssessmentResultPending>(
				`/assessment/result/by-submission/${submissionId}`
			);
			if (res.error) {
				polling = false;
				error = res.error.message;
				return;
			}
			const d = res.data as AssessmentResult | AssessmentResultPending | null;
			if (d && (d as AssessmentResult).id) {
				// Scored result arrived.
				result = d as AssessmentResult;
				polling = false;
				const url = new URL(page.url);
				url.searchParams.set('result', (d as AssessmentResult).id);
				replaceState(url, {});
				// Authenticated users get persistent history; prepend the new
				// summary locally so the list updates without a refetch.
				if (user) {
					history = [
						{
							id: (d as AssessmentResult).id,
							total_score: (d as AssessmentResult).total_score,
							threshold_label: (d as AssessmentResult).threshold_label,
							submission_id: (d as AssessmentResult).submission_id,
							created_at: (d as AssessmentResult).created_at
						},
						...history
					];
				} else {
					// Anonymous user — offer registration to save results (§71).
					showRegisterPrompt = true;
				}
				return;
			}
			// Still pending — wait and retry. Store the timer so it can be
			// cleared on destroy.
			await new Promise<void>((resolve) => {
				pollTimer = setTimeout(resolve, interval);
			});
		}
		polling = false;
		error = t(
			'readiness.scoring_timeout',
			'Scoring is taking longer than expected. Please check back shortly.'
		);
	}

	/** Load a past result from history — fetch the full result by id. */
	async function viewHistoryResult(item: AssessmentHistoryItem) {
		error = null;
		const res = await api.get<AssessmentResult>(`/assessment/result/${item.id}`);
		if (res.error) {
			error = res.error.message;
			return;
		}
		if (res.data) {
			result = res.data;
			const url = new URL(page.url);
			url.searchParams.set('result', res.data.id);
			replaceState(url, {});
		}
	}

	function retake() {
		result = null;
		answers = {};
		currentStep = 0;
		error = null;
		const url = new URL(page.url);
		url.searchParams.delete('result');
		replaceState(url, {});
	}

	$effect(() => {
		// Cleanup polling timer on destroy (§4.3 — no polling leaks).
		return () => {
			if (pollTimer) {
				clearTimeout(pollTimer);
				pollTimer = null;
			}
		};
	});

	function resolveThresholdColor(color?: string | null): string {
		if (!color) return 'var(--tool-accent)';
		if (color.startsWith('#')) return color;
		if (color.startsWith('chart-')) return `var(--${color})`;
		return color;
	}

	const resultThreshold = $derived.by(() => {
		if (!result || !form) return null;
		const label = result.threshold_label;
		return (form.thresholds ?? form.scoring_config?.thresholds ?? []).find(
			(th) => th.label === label
		) ?? null;
	});

	// Score gauge color is driven by the matching threshold's configured color.
	const gaugeColor = $derived.by(() => {
		if (!result) return 'var(--color-muted)';
		return resolveThresholdColor(resultThreshold?.color);
	});

	const gaugeCircumference = 2 * Math.PI * 52;
	const gaugeOffset = $derived(
		result
			? gaugeCircumference - (result.total_score / 100) * gaugeCircumference
			: gaugeCircumference
	);

	/** Resolve a roadmap step's content_slug into a platform link. */
	function stepHref(slug?: string): string | null {
		if (!slug) return null;
		// Content slugs are relative to the main app origin; link absolutely.
		if (slug.startsWith('http')) return slug;
		return `/${slug}`;
	}
</script>

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	<meta property="og:url" content={seo.canonical} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={seo.title} />
	<meta name="twitter:description" content={seo.description} />
</svelte:head>

<div class="mx-auto max-w-3xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
	<div class="mb-8">
		<h1 class="text-2xl sm:text-3xl font-bold text-text">{t('readiness.title', 'Japa Readiness Assessment')}</h1>
		<p class="mt-2 text-text-subtle">
			{t('readiness.subtitle', 'Answer honestly to get an accurate readiness score and personalized roadmap.')}
		</p>
	</div>

	{#if result}
		<!-- Results view -->
		<div class="space-y-8">
			<div class="text-center">
				<div class="relative inline-flex items-center justify-center w-32 h-32">
					<svg class="w-32 h-32 -rotate-90" viewBox="0 0 120 120">
						<circle
							cx="60"
							cy="60"
							r="52"
							fill="none"
							stroke="var(--color-surface-subtle)"
							stroke-width="8"
						/>
						<circle
							cx="60"
							cy="60"
							r="52"
							fill="none"
							stroke={gaugeColor}
							stroke-width="8"
							stroke-linecap="round"
							stroke-dasharray={gaugeCircumference}
							stroke-dashoffset={gaugeOffset}
							class="transition-all duration-1000"
						/>
					</svg>
					<div class="absolute inset-0 flex flex-col items-center justify-center">
						<span class="text-3xl font-bold text-text">{result.total_score}</span>
						<span class="text-xs text-muted">{t('readiness.score_label', '/ 100')}</span>
					</div>
				</div>
				<h2 class="mt-4 text-xl font-semibold" style="color: {gaugeColor};">
					{result.threshold_label}
				</h2>
			</div>

			{#if Object.keys(result.dimension_scores).length > 0}
				<div>
					<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
						{t('readiness.dimension_breakdown', 'Dimension breakdown')}
					</h3>
					<div class="space-y-3">
						{#each Object.entries(result.dimension_scores) as [dim, score] (dim)}
							<div>
								<div class="flex items-center justify-between text-sm mb-1">
									<span class="font-medium text-text capitalize">{dim}</span>
									<span class="text-muted">{score}</span>
								</div>
								<div class="w-full h-2 bg-surface-subtle rounded-full overflow-hidden">
									<div
										class="h-full bg-[var(--tool-accent)] transition-all duration-500"
										style="width: {score}%;"
									></div>
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/if}

			{#if result.roadmap.length > 0}
				<div>
					<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
						{t('readiness.roadmap', 'Your personalized roadmap')}
					</h3>
					<div class="space-y-3">
						{#each result.roadmap as step, i (i)}
							<div class="flex gap-3 p-4 bg-surface border border-border rounded-lg">
								<div
									class="flex-shrink-0 w-7 h-7 rounded-full bg-[var(--tool-accent)] text-primary-contrast flex items-center justify-center text-sm font-semibold"
								>
									{i + 1}
								</div>
								<div class="flex-1">
									<h4 class="text-sm font-semibold text-text">{step.title}</h4>
									{#if step.description}
										<p class="text-sm text-text-subtle mt-1">{step.description}</p>
									{/if}
									{#if stepHref(step.content_slug)}
										<a
											href={stepHref(step.content_slug)}
											class="mt-2 inline-block text-xs text-primary hover:text-primary-hover underline"
										>
											{t('readiness.read_more', 'Read more')}
										</a>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/if}

			<div class="flex gap-3">
				<button
					onclick={retake}
					class="px-4 py-2.5 bg-primary text-primary-contrast rounded-lg text-sm font-medium hover:bg-primary-hover transition-colors"
				>
					{t('readiness.retake', 'Retake assessment')}
				</button>
				<a
					href="/readiness"
					class="px-4 py-2.5 border border-border rounded-lg text-sm font-medium text-text hover:bg-surface-subtle transition-colors"
				>
					{t('readiness.clear_result', 'Clear result')}
				</a>
			</div>

			{#if user && history.length > 1}
				<!-- Past results history (§71 — persistent history for authenticated users) -->
				<div>
					<h3 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
						{t('readiness.past_results', 'Past results')}
					</h3>
					<div class="space-y-2">
						{#each history as past (past.id)}
							<button
								onclick={() => viewHistoryResult(past)}
								class="w-full flex items-center justify-between p-3 bg-surface border border-border rounded-lg hover:border-[var(--tool-accent)] transition-colors text-left"
							>
								<div>
									<span class="text-sm font-medium text-text">{past.threshold_label}</span>
									<p class="text-xs text-muted mt-0.5">
										{new Date(past.created_at).toLocaleDateString('en-US', {
											year: 'numeric',
											month: 'short',
											day: 'numeric'
										})}
									</p>
								</div>
								<span class="text-lg font-bold text-text">{past.total_score}</span>
							</button>
						{/each}
					</div>
				</div>
			{/if}
		</div>
	{:else if form}
		<!-- Assessment form -->
		<div class="space-y-6">
			{#if form.description}
				<p class="text-text-subtle">{form.description}</p>
			{/if}

			<!-- Step progress -->
			{#if totalSteps > 1}
				<div class="flex items-center gap-2">
					{#each steps as _, i (i)}
						<div
							class="flex-1 h-1.5 rounded-full transition-colors {i <= currentStep ? 'bg-[var(--tool-accent)]' : 'bg-surface-subtle'}"
						></div>
					{/each}
				</div>
				<p class="text-sm text-muted capitalize">
					{t('readiness.section_progress', 'Section {current} of {total}').replace('{current}', String(currentStep + 1)).replace('{total}', String(totalSteps))} — {currentStepData?.name}
				</p>
			{/if}

			{#if currentStepData}
				<div>
					<h2 class="text-xl font-semibold text-text mb-1 capitalize">{currentStepData.name}</h2>

					<div class="space-y-6">
						{#each currentStepData.fields as f (f.key)}
							<div>
								<p class="block text-sm font-medium text-text mb-2">
									{f.label}
									{#if f.is_required}<span class="text-red-500">*</span>{/if}
								</p>

								{#if f.field_type === 'select' && f.options.length > 0}
									<div class="space-y-2">
										{#each f.options as opt (opt.value)}
											<label
												class="flex items-center gap-3 p-3 border border-border rounded-lg cursor-pointer hover:border-[var(--tool-accent)] transition-colors"
											>
												<input
													type="radio"
													name={f.key}
													value={opt.value}
													checked={answers[f.key] === opt.value}
													onchange={() => setAnswer(f.key, opt.value)}
													class="accent-[var(--tool-accent)]"
												/>
												<span class="text-sm text-text">{opt.label}</span>
											</label>
										{/each}
									</div>
								{:else if f.field_type === 'multiselect' && f.options.length > 0}
									<div class="space-y-2">
										{#each f.options as opt (opt.value)}
											<label
												class="flex items-center gap-3 p-3 border border-border rounded-lg cursor-pointer hover:border-[var(--tool-accent)] transition-colors"
											>
												<input
													type="checkbox"
													value={opt.value}
													checked={Array.isArray(answers[f.key]) && (answers[f.key] as string[]).includes(opt.value)}
													onchange={(e) => {
														const current = Array.isArray(answers[f.key]) ? answers[f.key] as string[] : [];
														const next = e.currentTarget.checked
															? [...current, opt.value]
															: current.filter((v) => v !== opt.value);
														setAnswer(f.key, next);
													}}
													class="accent-[var(--tool-accent)]"
												/>
												<span class="text-sm text-text">{opt.label}</span>
											</label>
										{/each}
									</div>
								{:else if f.field_type === 'number' || f.field_type === 'rating'}
									<div class="flex items-center gap-3">
										<input
											type="range"
											min={0}
											max={100}
											value={(answers[f.key] as number) ?? 0}
											oninput={(e) => setAnswer(f.key, parseInt(e.currentTarget.value))}
											class="flex-1 accent-[var(--tool-accent)]"
										/>
										<span class="text-sm font-medium text-text w-8 text-center">
											{(answers[f.key] as number) ?? 0}
										</span>
									</div>
								{:else}
									<input
										type="text"
										value={(answers[f.key] as string) ?? ''}
										placeholder={f.placeholder || ''}
										oninput={(e) => setAnswer(f.key, e.currentTarget.value)}
										class="w-full px-4 py-2.5 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
									/>
								{/if}
							</div>
						{/each}
					</div>
				</div>
			{/if}

			{#if error}
				<p class="text-sm text-red-500">{error}</p>
			{/if}

			<div class="flex items-center justify-between">
				<button
					onclick={prevStep}
					disabled={currentStep === 0}
					class="inline-flex items-center gap-1.5 px-4 py-2.5 text-sm font-medium text-text hover:bg-surface-subtle rounded-lg transition-colors disabled:opacity-50"
				>
					<Icon name="arrow-left" size={16} />
					{t('readiness.previous', 'Previous')}
				</button>

				{#if currentStep < totalSteps - 1}
					<button
						onclick={nextStep}
						disabled={!stepComplete()}
						class="inline-flex items-center gap-1.5 px-4 py-2.5 bg-primary text-primary-contrast rounded-lg text-sm font-medium hover:bg-primary-hover transition-colors disabled:opacity-50"
					>
						{t('readiness.next', 'Next')}
						<Icon name="arrow-right" size={16} />
					</button>
				{:else}
					<button
						onclick={submitAssessment}
						disabled={submitting || polling || !stepComplete()}
						class="inline-flex items-center gap-2 px-4 py-2.5 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50"
					>
						{#if submitting || polling}
							<Icon name="loader-2" size={16} class="animate-spin" />
							{t('readiness.scoring', 'Scoring...')}
						{:else}
							<Icon name="check" size={16} />
							{form.submit_label || t('readiness.submit', 'Submit & get score')}
						{/if}
					</button>
				{/if}
			</div>
		</div>
	{:else}
		<EmptyState
			icon="clipboard-x"
			title={t('readiness.unavailable', 'Assessment unavailable')}
			description={t('readiness.unavailable_desc', 'The readiness assessment is not configured yet. Please check back later.')}
		/>
	{/if}

	{#if showRegisterPrompt}
		<RegistrationPrompt
			open={showRegisterPrompt}
			onClose={() => (showRegisterPrompt = false)}
			icon="user-plus"
			title={t('register.save_results', 'Save your results')}
			description={t('register.save_results_desc', 'Create a free account to save your assessment results, track your progress over time, and get your personalized roadmap.')}
		/>
	{/if}
	{/if}
</div>
