<script lang="ts">
	import { untrack } from 'svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Gauge } from 'lucide-svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { LineChart, Spline, Points } from 'layerchart';
	import type {
		CrsFactorsResponse,
		CrsDraw,
		CrsScoreHistoryItem,
		CrsCalculateResult,
		CrsCalculateInput
	} from '$lib/types';
	import type { PageData } from './$types';

	/**
	 * CRS Calculator tool — §107, ADR-118.
	 * Profile form → POST /crs/calculate → canonical score + factor
	 * breakdown. Cut-off trend chart from SSR-loaded draw history. Score
	 * history is saved automatically when the user is signed in.
	 *
	 * Ref: japaconnect-engineering/sections/107-crs-calculator.md
	 */
	let { data }: { data: PageData } = $props();

	let factors = $state<CrsFactorsResponse | null>(untrack(() => data.factors));
	let latestDraws = $state<CrsDraw[]>(untrack(() => data.latestDraws));
	let drawHistory = $state<CrsDraw[]>(untrack(() => data.drawHistory));
	let scoreHistory = $state<CrsScoreHistoryItem[]>(untrack(() => data.scoreHistory));
	const authed = $derived(data.authed);

	// --- Profile form state ---
	type SkillKey = 'listening' | 'reading' | 'writing' | 'speaking';
	type CLBState = Record<SkillKey, number>;

	let maritalStatus = $state<'single' | 'married'>('single');
	let age = $state(30);
	let education = $state('bachelors');
	let firstLanguage = $state<'english' | 'french'>('english');
	let clbFirst = $state<CLBState>({ listening: 8, reading: 8, writing: 8, speaking: 8 });
	let hasSecondLanguage = $state(false);
	let clbSecond = $state<CLBState>({ listening: 7, reading: 7, writing: 7, speaking: 7 });
	let canadianExp = $state(0);
	let foreignExp = $state(1);
	let hasCertificate = $state(false);
	let provincialNomination = $state(false);
	let jobOffer = $state(false);
	let siblingInCanada = $state(false);
	let canadianEducation = $state('none');

	// Spouse fields (married only). Defaults are neutral — no points are
	// granted until the user actually declares spouse factors.
	let spouseEducation = $state('none');
	let spouseHasTest = $state(false);
	let spouseClb = $state<CLBState>({ listening: 7, reading: 7, writing: 7, speaking: 7 });
	let spouseCanadianExp = $state(0);

	// --- Result state ---
	let result = $state<CrsCalculateResult | null>(null);
	let calculating = $state(false);
	let calcError = $state('');

	const title = $derived(t('crs_calculator.seo_title', 'Express Entry CRS Calculator — {site}'));
	const description = $derived(
		t(
			'crs_calculator.seo_description',
			'Calculate your Comprehensive Ranking System score for Canada Express Entry — factor breakdown, draw cut-offs, and score history.'
		)
	);

	// Dropdown options come from the active ruleset's factor rows — one row
	// per selectable level, with `input_descriptor.edu` as the value the
	// scoring engine matches against. The fallbacks use the same canonical
	// level keys the backend validates (educationRank in service.go).
	function fallbackEducation() {
		return [
			{ value: 'none', label: t('crs_calculator.edu_none', 'No post-secondary credential') },
			{ value: 'secondary', label: t('crs_calculator.edu_secondary', 'Secondary (high school) diploma') },
			{ value: 'one_year_post', label: t('crs_calculator.edu_one_year_post', 'One-year post-secondary program') },
			{ value: 'two_year_post', label: t('crs_calculator.edu_two_year_post', 'Two-year post-secondary program') },
			{ value: 'bachelors', label: t('crs_calculator.edu_bachelors', "Bachelor's degree") },
			{ value: 'two_or_more', label: t('crs_calculator.edu_two_or_more', 'Two or more post-secondary credentials') },
			{ value: 'masters', label: t('crs_calculator.edu_masters', "Master's degree") },
			{ value: 'professional', label: t('crs_calculator.edu_professional', 'Professional degree') },
			{ value: 'phd', label: t('crs_calculator.edu_phd', 'Doctoral degree (PhD)') }
		];
	}

	/** Collect select options for a factor key from the loaded ruleset. */
	function factorOptions(key: string, fallback: { value: string; label: string }[]) {
		const rows = (factors?.factors ?? [])
			.filter((f) => f.key === key)
			.slice()
			.sort((a, b) => a.sort_order - b.sort_order)
			.map((f) => ({
				value:
					typeof (f.input_descriptor as Record<string, unknown>)?.edu === 'string'
						? ((f.input_descriptor as Record<string, unknown>).edu as string)
						: '',
				label: f.label
			}))
			.filter((o) => o.value !== '');
		return rows.length > 0 ? rows : fallback;
	}

	const educationOptions = $derived(factorOptions('education', fallbackEducation()));
	const spouseEducationOptions = $derived(factorOptions('spouse_education', educationOptions));
	const canadianEducationOptions = $derived.by(() => {
		const opts = factorOptions('canadian_education', [
			{ value: 'one_or_two_years', label: t('crs_calculator.edu_canadian_1_2', '1–2 year credential') },
			{ value: 'three_plus_years', label: t('crs_calculator.edu_canadian_3_plus', '3+ year credential') }
		]);
		// 'none' is the always-valid "no Canadian credential" choice.
		return opts.some((o) => o.value === 'none')
			? opts
			: [{ value: 'none', label: t('crs_calculator.canadian_edu_none', 'None') }, ...opts];
	});

	const firstLanguageOptions = $derived([
		{ value: 'english', label: t('crs_calculator.lang_english', 'English') },
		{ value: 'french', label: t('crs_calculator.lang_french', 'French') }
	]);
	const secondLanguageLabel = $derived(
		firstLanguage === 'french'
			? t('crs_calculator.lang_english', 'English')
			: t('crs_calculator.lang_french', 'French')
	);

	const skills: { key: SkillKey; label: string }[] = $derived([
		{ key: 'listening', label: t('crs_calculator.skill_listening', 'Listening') },
		{ key: 'reading', label: t('crs_calculator.skill_reading', 'Reading') },
		{ key: 'writing', label: t('crs_calculator.skill_writing', 'Writing') },
		{ key: 'speaking', label: t('crs_calculator.skill_speaking', 'Speaking') }
	]);

	// Display labels for draw categories (backend codes → readable labels).
	const CATEGORY_LABELS: Record<string, string> = {
		general: 'General',
		cec: 'CEC',
		pnp: 'PNP',
		french: 'French',
		stem: 'STEM',
		health: 'Healthcare',
		trades: 'Trades',
		agriculture: 'Agriculture',
		education: 'Education',
		transport: 'Transport'
	};
	function categoryLabel(category: string): string {
		return t(`crs_calculator.cat_${category}`, CATEGORY_LABELS[category] ?? category);
	}

	// The score comparison targets the most recent draw overall — latestDraws
	// is ordered by category (DISTINCT ON), not by date, so [0] would pick an
	// arbitrary category.
	const latestDraw = $derived.by(() => {
		if (latestDraws.length === 0) return null;
		return latestDraws.reduce((best, d) => (d.draw_date > best.draw_date ? d : best));
	});

	// Cut-off trend chart — one series per selected category (mixing categories
	// on a single line produced a meaningless sawtooth).
	const drawCategories = $derived([...new Set(drawHistory.map((d) => d.category))]);
	let chartCategory = $state('general');
	$effect(() => {
		if (drawCategories.length > 0 && !drawCategories.includes(chartCategory)) {
			chartCategory = drawCategories.includes('general') ? 'general' : drawCategories[0];
		}
	});
	const chartCategoryOptions = $derived(
		drawCategories.map((c) => ({ value: c, label: categoryLabel(c) }))
	);
	const chartData = $derived.by(() => {
		return drawHistory
			.filter((d) => d.category === chartCategory)
			.slice()
			.sort((a, b) => a.draw_date.localeCompare(b.draw_date))
			.map((d) => ({ date: new Date(d.draw_date + 'T00:00:00'), cutoff: d.crs_cutoff }));
	});

	function buildInput(): CrsCalculateInput {
		const input: CrsCalculateInput = {
			marital_status: maritalStatus,
			age,
			education,
			first_language: firstLanguage,
			clb_first: { ...clbFirst },
			canadian_experience_years: canadianExp,
			foreign_experience_years: foreignExp,
			certificate_of_qualification: hasCertificate,
			provincial_nomination: provincialNomination,
			job_offer: jobOffer,
			sibling_in_canada: siblingInCanada,
			canadian_education: canadianEducation
		};
		if (hasSecondLanguage) {
			input.clb_second = { ...clbSecond };
		}
		// The French factor set is whichever declared official-language test is
		// French: the first set when first_language = french, otherwise the
		// second set (the only other official language). Deriving it here keeps
		// a single source of truth per test result — no double entry.
		if (firstLanguage === 'french') {
			input.french_clb = input.clb_first;
		} else if (input.clb_second) {
			input.french_clb = input.clb_second;
		}
		if (maritalStatus === 'married') {
			input.spouse = {
				education: spouseEducation,
				clb_first: spouseHasTest
					? { ...spouseClb }
					: { listening: 0, reading: 0, writing: 0, speaking: 0 },
				canadian_experience_years: spouseCanadianExp
			};
		}
		return input;
	}

	async function calculate(e: Event) {
		e.preventDefault();
		calculating = true;
		calcError = '';
		const res = await api.post<CrsCalculateResult>('/crs/calculate', buildInput());
		calculating = false;
		if (res.error) {
			calcError = res.error.message;
			result = null;
			return;
		}
		result = res.data;
		if (res.data?.saved && authed) {
			// Refresh history so the saved score appears immediately.
			const hist = await api.get<CrsScoreHistoryItem[]>('/me/crs-scores');
			if (!hist.error && hist.data) scoreHistory = hist.data;
		}
	}

	// Group breakdown by factor group for display, in canonical IRCC order.
	const breakdownGroups = $derived.by(() => {
		const ordered: [string, NonNullable<typeof result>['breakdown']][] = [];
		if (!result) return ordered;
		const order = ['core', 'spouse', 'transferability', 'additional'];
		const groups: Record<string, typeof result.breakdown> = {};
		for (const b of result.breakdown) {
			(groups[b.group] ??= []).push(b);
		}
		for (const g of [...order, ...Object.keys(groups).filter((k) => !order.includes(k))]) {
			if (groups[g]) ordered.push([g, groups[g]]);
		}
		return ordered;
	});

	const groupLabels: Record<string, string> = {
		core: 'Core / Human capital',
		spouse: 'Spouse factors',
		transferability: 'Skill transferability',
		additional: 'Additional points'
	};

	const inputCls =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]';
	const labelCls = 'mb-1 block text-sm font-medium text-[var(--color-text)]';
	const subLabelCls = 'mb-1 block text-xs text-[var(--color-muted)]';
	const cardCls = 'rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]';
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-4xl px-4 py-6">
	<!-- Header -->
	<div class="mb-6">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Gauge size={20} /> {t('crs_calculator.title', 'Express Entry CRS Calculator')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">
			{t('crs_calculator.subtitle', 'Calculate your Comprehensive Ranking System score against the current IRCC ruleset.')}
		</p>
		{#if factors?.ruleset}
			<p class="mt-1 text-xs text-[var(--color-muted)]">
				{t('crs_calculator.ruleset', 'Ruleset')}: {factors.ruleset.version_label}
				· {t('crs_calculator.effective', 'Effective')} {factors.ruleset.effective_date}
			</p>
		{/if}
	</div>

	<!-- Latest draw cut-offs -->
	{#if latestDraws.length > 0}
		<div class="mb-6 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
			{#each latestDraws as draw (draw.id)}
				<div class="{cardCls} p-3">
					<div class="text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">{categoryLabel(draw.category)}</div>
					<div class="mt-1 text-xl font-bold tabular-nums text-[var(--color-text)]">{draw.crs_cutoff}</div>
					<div class="mt-0.5 text-xs text-[var(--color-muted)]">
						{draw.draw_date}{#if draw.draw_number} · #{draw.draw_number}{/if} · {draw.invitations_issued.toLocaleString()} {t('crs_calculator.itas', 'ITAs')}
					</div>
				</div>
			{/each}
		</div>
	{/if}

	<!-- Calculator form -->
	<form onsubmit={calculate} class="{cardCls} p-4">
		<h2 class="mb-4 text-base font-semibold text-[var(--color-text)]">{t('crs_calculator.form_title', 'Your profile')}</h2>

		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
			<!-- Marital status -->
			<div>
				<label class={labelCls} for="marital">{t('crs_calculator.marital', 'Marital status')}</label>
				<AutocompleteSelect
					id="marital"
					value={maritalStatus}
					options={[
						{ value: 'single', label: t('crs_calculator.single', 'Single / not accompanying spouse') },
						{ value: 'married', label: t('crs_calculator.married', 'Married (spouse accompanying)') }
					]}
					onValueChange={(v) => (maritalStatus = v as 'single' | 'married')}
				/>
			</div>

			<!-- Age -->
			<div>
				<label class={labelCls} for="age">{t('crs_calculator.age', 'Age')}</label>
				<input id="age" type="number" bind:value={age} min="17" max="120" class={inputCls} />
			</div>

			<!-- Education -->
			<div>
				<label class={labelCls} for="education">{t('crs_calculator.education', 'Highest education')}</label>
				<AutocompleteSelect id="education" bind:value={education} options={educationOptions} />
			</div>

			<!-- Canadian experience -->
			<div>
				<label class={labelCls} for="canadianExp">{t('crs_calculator.canadian_exp', 'Canadian work experience (years)')}</label>
				<input id="canadianExp" type="number" bind:value={canadianExp} min="0" max="60" class={inputCls} />
			</div>

			<!-- Foreign experience -->
			<div>
				<label class={labelCls} for="foreignExp">{t('crs_calculator.foreign_exp', 'Foreign work experience (years)')}</label>
				<input id="foreignExp" type="number" bind:value={foreignExp} min="0" max="60" class={inputCls} />
			</div>

			<!-- Canadian education -->
			<div>
				<label class={labelCls} for="canadianEdu">{t('crs_calculator.canadian_edu', 'Canadian education')}</label>
				<AutocompleteSelect
					id="canadianEdu"
					bind:value={canadianEducation}
					options={canadianEducationOptions}
				/>
			</div>
		</div>

		<!-- First language (CLB) -->
		<fieldset class="mt-5">
			<legend class="mb-2 text-sm font-medium text-[var(--color-text)]">
				{t('crs_calculator.first_language', 'First official language (CLB scores)')}
			</legend>
			<div class="mb-3 sm:max-w-xs">
				<label class={subLabelCls} for="firstLang">{t('crs_calculator.first_lang_select', 'Language')}</label>
				<AutocompleteSelect
					id="firstLang"
					bind:value={firstLanguage}
					options={firstLanguageOptions}
				/>
			</div>
			<div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
				{#each skills as skill (skill.key)}
					<div>
						<label class={subLabelCls} for="clb-{skill.key}">{skill.label}</label>
						<input
							id="clb-{skill.key}"
							type="number"
							min="0"
							max="10"
							class={inputCls}
							bind:value={clbFirst[skill.key]}
						/>
					</div>
				{/each}
			</div>
		</fieldset>

		<!-- Second language toggle — the other official language -->
		<div class="mt-4">
			<label class="inline-flex items-center gap-2 text-sm text-[var(--color-text)]">
				<input type="checkbox" bind:checked={hasSecondLanguage} class="rounded" />
				{t('crs_calculator.second_language', 'I have a second official language result')} ({secondLanguageLabel})
			</label>
		</div>

		{#if hasSecondLanguage}
			<fieldset class="mt-3 rounded-[var(--radius)] border border-[var(--color-border)] p-4">
				<legend class="px-1 text-sm font-medium text-[var(--color-text)]">{secondLanguageLabel} (CLB)</legend>
				<div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
					{#each skills as skill (skill.key)}
						<div>
							<label class={subLabelCls} for="clb2-{skill.key}">{skill.label}</label>
							<input
								id="clb2-{skill.key}"
								type="number"
								min="0"
								max="10"
								class={inputCls}
								bind:value={clbSecond[skill.key]}
							/>
						</div>
					{/each}
				</div>
			</fieldset>
		{/if}

		<!-- Spouse section (married only) -->
		{#if maritalStatus === 'married'}
			<fieldset class="mt-5 rounded-[var(--radius)] border border-[var(--color-border)] p-4">
				<legend class="px-1 text-sm font-medium text-[var(--color-text)]">{t('crs_calculator.spouse', 'Spouse')}</legend>
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
					<div>
						<label class={subLabelCls} for="spouseEdu">{t('crs_calculator.spouse_education', 'Education')}</label>
						<AutocompleteSelect
							id="spouseEdu"
							bind:value={spouseEducation}
							options={spouseEducationOptions}
						/>
					</div>
					<div>
						<label class={subLabelCls} for="spouseExp">{t('crs_calculator.spouse_exp', 'Canadian experience (yrs)')}</label>
						<input id="spouseExp" type="number" bind:value={spouseCanadianExp} min="0" max="60" class={inputCls} />
					</div>
					<div class="sm:col-span-1"></div>
				</div>
				<label class="mt-3 inline-flex items-center gap-2 text-sm text-[var(--color-text)]">
					<input type="checkbox" bind:checked={spouseHasTest} class="rounded" />
					{t('crs_calculator.spouse_lang_test', 'Spouse has official language test results')}
				</label>
				{#if spouseHasTest}
					<div class="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
						{#each skills as skill (skill.key)}
							<div>
								<label class={subLabelCls} for="spouse-clb-{skill.key}">{skill.label} (CLB)</label>
								<input
									id="spouse-clb-{skill.key}"
									type="number"
									min="0"
									max="10"
									class={inputCls}
									bind:value={spouseClb[skill.key]}
								/>
							</div>
						{/each}
					</div>
				{/if}
			</fieldset>
		{/if}

		<!-- Additional factors -->
		<fieldset class="mt-5">
			<legend class="mb-2 text-sm font-medium text-[var(--color-text)]">{t('crs_calculator.additional', 'Additional factors')}</legend>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
				<label class="inline-flex items-center gap-2 text-sm text-[var(--color-text)]">
					<input type="checkbox" bind:checked={provincialNomination} class="rounded" />
					{t('crs_calculator.pnp', 'Provincial nomination (+600)')}
				</label>
				<label class="inline-flex items-center gap-2 text-sm text-[var(--color-text)]">
					<input type="checkbox" bind:checked={jobOffer} class="rounded" />
					{t('crs_calculator.job_offer', 'Valid job offer')}
				</label>
				<label class="inline-flex items-center gap-2 text-sm text-[var(--color-text)]">
					<input type="checkbox" bind:checked={siblingInCanada} class="rounded" />
					{t('crs_calculator.sibling', 'Sibling in Canada (citizen/PR)')}
				</label>
				<label class="inline-flex items-center gap-2 text-sm text-[var(--color-text)]">
					<input type="checkbox" bind:checked={hasCertificate} class="rounded" />
					{t('crs_calculator.certificate', 'Certificate of qualification (trade)')}
				</label>
			</div>
			<p class="mt-2 text-xs text-[var(--color-muted)]">
				{t('crs_calculator.french_hint', 'French test results entered above also count toward francophone bonus points automatically.')}
			</p>
		</fieldset>

		<div class="mt-5">
			<button
				type="submit"
				disabled={calculating}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
			>
				{calculating ? t('crs_calculator.calculating', 'Calculating…') : t('crs_calculator.calculate', 'Calculate my score')}
			</button>
		</div>
	</form>

	<!-- Result -->
	{#if calcError}
		<div class="mt-4 rounded-[var(--radius)] border border-[var(--color-danger)]/30 bg-[var(--color-danger)]/10 p-3 text-sm text-[var(--color-danger)]">
			{calcError}
		</div>
	{/if}

	{#if result}
		<div class="mt-4 {cardCls} p-4">
			<div class="flex items-baseline gap-4">
				<div>
					<div class="text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
						{t('crs_calculator.your_score', 'Your CRS score')}
					</div>
					<div class="mt-1 text-4xl font-bold tabular-nums text-[var(--color-primary)]">{result.score}</div>
				</div>
				{#if latestDraw}
					<div class="text-sm text-[var(--color-muted)]">
						{t('crs_calculator.vs_latest', 'vs latest draw')} ({categoryLabel(latestDraw.category)}):
						<span class="font-semibold {result.score >= latestDraw.crs_cutoff ? 'text-green-600' : 'text-amber-600'}">
							{result.score >= latestDraw.crs_cutoff ? '✓ ' + t('crs_calculator.above', 'above') : '✗ ' + t('crs_calculator.below', 'below')}
						</span>
						{latestDraw.crs_cutoff} ({latestDraw.draw_date})
					</div>
				{/if}
			</div>

			{#if result.saved}
				<p class="mt-2 text-xs text-green-600">
					{t('crs_calculator.saved', 'Score saved to your history')}
				</p>
			{/if}
			{#if result.ruleset_version}
				<p class="mt-1 text-xs text-[var(--color-muted)]">
					{t('crs_calculator.ruleset', 'Ruleset')}: {result.ruleset_version}
				</p>
			{/if}

			<!-- Breakdown -->
			<div class="mt-5 space-y-4">
				{#each breakdownGroups as [group, items] (group)}
					<div>
						<h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
							{t(`crs_calculator.group_${group}`, groupLabels[group] ?? group)}
						</h3>
						<div class="space-y-1">
							{#each items as item, i (`${item.key}-${i}`)}
								<div class="flex items-center justify-between text-sm text-[var(--color-text)]">
									<span>{item.label}</span>
									<span class="font-semibold tabular-nums">{item.points}</span>
								</div>
							{/each}
						</div>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Draw history chart -->
	{#if chartData.length > 0}
		<div class="mt-6 {cardCls} p-4">
			<div class="mb-3 flex flex-wrap items-center justify-between gap-3">
				<h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
					{t('crs_calculator.draw_trend', 'CRS cut-off trend')}
				</h2>
				{#if chartCategoryOptions.length > 1}
					<div class="w-44">
						<AutocompleteSelect
							ariaLabel={t('crs_calculator.draw_category', 'Draw category')}
							bind:value={chartCategory}
							options={chartCategoryOptions}
						/>
					</div>
				{/if}
			</div>
			<div class="h-56">
				<LineChart data={chartData} x="date" y="cutoff">
					{#snippet marks({ context })}
						{#each context.series.visibleSeries as s (s.key)}
							<Spline seriesKey={s.key} class="stroke-[var(--chart-1)]" />
							<Points
								seriesKey={s.key}
								fill="var(--chart-1)"
								stroke="var(--color-surface)"
								strokeWidth={1.5}
							/>
						{/each}
					{/snippet}
				</LineChart>
			</div>
		</div>
	{/if}

	<!-- Score history (auth) -->
	{#if authed}
		{#if scoreHistory.length > 0}
			<div class="mt-6 {cardCls} p-4">
				<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
					{t('crs_calculator.history', 'Your score history')}
				</h2>
				<div class="space-y-2">
					{#each scoreHistory as item (item.id)}
						<details class="group border-b border-[var(--color-border)] pb-2">
							<summary class="flex cursor-pointer items-center justify-between text-sm text-[var(--color-text)]">
								<span class="font-semibold tabular-nums">{item.score}</span>
								<span class="text-[var(--color-muted)]">
									{new Date(item.created_at).toLocaleDateString()}
									<span class="ms-1 inline-block transition-transform group-open:rotate-90">›</span>
								</span>
							</summary>
							{#if item.breakdown?.length}
								<div class="mt-2 space-y-1 ps-2">
									{#each item.breakdown as b, i (`${b.key}-${i}`)}
										<div class="flex items-center justify-between text-xs text-[var(--color-muted)]">
											<span>{b.label}</span>
											<span class="tabular-nums">{b.points}</span>
										</div>
									{/each}
								</div>
							{/if}
						</details>
					{/each}
				</div>
			</div>
		{/if}
	{:else}
		<div class="mt-6 rounded-[var(--radius-card)] border border-dashed border-[var(--color-border)] p-4 text-center">
			<p class="mb-3 text-sm text-[var(--color-muted)]">
				{t('crs_calculator.signin_hint', 'Sign in to save your score history and track changes over time.')}
			</p>
			<a
				href="/login?redirect={encodeURIComponent(page.url.pathname)}"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]"
			>
				{t('crs_calculator.signin', 'Sign in')}
			</a>
		</div>
	{/if}
</div>
