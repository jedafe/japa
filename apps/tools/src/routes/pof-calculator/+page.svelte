<script lang="ts">
	import { untrack } from 'svelte';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import type {
		GeoSpaceOption,
		PofRequirement,
		PofCalculateResult,
		PofScheduleBand,
		PofScheduleFormula
	} from '$lib/types';

	/**
	 * Proof of Funds Calculator — §108, ADR-119.
	 * Destination → visa type → household size → GET /pof/calculate.
	 * The backend resolves the amount from the published schedule and
	 * converts to NGN/USD via the shared remittance service.
	 */
	let { data } = $props();

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let requirements = $state<PofRequirement[]>(untrack(() => data.requirements));
	let visaTypes = $state<string[]>(untrack(() => data.visaTypes));

	let selectedDest = $state('');
	let selectedVisaType = $state('');
	let members = $state(1);
	let dependants = $state(0);

	let results = $state<PofCalculateResult[]>([]);
	let calculating = $state(false);
	let calcError = $state('');
	let loadingReqs = $state(false);

	const seo = $derived(
		buildSeoMeta({
			title: t('pof_calculator.seo_title', 'Proof of Funds Calculator — JapaConnect Tools'),
			description: t(
				'pof_calculator.seo_description',
				'Work out the proof-of-funds requirement for your destination and family size — with live NGN and USD equivalents.'
			),
			path: '/pof-calculator',
			origin: page.url.origin,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'SoftwareApplication',
				name: 'Proof of Funds Calculator',
				applicationCategory: 'UtilitiesApplication',
				operatingSystem: 'Web',
				offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
				description:
					'Calculate the proof-of-funds requirement for your visa application — Canada, UK, and more.'
			}
		})
	);

	function isBandSchedule(s: unknown): s is PofScheduleBand[] {
		return Array.isArray(s);
	}

	function scheduleLabel(req: PofRequirement): string {
		if (isBandSchedule(req.amount_schedule)) {
			return req.amount_schedule
				.slice()
				.sort((a, b) => a.members - b.members)
				.map((b) => `${b.members}: ${b.amount.toLocaleString()}`)
				.join(' · ');
		}
		const f = req.amount_schedule as PofScheduleFormula;
		return `base ${f.base.toLocaleString()} + ${f.per_additional.toLocaleString()}/extra`;
	}

	async function onDestChange() {
		requirements = [];
		visaTypes = [];
		results = [];
		selectedVisaType = '';
		if (!selectedDest) return;
		loadingReqs = true;
		const [reqsRes, typesRes] = await Promise.all([
			api.get<PofRequirement[]>(`/pof/requirements?dest=${selectedDest}`),
			api.get<string[]>(`/pof/visa-types?dest=${selectedDest}`)
		]);
		loadingReqs = false;
		if (!reqsRes.error && reqsRes.data) requirements = reqsRes.data;
		if (!typesRes.error && typesRes.data) visaTypes = typesRes.data;
	}

	async function calculate(e: Event) {
		e.preventDefault();
		if (!selectedDest || !selectedVisaType) return;
		calculating = true;
		calcError = '';
		const params = new URLSearchParams({
			dest: selectedDest,
			visa_type: selectedVisaType,
			members: String(members),
			dependants: String(dependants)
		});
		const res = await api.get<PofCalculateResult[]>(`/pof/calculate?${params.toString()}`);
		calculating = false;
		if (res.error) {
			calcError = res.error.message;
			results = [];
			return;
		}
		results = res.data ?? [];
	}
