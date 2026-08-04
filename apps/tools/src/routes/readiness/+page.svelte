<script lang="ts">
	import { untrack } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import type {
		AssessmentForm,
		AssessmentResult,
		AssessmentSubmission,
		AssessmentQuestion
	} from '$lib/types';

	/**
	 * Japa Readiness Assessment tool — §71, ADR-071.
	 * Scored questionnaire → server-side scoring (Asynq) → results page with
	 * score gauge, dimension breakdown, threshold label, and roadmap.
	 * Anonymous users can take the assessment but must sign in to save results.
	 */
	let { data } = $props();

	let form = $state<AssessmentForm | null>(untrack(() => data.form));
	let result = $state<AssessmentResult | null>(untrack(() => data.result));
	let history = $state<AssessmentResult[]>(untrack(() => data.history));
	let user = $state(untrack(() => data.user));

	// Form state
	let answers = $state<Record<string, string[] | string | number>>({});
	let currentSection = $state(0);
	let submitting = $state(false);
	let error = $state<string | null>(null);
	let showRegisterPrompt = $state(false);

	const totalSections = $derived(form?.sections.length ?? 0);
	const currentSectionData = $derived(form?.sections[currentSection] ?? null);

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

	function setAnswer(questionId: string, value: string[] | string | number) {
		answers = { ...answers, [questionId]: value };
	}

	function isQuestionAnswered(q: AssessmentQuestion): boolean {
		const a = answers[q.id];
		if (a === undefined || a === null) return false;
		if (Array.isArray(a)) return a.length > 0;
		if (typeof a === 'string') return a.trim().length > 0;
		return a !== undefined;
	}

	function sectionComplete(): boolean {
		if (!currentSectionData) return false;
		return currentSectionData.questions.every(
			(q) => !q.required || isQuestionAnswered(q)
		);
	}

	function nextSection() {
		if (currentSection < totalSections - 1) currentSection++;
	}

	function prevSection() {
		if (currentSection > 0) currentSection--;
	}

	async function submitAssessment() {
		if (!form) return;
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		submitting = true;
		error = null;
		const submissions: AssessmentSubmission[] = Object.entries(answers).map(
			([question_id, answer]) => ({ question_id, answer })
		);
		const res = await api.post<{ result_id: string }>('/assessment/submit', {
			form_id: form.id,
			answers: submissions
		});
		submitting = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		if (res.data) {
			// Fetch the result
			const resultRes = await api.get<AssessmentResult>(
				`/assessment/result/${res.data.result_id}`
			);
			if (!resultRes.error && resultRes.data) {
				result = resultRes.data;
				// Prepend the new result to the history list (§71 persistent history).
				history = [resultRes.data, ...history];
				const url = new URL(page.url);
				url.searchParams.set('result', resultRes.data.id);
				replaceState(url, {});
			}
		}
	}

	/** Load a past result from history into the results view (§71). */
	function viewHistoryResult(pastResult: AssessmentResult) {
		result = pastResult;
		const url = new URL(page.url);
		url.searchParams.set('result', pastResult.id);
		replaceState(url, {});
	}

	// Score gauge color based on threshold
	const gaugeColor = $derived.by(() => {
		if (!result) return 'var(--color-muted)';
		const pct = result.total_score;
		if (pct >= 75) return 'var(--chart-3)';
		if (pct >= 50) return 'var(--chart-4)';
		return 'var(--chart-5)';
	});

	const gaugeCircumference = 2 * Math.PI * 52;
	const gaugeOffset = $derived(
		result ? gaugeCircumference - (result.total_score / 100) * gaugeCircumference : gaugeCircumference
	);
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

	{#if !form && !result}
		<div class="space-y-3">
			<LoadingSkeleton height="120px" />
			<LoadingSkeleton height="200px" />
		</div>
	{:else if result}
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
									{#if step.links?.length}
										<div class="mt-2 flex flex-wrap gap-2">
											{#each step.links as link (link.url)}
												<a
													href={link.url}
													class="text-xs text-primary hover:text-primary-hover underline"
												>
													{link.label}
												</a>
											{/each}
										</div>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/if}

			<div class="flex gap-3">
				<button
					onclick={() => { result = null; answers = {}; currentSection = 0; }}
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

			{#if history.length > 1}
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

			<!-- Section progress -->
			<div class="flex items-center gap-2">
				{#each form.sections as _, i (i)}
					<div
						class="flex-1 h-1.5 rounded-full transition-colors {i <= currentSection ? 'bg-[var(--tool-accent)]' : 'bg-surface-subtle'}"
					></div>
				{/each}
			</div>
			<p class="text-sm text-muted">
				{t('readiness.section_progress', 'Section {current} of {total}').replace('{current}', String(currentSection + 1)).replace('{total}', String(totalSections))}
			</p>

			{#if currentSectionData}
				<div>
					<h2 class="text-xl font-semibold text-text mb-1">{currentSectionData.title}</h2>
					{#if currentSectionData.description}
						<p class="text-sm text-text-subtle mb-4">{currentSectionData.description}</p>
					{/if}

					<div class="space-y-6">
						{#each currentSectionData.questions as q (q.id)}
							<div>
								<p class="block text-sm font-medium text-text mb-2">
									{q.prompt}
									{#if q.required}<span class="text-red-500">*</span>{/if}
								</p>

								{#if q.type === 'single' && q.options}
									<div class="space-y-2">
										{#each q.options as opt (opt.id)}
											<label
												class="flex items-center gap-3 p-3 border border-border rounded-lg cursor-pointer hover:border-[var(--tool-accent)] transition-colors"
											>
												<input
													type="radio"
													name={q.id}
													value={opt.id}
													checked={answers[q.id] === opt.id}
													onchange={() => setAnswer(q.id, opt.id)}
													class="accent-[var(--tool-accent)]"
												/>
												<span class="text-sm text-text">{opt.label}</span>
											</label>
										{/each}
									</div>
								{:else if q.type === 'multiple' && q.options}
									<div class="space-y-2">
										{#each q.options as opt (opt.id)}
											<label
												class="flex items-center gap-3 p-3 border border-border rounded-lg cursor-pointer hover:border-[var(--tool-accent)] transition-colors"
											>
												<input
													type="checkbox"
													value={opt.id}
													checked={Array.isArray(answers[q.id]) && (answers[q.id] as string[]).includes(opt.id)}
													onchange={(e) => {
														const current = Array.isArray(answers[q.id]) ? answers[q.id] as string[] : [];
														const next = e.currentTarget.checked
															? [...current, opt.id]
															: current.filter((v) => v !== opt.id);
														setAnswer(q.id, next);
													}}
													class="accent-[var(--tool-accent)]"
												/>
												<span class="text-sm text-text">{opt.label}</span>
											</label>
										{/each}
									</div>
								{:else if q.type === 'scale'}
									<div class="flex items-center gap-3">
										<input
											type="range"
											min={q.scale_min ?? 1}
											max={q.scale_max ?? 10}
											value={(answers[q.id] as number) ?? q.scale_min ?? 1}
											oninput={(e) => setAnswer(q.id, parseInt(e.currentTarget.value))}
											class="flex-1 accent-[var(--tool-accent)]"
										/>
										<span class="text-sm font-medium text-text w-8 text-center">
											{(answers[q.id] as number) ?? q.scale_min ?? 1}
										</span>
									</div>
								{:else}
									<input
										type="text"
										value={(answers[q.id] as string) ?? ''}
										oninput={(e) => setAnswer(q.id, e.currentTarget.value)}
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
					onclick={prevSection}
					disabled={currentSection === 0}
					class="inline-flex items-center gap-1.5 px-4 py-2.5 text-sm font-medium text-text hover:bg-surface-subtle rounded-lg transition-colors disabled:opacity-50"
				>
					<Icon name="arrow-left" size={16} />
					{t('readiness.previous', 'Previous')}
				</button>

				{#if currentSection < totalSections - 1}
					<button
						onclick={nextSection}
						disabled={!sectionComplete()}
						class="inline-flex items-center gap-1.5 px-4 py-2.5 bg-primary text-primary-contrast rounded-lg text-sm font-medium hover:bg-primary-hover transition-colors disabled:opacity-50"
					>
						{t('readiness.next', 'Next')}
						<Icon name="arrow-right" size={16} />
					</button>
				{:else}
					<button
						onclick={submitAssessment}
						disabled={submitting || !sectionComplete()}
						class="inline-flex items-center gap-2 px-4 py-2.5 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50"
					>
						{#if submitting}
							<Icon name="loader-2" size={16} class="animate-spin" />
							{t('readiness.scoring', 'Scoring...')}
						{:else}
							<Icon name="check" size={16} />
							{t('readiness.submit', 'Submit & get score')}
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
