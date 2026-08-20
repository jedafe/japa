<script lang="ts">
	// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { ArrowLeft, Send, TriangleAlert } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { api } from '$lib/api/client';
	import type { ScamType, ScamPlatform } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const SCAM_TYPES: ScamType[] = [
		'fraudulent_agent',
		'fake_job',
		'housing_fraud',
		'fake_scholarship',
		'whatsapp_telegram',
		'other'
	];

	const PLATFORMS: ScamPlatform[] = [
		'whatsapp',
		'instagram',
		'linkedin',
		'email',
		'website',
		'other'
	];

	let scamType = $state<ScamType>('fraudulent_agent');
	let destinationSpaceId = $state('');
	let originSpaceId = $state('');
	let originCommunity = $state('');
	let platformUsed = $state<ScamPlatform>('whatsapp');
	let approximateDate = $state('');
	let amountRange = $state('');
	let description = $state('');
	let reportedToAuthorities = $state(false);

	let loading = $state(false);
	let errorKey = $state('');

	const canSubmit = $derived(
		scamType.length > 0 &&
			destinationSpaceId.length > 0 &&
			description.trim().length > 0 &&
			!loading
	);

	function scamTypeLabel(type: ScamType): string {
		return t(`scam_alerts.type.${type}`, type.replace(/_/g, ' '));
	}

	function platformLabel(p: ScamPlatform): string {
		return t(`scam_alerts.platform.${p}`, p);
	}

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none';

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!canSubmit) return;
		loading = true;
		try {
			const body: Record<string, unknown> = {
				scam_type: scamType,
				destination_space_id: destinationSpaceId,
				platform_used: platformUsed,
				description: description.trim()
			};
			if (originSpaceId) body.origin_space_id = originSpaceId;
			if (originCommunity.trim()) body.origin_community = originCommunity.trim();
			if (approximateDate) body.approximate_date = approximateDate;
			if (amountRange.trim()) body.amount_range = amountRange.trim();
			body.reported_to_authorities = reportedToAuthorities;

			const res = await api.post<{ id: string }>('/scam-alerts', body);
			if (res.data?.id) {
				await goto(`/scam-alerts/${res.data.id}`);
			} else {
				errorKey = 'scam_alerts.errors.submit_failed';
			}
		} catch {
			errorKey = 'scam_alerts.errors.submit_failed';
		} finally {
			loading = false;
		}
	}

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('common.all_countries', 'All countries');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...(data.countries ?? []).map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = (data.countries ?? []).filter(
			(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
		);
		const items = filtered.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
		return Promise.resolve(matchesAll ? [allItem, ...items] : items);
	}
	function fetchDestinationCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinationCountries('');
	}
	const selectedDestinationLabel = $derived(
		destinationSpaceId ? (data.countries ?? []).find((c) => c.id === destinationSpaceId)?.name ?? null : null
	);

	function fetchOriginCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('common.all_countries', 'All countries');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...(data.countries ?? []).map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = (data.countries ?? []).filter(
			(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
		);
		const items = filtered.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
		return Promise.resolve(matchesAll ? [allItem, ...items] : items);
	}
	function fetchOriginCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchOriginCountries('');
	}
	const selectedOriginLabel = $derived(
		originSpaceId ? (data.countries ?? []).find((c) => c.id === originSpaceId)?.name ?? null : null
	);
</script>

