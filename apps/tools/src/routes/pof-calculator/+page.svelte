<script lang="ts">
	import { untrack } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import Icon from '$lib/components/ui/Icon.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { formatMoney, currencySymbol, currencyName } from '$lib/utils/currency';
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

	let selectedDest = $state<string>(untrack(() => data.initialDestId ?? ''));
	let selectedVisaType = $state<string>(untrack(() => data.initialVisaType ?? ''));
	let members = $state(1);
	let dependants = $state(0);

	let results = $state<PofCalculateResult[]>([]);
	let calculating = $state(false);
	let calcError = $state('');
	let loadingReqs = $state(false);
	// Guards against out-of-order responses when the destination is
	// changed again before a previous fetch resolves.
	let destReqToken = 0;

	const seo = $derived(
		buildSeoMeta({
			title: t('pof_calculator.seo_title', 'Proof of Funds Calculator — JapaConnect Tools'),
			description: t(
				'pof_calculator.seo_description',
				'Work out the proof-of-funds requirement for your destination and family size — with live NGN and USD equivalents.'
			),
			path: '/pof-calculator',
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-pof-calculator.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'SoftwareApplication',
				name: 'Proof of Funds Calculator',
				applicationCategory: 'UtilitiesApplication',
				operatingSystem: 'Web',
				offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
				description: t(
					'pof_calculator.seo_description',
					'Work out the proof-of-funds requirement for your destination and family size — with live NGN and USD equivalents.'
				)
			}
		})
	);

	function humanize(value: string): string {
		return value
			.split(/[_-]+/)
			.filter(Boolean)
			.map((w) => w.charAt(0).toUpperCase() + w.slice(1))
			.join(' ');
	}

	function isBandSchedule(s: unknown): s is PofScheduleBand[] {
		return Array.isArray(s);
	}

	function scheduleLabel(req: PofRequirement): string {
		if (isBandSchedule(req.amount_schedule)) {
			return req.amount_schedule
				.slice()
				.sort((a, b) => a.members - b.members)
				.map(
					(b) =>
						`${b.members}: ${b.amount.toLocaleString()}` +
						(b.per_additional ? ` (+${b.per_additional.toLocaleString()}/extra)` : '')
				)
				.join(' · ');
		}
		const f = req.amount_schedule as PofScheduleFormula;
		return `base ${f.base.toLocaleString()} + ${f.per_additional.toLocaleString()}/extra`;
	}

	// ADR-119: figures display a staleness warning when the effective date
	// is older than ~12 months (POF tables are republished annually).
	function isStale(dateStr: string): boolean {
		const d = Date.parse(dateStr);
		if (Number.isNaN(d)) return false;
		return Date.now() - d > 365 * 24 * 60 * 60 * 1000;
	}

	function fetchVisaTypes(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered =
			query === ''
				? visaTypes
				: visaTypes.filter(
						(v) => v.toLowerCase().includes(query) || humanize(v).toLowerCase().includes(query)
					);
		return Promise.resolve(filtered.map((v) => ({ id: v, label: humanize(v) })));
	}

	function fetchVisaTypesInitial(): Promise<AutocompleteItem[]> {
		return fetchVisaTypes('');
	}

	// Keep the selection in the URL so results pages are shareable
	// (?dest=&visa_type= are consumed by the server load).
	function syncUrl() {
		const url = new URL(page.url);
		if (selectedDest) url.searchParams.set('dest', selectedDest);
		else url.searchParams.delete('dest');
		if (selectedVisaType) url.searchParams.set('visa_type', selectedVisaType);
		else url.searchParams.delete('visa_type');
		replaceState(url, {});
	}

	function clearResults() {
		results = [];
		calcError = '';
	}

	async function onDestChange(id: string) {
		// Bump the token before the early return so clearing the destination
		// also invalidates a still in-flight fetch for the previous one.
		const token = ++destReqToken;
		requirements = [];
		visaTypes = [];
		results = [];
		calcError = '';
		selectedVisaType = '';
		loadingReqs = false;
		syncUrl();
		if (!id) return;
		loadingReqs = true;
		const [reqsRes, typesRes] = await Promise.all([
			api.get<PofRequirement[]>(`/pof/requirements?dest=${encodeURIComponent(id)}`),
			api.get<string[]>(`/pof/visa-types?dest=${encodeURIComponent(id)}`)
		]);
		if (token !== destReqToken) return;
		loadingReqs = false;
		if (reqsRes.error || typesRes.error) {
			calcError = t('pof_calculator.load_failed', 'Failed to load requirements for this destination.');
			return;
		}
		if (reqsRes.data) requirements = reqsRes.data;
		if (typesRes.data) visaTypes = typesRes.data;
	}

	function onVisaTypeChange(id: string) {
		selectedVisaType = id;
		clearResults();
		syncUrl();
	}

	async function calculate(e: Event) {
		e.preventDefault();
		if (!selectedDest || !selectedVisaType) return;
		calculating = true;
		calcError = '';
		const params = new URLSearchParams({
			dest: selectedDest,
			visa_type: selectedVisaType,
			// Household bounds mirror the API contract (members+dependants <= 50).
			members: String(Math.max(1, Math.min(50, Math.floor(Number(members)) || 1))),
			dependants: String(Math.max(0, Math.min(49, Math.floor(Number(dependants)) || 0)))
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

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

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
			<!-- Destination (self-labeled by DestinationSelector) -->
			<div class="mb-4">
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
					<Autocomplete
						id="visaType"
						placeholder={t('pof_calculator.select_visa', 'Select visa type')}
						emptyMessage={t('pof_calculator.no_visa_types', 'No visa types found')}
						fetcher={fetchVisaTypes}
						initialFetcher={fetchVisaTypesInitial}
						selectedId={selectedVisaType || null}
						selectedLabel={selectedVisaType ? humanize(selectedVisaType) : null}
						onSelect={(item) => onVisaTypeChange(item.id)}
						onClear={() => onVisaTypeChange('')}
						class="w-full"
					/>
				</div>
			{/if}

			<!-- Household -->
			<div class="grid gap-4 sm:grid-cols-2 mb-6">
				<div>
					<label class="block text-sm font-medium mb-1" for="members">
						{t('pof_calculator.members', 'Family members (you + spouse/partner)')}
					</label>
					<input id="members" type="number" bind:value={members} onchange={clearResults} min="1" max="50" class="input w-full" />
				</div>
				<div>
					<label class="block text-sm font-medium mb-1" for="dependants">
						{t('pof_calculator.dependants', 'Dependants (children)')}
					</label>
					<input id="dependants" type="number" bind:value={dependants} onchange={clearResults} min="0" max="49" class="input w-full" />
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
					{formatMoney(result.amount, result.currency, data.currencies)}
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
									{currencySymbol('NGN', data.currencies)}{result.ngn_equivalent.toLocaleString(undefined, { maximumFractionDigits: 0 })}
								</div>
							</div>
						{/if}
						{#if result.usd_equivalent}
							<div class="rounded-lg border border-border p-3">
								<div class="text-xs text-muted">USD</div>
								<div class="text-lg font-semibold tabular-nums">
									{currencySymbol('USD', data.currencies)}{result.usd_equivalent.toLocaleString(undefined, { maximumFractionDigits: 0 })}
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
									{humanize(proof)}
								</li>
							{/each}
						</ul>
					</div>
				{/if}

				{#if result.notes}
					<p class="mt-4 text-sm text-muted">{result.notes}</p>
				{/if}

				{#if isStale(result.effective_date)}
					<div
						class="mt-4 flex items-start gap-2 rounded-lg p-3 text-xs"
						style="background: color-mix(in srgb, var(--color-warning) 10%, transparent); border: 1px solid var(--color-warning); color: var(--color-warning)"
					>
						<Icon name="alert-triangle" size={14} class="mt-0.5 flex-shrink-0" />
						{t(
							'pof_calculator.stale_warning',
							'These figures may be outdated — the official requirement was last updated over a year ago. Verify against the official source.'
						)}
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
									<div class="flex items-center gap-2 font-medium">
										{req.program_label}
										{#if isStale(req.effective_date)}
											<span
												class="inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] font-medium"
												style="background: color-mix(in srgb, var(--color-warning) 10%, transparent); border: 1px solid var(--color-warning); color: var(--color-warning)"
												title={t('pof_calculator.stale_warning', 'These figures may be outdated')}
											>
												<Icon name="alert-triangle" size={10} />
												{t('pof_calculator.stale', 'Outdated')}
											</span>
										{/if}
									</div>
									<div class="text-xs text-muted mt-0.5">
										{humanize(req.visa_type)} · {currencyName(req.currency, data.currencies) || req.currency} · {t('pof_calculator.effective', 'effective')} {req.effective_date}
									</div>
								</div>
								{#if req.source_url}
									<a href={req.source_url} target="_blank" rel="noopener" class="text-xs underline shrink-0">
										{t('pof_calculator.source', 'Source')}
									</a>
								{/if}
							</div>
							<div class="mt-2 text-xs text-muted font-mono">{scheduleLabel(req)}</div>
							{#if req.notes}
								<p class="mt-2 text-xs text-muted">{req.notes}</p>
							{/if}
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
