<script lang="ts">
	import { untrack } from 'svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Wallet } from 'lucide-svelte';
	import { EmptyState, AutocompleteSelect } from '$lib/components/ui';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import type {
		GeoSpaceOption,
		PofRequirement,
		PofCalculateResult,
		PofScheduleBand,
		PofScheduleFormula
	} from '$lib/types';
	import type { PageData } from './$types';

	/**
	 * Proof of Funds Calculator — §108, ADR-119.
	 * Destination → visa type → household size → GET /pof/calculate.
	 * The backend resolves the amount from the published schedule and
	 * converts to NGN/USD via the shared remittance service.
	 *
	 * Ref: japaconnect-engineering/sections/108-pof-calculator.md
	 */
	let { data }: { data: PageData } = $props();

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

	const title = $derived(t('pof_calculator.seo_title', 'Proof of Funds Calculator — JapaConnect'));
	const description = $derived(
		t(
			'pof_calculator.seo_description',
			'Work out the proof-of-funds requirement for your destination and family size — with live NGN and USD equivalents.'
		)
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
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
	<div class="mb-6">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Wallet size={20} /> {t('pof_calculator.title', 'Proof of Funds Calculator')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">
			{t(
				'pof_calculator.subtitle',
				'Work out the settlement funds you need to show — in local currency, NGN, and USD.'
			)}
		</p>
	</div>

	<form onsubmit={calculate} class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<!-- Destination -->
		<div class="mb-4">
			<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="dest">
				{t('pof_calculator.destination', 'Destination country')}
			</label>
			<DestinationSelector {destinations} bind:selectedId={selectedDest} onchange={onDestChange} />
		</div>

		<!-- Visa type -->
		{#if visaTypes.length > 0}
			<div class="mb-4">
				<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="visaType">
					{t('pof_calculator.visa_type', 'Visa / program type')}
				</label>
				<AutocompleteSelect
					id="visaType"
					bind:value={selectedVisaType}
					options={[
						{ value: '', label: t('pof_calculator.select_visa', 'Select visa type') },
						...visaTypes.map((vt) => ({ value: vt, label: vt }))
					]}
				/>
			</div>
		{/if}

		<!-- Household -->
		<div class="mb-5 grid gap-4 sm:grid-cols-2">
			<div>
				<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="members">
					{t('pof_calculator.members', 'Family members (you + spouse/partner)')}
				</label>
				<input
					id="members"
					type="number"
					bind:value={members}
					min="1"
					max="20"
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				/>
			</div>
			<div>
				<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="dependants">
					{t('pof_calculator.dependants', 'Dependants (children)')}
				</label>
				<input
					id="dependants"
					type="number"
					bind:value={dependants}
					min="0"
					max="20"
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				/>
			</div>
		</div>

		<button
			type="submit"
			disabled={calculating || !selectedDest || !selectedVisaType}
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
		>
			{calculating ? t('pof_calculator.calculating', 'Calculating…') : t('pof_calculator.calculate', 'Calculate requirement')}
		</button>
	</form>

	{#if calcError}
		<div class="mt-4 rounded-[var(--radius)] border border-[var(--color-danger)]/30 bg-[var(--color-danger)]/10 p-3 text-sm text-[var(--color-danger)]">
			{calcError}
		</div>
	{/if}

	<!-- Results -->
	{#each results as result (result.requirement_id)}
		<div class="mt-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div class="text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
				{t('pof_calculator.required', 'Required proof of funds')}
			</div>
			<div class="mt-2 text-3xl font-bold tabular-nums text-[var(--color-primary)]">
				{result.amount.toLocaleString()} {result.currency}
			</div>
			<div class="mt-1 text-sm text-[var(--color-muted)]">
				{result.program_label} · {t('pof_calculator.household', 'household of')} {result.household}
			</div>

			{#if result.ngn_equivalent || result.usd_equivalent}
				<div class="mt-4 grid gap-3 sm:grid-cols-2">
					{#if result.ngn_equivalent}
						<div class="rounded-[var(--radius)] border border-[var(--color-border)] p-3">
							<div class="text-xs text-[var(--color-muted)]">NGN</div>
							<div class="text-lg font-semibold tabular-nums text-[var(--color-text)]">
								₦{result.ngn_equivalent.toLocaleString(undefined, { maximumFractionDigits: 0 })}
							</div>
						</div>
					{/if}
					{#if result.usd_equivalent}
						<div class="rounded-[var(--radius)] border border-[var(--color-border)] p-3">
							<div class="text-xs text-[var(--color-muted)]">USD</div>
							<div class="text-lg font-semibold tabular-nums text-[var(--color-text)]">
								${result.usd_equivalent.toLocaleString(undefined, { maximumFractionDigits: 0 })}
							</div>
						</div>
					{/if}
				</div>
			{/if}

			{#if result.accepted_proof && result.accepted_proof.length > 0}
				<div class="mt-4">
					<h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
						{t('pof_calculator.accepted_proof', 'Accepted proof')}
					</h3>
					<ul class="space-y-1 text-sm text-[var(--color-text)]">
						{#each result.accepted_proof as proof, i (i)}
							<li class="flex items-start gap-2">
								<span class="mt-0.5 text-green-600">✓</span>
								{proof}
							</li>
						{/each}
					</ul>
				</div>
			{/if}

			<div class="mt-4 text-xs text-[var(--color-muted)]">
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
		<div class="mt-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
				{t('pof_calculator.requirements', 'Published requirements')}
			</h2>
			<div class="space-y-3">
				{#each requirements as req (req.id)}
					<div class="rounded-[var(--radius)] border border-[var(--color-border)] p-3">
						<div class="flex items-start justify-between gap-4">
							<div>
								<div class="font-medium text-[var(--color-text)]">{req.program_label}</div>
								<div class="mt-0.5 text-xs text-[var(--color-muted)]">
									{req.visa_type} · {req.currency} · {t('pof_calculator.effective', 'effective')} {req.effective_date}
								</div>
							</div>
							{#if req.source_url}
								<a href={req.source_url} target="_blank" rel="noopener" class="shrink-0 text-xs underline text-[var(--color-primary)]">
									{t('pof_calculator.source', 'Source')}
								</a>
							{/if}
						</div>
						<div class="mt-2 font-mono text-xs text-[var(--color-muted)]">{scheduleLabel(req)}</div>
					</div>
				{/each}
			</div>
		</div>
	{:else if selectedDest && !loadingReqs && requirements.length === 0}
		<EmptyState
			icon={Wallet}
			title={t('pof_calculator.no_requirements', 'No requirements yet')}
			body={t('pof_calculator.no_requirements_desc', 'Proof-of-funds requirements for this destination will appear here once published.')}
		/>
	{/if}
</div>
