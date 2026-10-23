<script lang="ts">
	import { untrack } from 'svelte';
	import { api } from '$lib/api/client';
	import { buildSeoMeta, mainAppUrl } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { LineChart } from 'layerchart';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type {
		CrsFactorsResponse,
		CrsDraw,
		CrsScoreHistoryItem,
		CrsCalculateResult,
		CrsCalculateInput
	} from '$lib/types';

	/**
	 * CRS Calculator tool — §107, ADR-118.
	 * Profile form → POST /crs/calculate → canonical score + factor
	 * breakdown. Cut-off trend chart from SSR-loaded draw history. Score
	 * history is saved automatically when the user is signed in.
	 */
	let { data } = $props();

	let factors = $state<CrsFactorsResponse | null>(untrack(() => data.factors));
	let latestDraws = $state<CrsDraw[]>(untrack(() => data.latestDraws));
	let drawHistory = $state<CrsDraw[]>(untrack(() => data.drawHistory));
	let scoreHistory = $state<CrsScoreHistoryItem[]>(untrack(() => data.scoreHistory));
	let user = $state(untrack(() => data.user));

	// --- Profile form state ---
	let maritalStatus = $state<'single' | 'married'>('single');
	let age = $state(30);
	let education = $state('bachelors');
	let clbListening = $state(8);
	let clbReading = $state(8);
	let clbWriting = $state(8);
	let clbSpeaking = $state(8);
	let hasSecondLanguage = $state(false);
	let clb2Listening = $state(7);
	let clb2Reading = $state(7);
	let clb2Writing = $state(7);
	let clb2Speaking = $state(7);
	let canadianExp = $state(0);
	let foreignExp = $state(1);
	let hasCertificate = $state(false);
	let provincialNomination = $state(false);
	let jobOffer = $state(false);
	let siblingInCanada = $state(false);
	let hasFrench = $state(false);
	let frenchListening = $state(7);
	let frenchReading = $state(7);
	let frenchWriting = $state(7);
	let frenchSpeaking = $state(7);
	let canadianEducation = $state('none');

	// Spouse fields (married only)
	let spouseEducation = $state('bachelors');
	let spouseClbListening = $state(7);
	let spouseClbReading = $state(7);
	let spouseClbWriting = $state(7);
	let spouseClbSpeaking = $state(7);
	let spouseCanadianExp = $state(0);

	// --- Result state ---
	let result = $state<CrsCalculateResult | null>(null);
	let calculating = $state(false);
	let calcError = $state('');
	let showRegisterPrompt = $state(false);

	const seo = $derived(
		buildSeoMeta({
			title: t('crs_calculator.seo_title', 'Express Entry CRS Calculator — {site} Tools'),
			description: t(
				'crs_calculator.seo_description',
				'Calculate your Comprehensive Ranking System score for Canada Express Entry — factor breakdown, draw cut-offs, and score history.'
			),
			path: '/crs-calculator',
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-crs-calculator.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'SoftwareApplication',
				name: 'Express Entry CRS Calculator',
				applicationCategory: 'UtilitiesApplication',
				operatingSystem: 'Web',
				offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
				description:
					'Calculate your Comprehensive Ranking System score for Canada Express Entry with the live IRCC ruleset.'
			}
		})
	);

	// Education options — the backend matches on the input_descriptor key.
	// Labels come from the active ruleset factors when available.
	const educationOptions = $derived.by(() => {
		const eduFactor = factors?.factors?.find((f) => f.key === 'education');
		if (eduFactor && typeof eduFactor.input_descriptor === 'object') {
			const desc = eduFactor.input_descriptor as { levels?: Array<{ value: string; label: string }> };
			if (Array.isArray(desc.levels)) return desc.levels;
		}
		return [
			{ value: 'high_school', label: 'High school diploma' },
			{ value: 'one_year', label: 'One-year post-secondary' },
			{ value: 'two_year', label: 'Two-year post-secondary' },
			{ value: 'bachelors', label: "Bachelor's degree" },
			{ value: 'two_or_more', label: 'Two or more degrees' },
			{ value: 'masters', label: "Master's degree" },
			{ value: 'phd', label: 'PhD' }
		];
	});

	const canadianEducationOptions = [
		{ value: 'none', label: 'None' },
		{ value: 'one_or_two_years', label: '1–2 year credential' },
		{ value: 'three_plus_years', label: '3+ year credential' }
	];

	// Chart data — draw cut-off over time (general + all-program draws).
	const chartData = $derived.by(() => {
		return drawHistory
			.slice()
			.sort((a, b) => a.draw_date.localeCompare(b.draw_date))
			.map((d) => ({ date: new Date(d.draw_date), cutoff: d.crs_cutoff }));
	});

	function buildInput(): CrsCalculateInput {
		const input: CrsCalculateInput = {
			marital_status: maritalStatus,
			age,
			education,
			clb_first: {
				listening: clbListening,
				reading: clbReading,
				writing: clbWriting,
				speaking: clbSpeaking
			},
			canadian_experience_years: canadianExp,
			foreign_experience_years: foreignExp,
			certificate_of_qualification: hasCertificate,
			provincial_nomination: provincialNomination,
			job_offer: jobOffer,
			sibling_in_canada: siblingInCanada,
			canadian_education: canadianEducation
		};
		if (hasSecondLanguage) {
			input.clb_second = {
				listening: clb2Listening,
				reading: clb2Reading,
				writing: clb2Writing,
				speaking: clb2Speaking
			};
		}
		if (maritalStatus === 'married') {
			input.spouse = {
				education: spouseEducation,
				clb_first: {
					listening: spouseClbListening,
					reading: spouseClbReading,
					writing: spouseClbWriting,
					speaking: spouseClbSpeaking
				},
				canadian_experience_years: spouseCanadianExp
			};
		}
		if (hasFrench) {
			input.french_clb = {
				listening: frenchListening,
				reading: frenchReading,
				writing: frenchWriting,
				speaking: frenchSpeaking
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
		if (res.data?.saved && user) {
			// Refresh history so the saved score appears immediately.
			const hist = await api.get<CrsScoreHistoryItem[]>('/me/crs-scores');
			if (!hist.error && hist.data) scoreHistory = hist.data;
		}
	}

	// Group breakdown by factor group for display.
	const breakdownGroups = $derived.by(() => {
		if (!result) return {};
		const groups: Record<string, typeof result.breakdown> = {};
		for (const b of result.breakdown) {
			if (!groups[b.group]) groups[b.group] = [];
			groups[b.group].push(b);
		}
		return groups;
	});

	const groupLabels: Record<string, string> = {
		core: 'Core / Human capital',
		spouse: 'Spouse factors',
		transferability: 'Skill transferability',
		additional: 'Additional points'
	};
</script>

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

{#if !data.enabled}
	<ToolDisabled tool={data.tool} />
{:else}
	<div class="mx-auto max-w-4xl px-4 py-8">
		<!-- Header -->
		<div class="mb-8">
			<h1 class="text-3xl font-bold tracking-tight">
				{t('crs_calculator.title', 'Express Entry CRS Calculator')}
			</h1>
			<p class="mt-2 text-muted">
				{t(
					'crs_calculator.subtitle',
					'Calculate your Comprehensive Ranking System score against the current IRCC ruleset.'
				)}
			</p>
			{#if factors?.ruleset}
				<p class="mt-1 text-xs text-muted">
					{t('crs_calculator.ruleset', 'Ruleset')}: {factors.ruleset.version_label}
					· {t('crs_calculator.effective', 'Effective')} {factors.ruleset.effective_date}
				</p>
			{/if}
		</div>

		<!-- Latest draw cut-offs -->
		{#if latestDraws.length > 0}
			<div class="mb-8 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
				{#each latestDraws as draw (draw.id)}
					<div class="rounded-lg border border-border bg-surface p-4">
						<div class="text-xs font-medium text-muted uppercase tracking-wide">{draw.category}</div>
						<div class="mt-1 text-2xl font-bold tabular-nums">{draw.crs_cutoff}</div>
						<div class="mt-0.5 text-xs text-muted">
							{draw.draw_date} · {draw.invitations_issued.toLocaleString()} ITAs
						</div>
					</div>
				{/each}
			</div>
		{/if}

		<!-- Calculator form -->
		<form onsubmit={calculate} class="rounded-xl border border-border bg-surface p-6">
			<h2 class="text-lg font-semibold mb-4">{t('crs_calculator.form_title', 'Your profile')}</h2>

			<div class="grid gap-4 sm:grid-cols-2">
				<!-- Marital status -->
				<div>
					<label class="block text-sm font-medium mb-1" for="marital">
						{t('crs_calculator.marital', 'Marital status')}
					</label>
					<AutocompleteSelect
						id="marital"
						bind:value={maritalStatus}
						options={[
							{ value: 'single', label: t('crs_calculator.single', 'Single / not accompanying spouse') },
							{ value: 'married', label: t('crs_calculator.married', 'Married (spouse accompanying)') }
						]}
						class="w-full"
					/>
				</div>

				<!-- Age -->
				<div>
					<label class="block text-sm font-medium mb-1" for="age">
						{t('crs_calculator.age', 'Age')}
					</label>
					<input id="age" type="number" bind:value={age} min="17" max="50" class="input w-full" />
				</div>

				<!-- Education -->
				<div>
					<label class="block text-sm font-medium mb-1" for="education">
						{t('crs_calculator.education', 'Highest education')}
					</label>
					<AutocompleteSelect
						id="education"
						bind:value={education}
						options={educationOptions}
						class="w-full"
					/>
				</div>

				<!-- Canadian experience -->
				<div>
					<label class="block text-sm font-medium mb-1" for="canadianExp">
						{t('crs_calculator.canadian_exp', 'Canadian work experience (years)')}
					</label>
					<input id="canadianExp" type="number" bind:value={canadianExp} min="0" max="10" class="input w-full" />
				</div>

				<!-- Foreign experience -->
				<div>
					<label class="block text-sm font-medium mb-1" for="foreignExp">
						{t('crs_calculator.foreign_exp', 'Foreign work experience (years)')}
					</label>
					<input id="foreignExp" type="number" bind:value={foreignExp} min="0" max="10" class="input w-full" />
				</div>

				<!-- Canadian education -->
				<div>
					<label class="block text-sm font-medium mb-1" for="canadianEdu">
						{t('crs_calculator.canadian_edu', 'Canadian education')}
					</label>
					<AutocompleteSelect
						id="canadianEdu"
						bind:value={canadianEducation}
						options={canadianEducationOptions}
						class="w-full"
					/>
				</div>
			</div>

			<!-- First language (CLB) -->
			<fieldset class="mt-6">
				<legend class="text-sm font-medium mb-2">
					{t('crs_calculator.first_language', 'First official language (CLB scores)')}
				</legend>
				<div class="grid gap-3 grid-cols-2 sm:grid-cols-4">
					{#each [['Listening', 'listening'], ['Reading', 'reading'], ['Writing', 'writing'], ['Speaking', 'speaking']] as [label, key] (key)}
						<div>
							<label class="block text-xs text-muted mb-1" for="clb-{key}">{label}</label>
							<input
								id="clb-{key}"
								type="number"
								min="4"
								max="10"
								class="input w-full"
								value={key === 'listening' ? clbListening : key === 'reading' ? clbReading : key === 'writing' ? clbWriting : clbSpeaking}
								onchange={(e) => {
									const v = Number((e.target as HTMLInputElement).value);
									if (key === 'listening') clbListening = v;
									else if (key === 'reading') clbReading = v;
									else if (key === 'writing') clbWriting = v;
									else clbSpeaking = v;
								}}
							/>
						</div>
					{/each}
				</div>
			</fieldset>

			<!-- Second language toggle -->
			<div class="mt-4">
				<label class="inline-flex items-center gap-2 text-sm">
					<input type="checkbox" bind:checked={hasSecondLanguage} class="rounded" />
					{t('crs_calculator.second_language', 'I have a second official language result')}
				</label>
			</div>

			{#if hasSecondLanguage}
				<fieldset class="mt-3 rounded-lg border border-border p-4">
					<legend class="text-sm font-medium px-1">Second language (CLB)</legend>
					<div class="grid gap-3 grid-cols-2 sm:grid-cols-4">
						{#each [['Listening', 'l'], ['Reading', 'r'], ['Writing', 'w'], ['Speaking', 's']] as [label, key] (key)}
							<div>
								<label class="block text-xs text-muted mb-1" for="clb2-{key}">{label}</label>
								<input
									id="clb2-{key}"
									type="number"
									min="4"
									max="10"
									class="input w-full"
									value={key === 'l' ? clb2Listening : key === 'r' ? clb2Reading : key === 'w' ? clb2Writing : clb2Speaking}
									onchange={(e) => {
										const v = Number((e.target as HTMLInputElement).value);
										if (key === 'l') clb2Listening = v;
										else if (key === 'r') clb2Reading = v;
										else if (key === 'w') clb2Writing = v;
										else clb2Speaking = v;
									}}
								/>
							</div>
						{/each}
					</div>
				</fieldset>
			{/if}

			<!-- Spouse section (married only) -->
			{#if maritalStatus === 'married'}
				<fieldset class="mt-6 rounded-lg border border-border p-4">
					<legend class="text-sm font-medium px-1">{t('crs_calculator.spouse', 'Spouse')}</legend>
					<div class="grid gap-4 sm:grid-cols-3">
						<div>
							<label class="block text-xs text-muted mb-1" for="spouseEdu">{t('crs.spouse_education', 'Education')}</label>
							<AutocompleteSelect
								id="spouseEdu"
								bind:value={spouseEducation}
								options={educationOptions}
								class="w-full"
							/>
						</div>
						<div>
							<label class="block text-xs text-muted mb-1" for="spouseExp">{t('crs.spouse_experience', 'Canadian experience (yrs)')}</label>
							<input id="spouseExp" type="number" bind:value={spouseCanadianExp} min="0" max="10" class="input w-full" />
						</div>
						<div class="sm:col-span-1"></div>
					</div>
					<div class="mt-3 grid gap-3 grid-cols-2 sm:grid-cols-4">
						{#each [['Listening', 'l'], ['Reading', 'r'], ['Writing', 'w'], ['Speaking', 's']] as [label, key] (key)}
							<div>
								<label class="block text-xs text-muted mb-1" for="spouse-clb-{key}">{label} (CLB)</label>
								<input
									id="spouse-clb-{key}"
									type="number"
									min="4"
									max="10"
									class="input w-full"
									value={key === 'l' ? spouseClbListening : key === 'r' ? spouseClbReading : key === 'w' ? spouseClbWriting : spouseClbSpeaking}
									onchange={(e) => {
										const v = Number((e.target as HTMLInputElement).value);
										if (key === 'l') spouseClbListening = v;
										else if (key === 'r') spouseClbReading = v;
										else if (key === 'w') spouseClbWriting = v;
										else spouseClbSpeaking = v;
									}}
								/>
							</div>
						{/each}
					</div>
				</fieldset>
			{/if}

			<!-- Additional factors -->
			<fieldset class="mt-6">
				<legend class="text-sm font-medium mb-2">{t('crs_calculator.additional', 'Additional factors')}</legend>
				<div class="grid gap-3 sm:grid-cols-2">
					<label class="inline-flex items-center gap-2 text-sm">
						<input type="checkbox" bind:checked={provincialNomination} class="rounded" />
						{t('crs_calculator.pnp', 'Provincial nomination (+600)')}
					</label>
					<label class="inline-flex items-center gap-2 text-sm">
						<input type="checkbox" bind:checked={jobOffer} class="rounded" />
						{t('crs_calculator.job_offer', 'Valid job offer')}
					</label>
					<label class="inline-flex items-center gap-2 text-sm">
						<input type="checkbox" bind:checked={siblingInCanada} class="rounded" />
						{t('crs_calculator.sibling', 'Sibling in Canada (citizen/PR)')}
					</label>
					<label class="inline-flex items-center gap-2 text-sm">
						<input type="checkbox" bind:checked={hasCertificate} class="rounded" />
						{t('crs_calculator.certificate', 'Certificate of qualification (trade)')}
					</label>
					<label class="inline-flex items-center gap-2 text-sm">
						<input type="checkbox" bind:checked={hasFrench} class="rounded" />
						{t('crs_calculator.french', 'French language results')}
					</label>
				</div>
			</fieldset>

			{#if hasFrench}
				<fieldset class="mt-3 rounded-lg border border-border p-4">
					<legend class="text-sm font-medium px-1">French (NCLC/CLB)</legend>
					<div class="grid gap-3 grid-cols-2 sm:grid-cols-4">
						{#each [['Listening', 'l'], ['Reading', 'r'], ['Writing', 'w'], ['Speaking', 's']] as [label, key] (key)}
							<div>
								<label class="block text-xs text-muted mb-1" for="fr-{key}">{label}</label>
								<input
									id="fr-{key}"
									type="number"
									min="4"
									max="10"
									class="input w-full"
									value={key === 'l' ? frenchListening : key === 'r' ? frenchReading : key === 'w' ? frenchWriting : frenchSpeaking}
									onchange={(e) => {
										const v = Number((e.target as HTMLInputElement).value);
										if (key === 'l') frenchListening = v;
										else if (key === 'r') frenchReading = v;
										else if (key === 'w') frenchWriting = v;
										else frenchSpeaking = v;
									}}
								/>
							</div>
						{/each}
					</div>
				</fieldset>
			{/if}

			<div class="mt-6">
				<button type="submit" class="btn-primary w-full sm:w-auto" disabled={calculating}>
					{calculating ? t('crs_calculator.calculating', 'Calculating…') : t('crs_calculator.calculate', 'Calculate my score')}
				</button>
			</div>
		</form>

		<!-- Result -->
		{#if calcError}
			<div class="mt-6 rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-600">
				{calcError}
			</div>
		{/if}

		{#if result}
			<div class="mt-6 rounded-xl border border-border bg-surface p-6">
				<div class="flex items-baseline gap-4">
					<div>
						<div class="text-xs font-medium text-muted uppercase tracking-wide">
							{t('crs_calculator.your_score', 'Your CRS score')}
						</div>
						<div class="mt-1 text-5xl font-bold tabular-nums text-accent">{result.score}</div>
					</div>
					{#if latestDraws.length > 0}
						{@const latest = latestDraws[0]}
						<div class="text-sm text-muted">
							{t('crs_calculator.vs_latest', 'vs latest draw')}:
							<span class="font-semibold {result.score >= latest.crs_cutoff ? 'text-green-600' : 'text-amber-600'}">
								{result.score >= latest.crs_cutoff ? '✓ ' + t('crs_calculator.above', 'above') : '✗ ' + t('crs_calculator.below', 'below')}
							</span>
							{latest.crs_cutoff} ({latest.draw_date})
						</div>
					{/if}
				</div>

				{#if result.saved}
					<p class="mt-2 text-xs text-green-600">
						{t('crs_calculator.saved', 'Score saved to your history')}
					</p>
				{/if}

				<!-- Breakdown -->
				<div class="mt-6 space-y-4">
					{#each Object.entries(breakdownGroups) as [group, items] (group)}
						<div>
							<h3 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
								{groupLabels[group] ?? group}
							</h3>
							<div class="space-y-1">
								{#each items as item (item.key)}
									<div class="flex items-center justify-between text-sm">
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
			<div class="mt-8 rounded-xl border border-border bg-surface p-6">
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-4">
					{t('crs_calculator.draw_trend', 'CRS cut-off trend')}
				</h2>
				<div class="h-56">
					<LineChart
						data={chartData}
						x="date"
						y="cutoff"
						props={{
							line: { class: 'stroke-[var(--chart-1)]' },
							area: { class: 'fill-[var(--chart-1)]/10' }
						}}
					/>
				</div>
			</div>
		{/if}

		<!-- Score history (auth) -->
		{#if user}
			{#if scoreHistory.length > 0}
				<div class="mt-8 rounded-xl border border-border bg-surface p-6">
					<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-4">
						{t('crs_calculator.history', 'Your score history')}
					</h2>
					<div class="space-y-2">
						{#each scoreHistory as item (item.id)}
							<div class="flex items-center justify-between text-sm border-b border-border pb-2">
								<span class="font-semibold tabular-nums">{item.score}</span>
								<span class="text-muted">{new Date(item.created_at).toLocaleDateString()}</span>
							</div>
						{/each}
					</div>
				</div>
			{/if}
		{:else}
			<div class="mt-8 rounded-xl border border-dashed border-border p-6 text-center">
				<p class="text-sm text-muted mb-3">
					{t('crs_calculator.signin_hint', 'Sign in to save your score history and track changes over time.')}
				</p>
				<button class="btn-secondary" onclick={() => (showRegisterPrompt = true)}>
					{t('crs_calculator.signin', 'Sign in')}
				</button>
			</div>
		{/if}
	</div>

	<RegistrationPrompt
		open={showRegisterPrompt}
		onClose={() => (showRegisterPrompt = false)}
		icon="gauge"
		title={t('register.crs_history', 'Save your score history')}
		description={t('register.crs_history_desc', 'Sign in to save your CRS scores and track how they change over time.')}
	/>
{/if}