</script>

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	{#if seo.jsonLdString}
		{@html `<script type="application/ld+json">${seo.jsonLdString}<\/script>`}
	{/if}
</svelte:head>

{#if !data.enabled}
	<ToolDisabled tool={data.tool} />
{:else}
	<div class="mx-auto max-w-3xl px-4 py-8">
		<div class="mb-8">
			<h1 class="text-3xl font-bold tracking-tight">
				{t('pof_calculator.title', 'Proof of Funds Calculator')}
			</h1>
			<p class="mt-2 text-muted">
				{t(
					'pof_calculator.subtitle',
					'Work out the settlement funds you need to show — in local currency, NGN, and USD.'
				)}
			</p>
		</div>

		<form onsubmit={calculate} class="rounded-xl border border-border bg-surface p-6">
			<!-- Destination -->
			<div class="mb-4">
				<label class="block text-sm font-medium mb-1" for="dest">
					{t('pof_calculator.destination', 'Destination country')}
				</label>
				<DestinationSelector
					{destinations}
					bind:selectedId={selectedDest}
					onchange={onDestChange}
				/>
			</div>

			<!-- Visa type -->
			{#if visaTypes.length > 0}
				<div class="mb-4">
					<label class="block text-sm font-medium mb-1" for="visaType">
						{t('pof_calculator.visa_type', 'Visa / program type')}
					</label>
					<select id="visaType" bind:value={selectedVisaType} class="input w-full">
						<option value="" disabled>{t('pof_calculator.select_visa', 'Select visa type')}</option>
						{#each visaTypes as vt (vt)}
							<option value={vt}>{vt}</option>
						{/each}
					</select>
				</div>
			{/if}

			<!-- Household -->
			<div class="grid gap-4 sm:grid-cols-2 mb-6">
				<div>
					<label class="block text-sm font-medium mb-1" for="members">
						{t('pof_calculator.members', 'Family members (you + spouse/partner)')}
					</label>
					<input id="members" type="number" bind:value={members} min="1" max="20" class="input w-full" />
				</div>
				<div>
					<label class="block text-sm font-medium mb-1" for="dependants">
						{t('pof_calculator.dependants', 'Dependants (children)')}
					</label>
					<input id="dependants" type="number" bind:value={dependants} min="0" max="20" class="input w-full" />
				</div>
			</div>

			<button
				type="submit"
				class="btn-primary w-full sm:w-auto"
				disabled={calculating || !selectedDest || !selectedVisaType}
			>
				{calculating ? t('pof_calculator.calculating', 'Calculating…') : t('pof_calculator.calculate', 'Calculate requirement')}
			</button>
		</form>

		{#if calcError}
			<div class="mt-6 rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-600">
				{calcError}
			</div>
		{/if}

		<!-- Results -->
		{#each results as result (result.requirement_id)}
			<div class="mt-6 rounded-xl border border-border bg-surface p-6">
				<div class="text-xs font-medium text-muted uppercase tracking-wide">
					{t('pof_calculator.required', 'Required proof of funds')}
				</div>
				<div class="mt-2 text-4xl font-bold tabular-nums text-accent">
					{result.amount.toLocaleString()} {result.currency}
				</div>
				<div class="mt-1 text-sm text-muted">
					{result.program_label} · {t('pof_calculator.household', 'household of')} {result.household}
				</div>

				{#if result.ngn_equivalent || result.usd_equivalent}
					<div class="mt-4 grid gap-3 sm:grid-cols-2">
						{#if result.ngn_equivalent}
							<div class="rounded-lg border border-border p-3">
								<div class="text-xs text-muted">NGN</div>
								<div class="text-lg font-semibold tabular-nums">
									₦{result.ngn_equivalent.toLocaleString(undefined, { maximumFractionDigits: 0 })}
								</div>
							</div>
						{/if}
						{#if result.usd_equivalent}
							<div class="rounded-lg border border-border p-3">
								<div class="text-xs text-muted">USD</div>
								<div class="text-lg font-semibold tabular-nums">
									${result.usd_equivalent.toLocaleString(undefined, { maximumFractionDigits: 0 })}
								</div>
							</div>
						{/if}
					</div>
				{/if}

				{#if result.accepted_proof && result.accepted_proof.length > 0}
					<div class="mt-4">
						<h3 class="text-xs font-semibold text-muted uppercase tracking-wide mb-2">
							{t('pof_calculator.accepted_proof', 'Accepted proof')}
						</h3>
						<ul class="text-sm space-y-1">
							{#each result.accepted_proof as proof, i (i)}
								<li class="flex items-start gap-2">
									<span class="text-green-600 mt-0.5">✓</span>
									{proof}
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				<div class="mt-4 text-xs text-muted">
					{#if result.funds_must_be_liquid}
						{t('pof_calculator.liquid', 'Funds must be liquid and immediately available.')}
					{/if}
					{t('pof_calculator.effective', 'Effective')} {result.effective_date}
					{#if result.source_url}
						· <a href={result.source_url} target="_blank" rel="noopener" class="underline">
							{t('pof_calculator.source', 'Official source')}
						</a>
					{/if}
				</div>
			</div>
		{/each}

		<!-- Published requirements (browse) -->
		{#if requirements.length > 0 && results.length === 0}
			<div class="mt-8 rounded-xl border border-border bg-surface p-6">
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-4">
					{t('pof_calculator.requirements', 'Published requirements')}
				</h2>
				<div class="space-y-3">
					{#each requirements as req (req.id)}
						<div class="rounded-lg border border-border p-4">
							<div class="flex items-start justify-between gap-4">
								<div>
									<div class="font-medium">{req.program_label}</div>
									<div class="text-xs text-muted mt-0.5">
										{req.visa_type} · {req.currency} · {t('pof_calculator.effective', 'effective')} {req.effective_date}
									</div>
								</div>
								{#if req.source_url}
									<a href={req.source_url} target="_blank" rel="noopener" class="text-xs underline shrink-0">
										{t('pof_calculator.source', 'Source')}
									</a>
								{/if}
							</div>
							<div class="mt-2 text-xs text-muted font-mono">{scheduleLabel(req)}</div>
						</div>
					{/each}
				</div>
			</div>
		{:else if selectedDest && !loadingReqs && requirements.length === 0}
			<EmptyState
				icon="wallet"
				title={t('pof_calculator.no_requirements', 'No requirements yet')}
				description={t('pof_calculator.no_requirements_desc', 'Proof-of-funds requirements for this destination will appear here once published.')}
			/>
		{/if}
	</div>
{/if}