<svelte:head>
	<title>{t('scam_alerts.new_title', 'Report a Scam — JapaConnect')}</title>
	<meta name="description" content={t('scam_alerts.new_description', 'Report a scam to warn the community.')} />
	<meta name="robots" content="noindex" />
	<meta property="og:title" content={t('scam_alerts.new_title', 'Report a Scam')} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/scam-alerts"
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('scam_alerts.back', 'Back to scam alerts')}
		>
			<ArrowLeft size={18} />
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{t('scam_alerts.new_heading', 'Report a Scam')}
		</h1>
	</div>

	<form onsubmit={submit} class="mx-auto max-w-2xl space-y-5 p-4" novalidate>
		<div class="rounded-[var(--radius)] border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-700">
			<TriangleAlert size={14} class="mb-0.5 inline" />
			{t('scam_alerts.new_disclaimer', 'Do not include exact personal financial details. Use approximate amounts only.')}
		</div>

		<!-- Scam type -->
		<div class="flex flex-col gap-1.5">
			<label for="scam_type" class="text-sm font-medium text-[var(--color-text)]">
				{t('scam_alerts.field_scam_type', 'Scam type')} <span class="text-red-500">*</span>
			</label>
			<select
				id="scam_type"
				value={scamType}
				onchange={(e) => (scamType = e.currentTarget.value as ScamType)}
				class={inputClass}
			>
				{#each SCAM_TYPES as st (st)}
					<option value={st}>{scamTypeLabel(st)}</option>
				{/each}
			</select>
		</div>

		<!-- Destination + origin country -->
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
			<div class="flex flex-col gap-1.5">
				<label for="destination_space_id" class="text-sm font-medium text-[var(--color-text)]">
					{t('scam_alerts.field_destination', 'Destination country')} <span class="text-red-500">*</span>
				</label>
				<Autocomplete
					placeholder={t('scam_alerts.placeholder_destination', 'Select destination country')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinationCountries}
					initialFetcher={fetchDestinationCountriesInitial}
					selectedId={destinationSpaceId || null}
					selectedLabel={selectedDestinationLabel}
					onSelect={(item) => (destinationSpaceId = item.id)}
					onClear={() => (destinationSpaceId = '')}
				/>
			</div>
			<div class="flex flex-col gap-1.5">
				<label for="origin_space_id" class="text-sm font-medium text-[var(--color-text)]">
					{t('scam_alerts.field_origin_country', 'Origin country')}
				</label>
				<Autocomplete
					placeholder={t('scam_alerts.placeholder_origin_country', 'Select origin country')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchOriginCountries}
					initialFetcher={fetchOriginCountriesInitial}
					selectedId={originSpaceId || null}
					selectedLabel={selectedOriginLabel}
					onSelect={(item) => (originSpaceId = item.id)}
					onClear={() => (originSpaceId = '')}
				/>
			</div>
		</div>

		<!-- Origin community -->
		<div class="flex flex-col gap-1.5">
			<label for="origin_community" class="text-sm font-medium text-[var(--color-text)]">
				{t('scam_alerts.field_origin_community', 'Origin community')}
			</label>
			<input
				id="origin_community"
				type="text"
				value={originCommunity}
				oninput={(e) => (originCommunity = e.currentTarget.value)}
				placeholder={t('scam_alerts.placeholder_origin_community', 'e.g. Yoruba, Igbo')}
				class={inputClass}
			/>
		</div>

		<!-- Platform used -->
		<div class="flex flex-col gap-1.5">
			<label for="platform_used" class="text-sm font-medium text-[var(--color-text)]">
				{t('scam_alerts.field_platform', 'Platform used')}
			</label>
			<select
				id="platform_used"
				value={platformUsed}
				onchange={(e) => (platformUsed = e.currentTarget.value as ScamPlatform)}
				class={inputClass}
			>
				{#each PLATFORMS as p (p)}
					<option value={p}>{platformLabel(p)}</option>
				{/each}
			</select>
		</div>

		<!-- Approximate date + amount range -->
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
			<div class="flex flex-col gap-1.5">
				<label for="approximate_date" class="text-sm font-medium text-[var(--color-text)]">
					{t('scam_alerts.field_date', 'Approximate date')}
				</label>
				<input
					id="approximate_date"
					type="date"
					value={approximateDate}
					oninput={(e) => (approximateDate = e.currentTarget.value)}
					class={inputClass}
				/>
			</div>
			<div class="flex flex-col gap-1.5">
				<label for="amount_range" class="text-sm font-medium text-[var(--color-text)]">
					{t('scam_alerts.field_amount', 'Amount range')}
				</label>
				<input
					id="amount_range"
					type="text"
					value={amountRange}
					oninput={(e) => (amountRange = e.currentTarget.value)}
					placeholder={t('scam_alerts.placeholder_amount', 'e.g. £500–£1000')}
					class={inputClass}
				/>
			</div>
		</div>

		<!-- Description -->
		<div class="flex flex-col gap-1.5">
			<label for="description" class="text-sm font-medium text-[var(--color-text)]">
				{t('scam_alerts.field_description', 'Description')} <span class="text-red-500">*</span>
			</label>
			<textarea
				id="description"
				rows="6"
				value={description}
				oninput={(e) => (description = e.currentTarget.value)}
				placeholder={t('scam_alerts.placeholder_description', 'Describe what happened, how the scam worked, and any warning signs…')}
				class={inputClass}
			></textarea>
		</div>

		<!-- Reported to authorities -->
		<label class="flex cursor-pointer items-center gap-2 text-sm text-[var(--color-text-subtle)]">
			<input
				type="checkbox"
				checked={reportedToAuthorities}
				onchange={(e) => (reportedToAuthorities = e.currentTarget.checked)}
				class="h-4 w-4 accent-[var(--color-primary)]"
			/>
			{t('scam_alerts.field_reported_authorities', 'I reported this to the authorities')}
		</label>

		{#if errorKey}
			<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
				{t(errorKey, 'Failed to submit your report. Please try again.')}
			</p>
		{/if}

		<!-- Submit -->
		<div class="flex items-center justify-end gap-2 border-t border-[var(--color-border)] pt-4">
			<a
				href="/scam-alerts"
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{t('common.cancel', 'Cancel')}
			</a>
			<button
				type="submit"
				disabled={!canSubmit}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				<Send size={16} /> {loading ? t('common.submitting', 'Submitting…') : t('scam_alerts.submit', 'Submit report')}
			</button>
		</div>
	</form>
</div>
