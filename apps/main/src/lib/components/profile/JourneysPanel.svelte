<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { untrack } from 'svelte';
	// Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md (ADR-129)
	// Trips & other journeys — concurrent/past journey records beside the
	// primary profile journey. Lists GET /me/japa-stage/journeys, creates via
	// POST, and advances lifecycle state via PATCH status transitions
	// (planned → active → completed/cancelled; cancelled → planned).
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import RelativeTime from '$lib/components/ui/RelativeTime.svelte';
	import { CalendarRange, Plus, Trash2, X } from 'lucide-svelte';
	import {
		japaStageColors,
		japaStageLabel,
		japaStageOptions,
		japaStagesForIntent,
		japaIntentLabel,
		japaIntentOptions
	} from '$lib/enumOptions';
	import type { GeoCountry, JapaIntent, JapaStage, JapaStageOption, JourneyStatus, UserJourney } from '$lib/types';

	let {
		journeys: initialJourneys,
		countries,
		stages,
		intents
	}: {
		journeys: UserJourney[];
		countries: GeoCountry[];
		stages: JapaStageOption[];
		intents: JapaIntent[];
	} = $props();

	let items = $state<UserJourney[]>(untrack(() => initialJourneys));

	// Re-seed when SSR data refreshes (navigation/invalidation) — the
	// server list is the source of truth.
	$effect(() => {
		items = initialJourneys;
	});

	const intentOpts = $derived(japaIntentOptions(t, intents));

	// Create-form state.
	let showForm = $state(false);
	let jIntent = $state<JapaIntent>('visit');
	let jStage = $state<JapaStage | ''>('');
	let jDest = $state('');
	let jLabel = $state('');
	let jStart = $state('');
	let jEnd = $state('');
	let jPublic = $state(false);
	let busy = $state(false);
	let errorKey = $state('');

	const jStageOpts = $derived(japaStageOptions(t, japaStagesForIntent(stages, jIntent)));

	// Drop a stage selection that the new journey class does not allow.
	$effect(() => {
		if (jStage && !japaStagesForIntent(stages, jIntent).some((o) => o.value === jStage)) {
			jStage = '';
		}
	});

	const statusColors: Record<JourneyStatus, string> = {
		planned: 'var(--color-muted)',
		active: '#16a34a',
		completed: '#2563eb',
		cancelled: 'var(--color-text-subtle)'
	};

	function destLabel(j: UserJourney): string {
		return j.destination_space_name || (j.destination_space_id ? destName(j.destination_space_id) : '');
	}

	function destName(id?: string): string {
		if (!id) return '';
		return countries.find((c) => c.id === id)?.name ?? '';
	}

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		const filtered = (countries ?? []).filter(
			(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
		);
		return Promise.resolve(filtered.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code })));
	}

	function fetchDestinationCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinationCountries('');
	}

	const selectedDestinationLabel = $derived(
		jDest ? (countries ?? []).find((c) => c.id === jDest)?.name ?? null : null
	);

	function windowText(j: UserJourney): string {
		if (j.travel_start && j.travel_end) return `${j.travel_start} → ${j.travel_end}`;
		if (j.travel_start) return `${t('journeys.from', 'From')} ${j.travel_start}`;
		if (j.travel_end) return `${t('journeys.until', 'Until')} ${j.travel_end}`;
		return '';
	}

	async function create() {
		errorKey = '';
		busy = true;
		try {
			const res = await api.post<UserJourney>('/me/japa-stage/journeys', {
				intent: jIntent,
				...(jStage ? { stage: jStage } : {}),
				...(jDest ? { destination_space_id: jDest } : {}),
				...(jLabel.trim() ? { label: jLabel.trim() } : {}),
				...(jStart ? { travel_start: jStart } : {}),
				...(jEnd ? { travel_end: jEnd } : {}),
				is_public: jPublic
			});
			if (res.data) {
				items = [res.data, ...items];
				showForm = false;
				jLabel = '';
				jDest = '';
				jStart = '';
				jEnd = '';
				jStage = '';
				jPublic = false;
			} else {
				errorKey = 'journeys.save_failed';
			}
		} catch {
			errorKey = 'journeys.save_failed';
		} finally {
			busy = false;
		}
	}

	async function transition(j: UserJourney, status: JourneyStatus) {
		errorKey = '';
		try {
			const res = await api.patch<UserJourney>(`/me/japa-stage/journeys/${j.id}`, { status });
			if (res.data) {
				items = items.map((i) => (i.id === j.id ? res.data! : i));
			} else {
				errorKey = 'journeys.save_failed';
			}
		} catch {
			errorKey = 'journeys.save_failed';
		}
	}

	async function remove(j: UserJourney) {
		if (!confirm(t('journeys.delete_confirm', 'Delete this journey record?'))) return;
		errorKey = '';
		const res = await api.delete(`/me/japa-stage/journeys/${j.id}`);
		if (res.error) {
			errorKey = 'journeys.delete_failed';
			return;
		}
		items = items.filter((i) => i.id !== j.id);
	}

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
</script>

<div class="rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<div class="flex items-center justify-between">
		<div>
			<h2 class="text-sm font-semibold text-[var(--color-text)]">
				{t('journeys.title', 'Other journeys & trips')}
			</h2>
			<p class="mt-0.5 text-xs text-[var(--color-muted)]">
				{t('journeys.desc', 'Track trips and side journeys alongside your primary journey.')}
			</p>
		</div>
		{#if !showForm}
			<button
				type="button"
				onclick={() => (showForm = true)}
				class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition hover:bg-[var(--color-surface-hover)]"
			>
				<Plus size={14} /> {t('journeys.add', 'Add journey')}
			</button>
		{/if}
	</div>

	{#if items.length === 0 && !showForm}
		<div class="mt-3">
			<EmptyState
				icon={CalendarRange}
				title={t('journeys.empty', 'No journeys yet')}
				body={t('journeys.empty_body', 'Record a business trip, family visit, or another journey here.')}
			/>
		</div>
	{/if}

	{#if showForm}
		<form
			onsubmit={(e) => {
				e.preventDefault();
				create();
			}}
			class="mt-4 space-y-3 rounded-[10px] border border-[var(--color-border)] bg-[var(--color-bg)] p-4"
			novalidate
		>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
				<div class="flex flex-col gap-1.5">
					<label for="j-intent" class="text-xs font-medium text-[var(--color-text)]">
						{t('japa_intent.label', 'Journey type')}
					</label>
					<AutocompleteSelect
						id="j-intent"
						value={jIntent}
						options={intentOpts}
						onValueChange={(v) => (jIntent = v as JapaIntent)}
					/>
				</div>
				<div class="flex flex-col gap-1.5">
					<label for="j-stage" class="text-xs font-medium text-[var(--color-text)]">
						{t('japa_stage.current_stage', 'Current stage')}
					</label>
					<AutocompleteSelect
						id="j-stage"
						value={jStage}
						options={[
							{ value: '', label: t('japa_stage.stage_unset', 'Not set yet') },
							...jStageOpts
						]}
						onValueChange={(v) => (jStage = v as JapaStage | '')}
					/>
				</div>
			</div>

			<div class="flex flex-col gap-1.5">
				<label for="j-dest" class="text-xs font-medium text-[var(--color-text)]">
					{t('japa_stage.dest_country', 'Destination country')}
				</label>
				<Autocomplete
					placeholder={t('japa_stage.dest_placeholder', 'e.g. Canada')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinationCountries}
					initialFetcher={fetchDestinationCountriesInitial}
					selectedId={jDest || null}
					selectedLabel={selectedDestinationLabel}
					onSelect={(item) => (jDest = item.id)}
					onClear={() => (jDest = '')}
				/>
				<HintIcon text={t('journeys.dest_hint', 'Where you plan to move — drives guides and checklists.')} />
			</div>

			<div class="flex flex-col gap-1.5">
				<label for="j-label" class="text-xs font-medium text-[var(--color-text)]">
					{t('journeys.label', 'Label')}
					<span class="font-normal text-[var(--color-text-subtle)]">({t('common.optional', 'optional')})</span>
				</label>
				<input
					id="j-label"
					type="text"
					class={inputClass}
					bind:value={jLabel}
					placeholder={t('journeys.label_placeholder', 'e.g. Lagos business trip')}
					aria-describedby="j-label-hint"
					maxlength="120"
				/>
				<HintIcon id="j-label-hint" text={t('journeys.label_hint', 'A name for this journey — e.g. a move, a trip, or an application.')} />
			</div>

			<div class="grid grid-cols-2 gap-3">
				<div class="flex flex-col gap-1.5">
					<label for="j-start" class="text-xs font-medium text-[var(--color-text)]">
						{t('journeys.travel_start', 'Travel start')}
					</label>
					<input id="j-start" type="date" class={inputClass} bind:value={jStart} aria-describedby="j-start-hint" />
					<HintIcon id="j-start-hint" text={t('journeys.start_hint', 'When this trip or journey stage begins.')} />
				</div>
				<div class="flex flex-col gap-1.5">
					<label for="j-end" class="text-xs font-medium text-[var(--color-text)]">
						{t('journeys.travel_end', 'Travel end')}
					</label>
					<input id="j-end" type="date" class={inputClass} bind:value={jEnd} min={jStart || undefined} aria-describedby="j-end-hint" />
					<HintIcon id="j-end-hint" text={t('journeys.end_hint', 'Leave blank if the trip is ongoing.')} />
				</div>
			</div>

			<label class="flex items-center gap-2 text-xs text-[var(--color-text)]">
				<input type="checkbox" bind:checked={jPublic} class="h-4 w-4 rounded border-[var(--color-border)]" />
				{t('journeys.make_public', 'Show this journey on my public profile')}
			</label>

			{#if errorKey && showForm}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					{t(errorKey)}
				</p>
			{/if}

			<div class="flex justify-end gap-2">
				<button
					type="button"
					onclick={() => (showForm = false)}
					class="inline-flex items-center gap-1 rounded-[var(--radius)] px-3 py-1.5 text-xs font-medium text-[var(--color-muted)] transition hover:text-[var(--color-text)]"
				>
					<X size={14} /> {t('common.cancel', 'Cancel')}
				</button>
				<button
					type="submit"
					disabled={busy}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90 disabled:opacity-50"
				>
					{t('journeys.add', 'Add journey')}
				</button>
			</div>
		</form>
	{/if}

	{#if items.length > 0}
		<ol class="mt-4 space-y-2">
			{#each items as j (j.id)}
				<li class="rounded-[10px] border border-[var(--color-border)] p-3">
					<div class="flex items-start gap-2">
						<span
							class="mt-1 inline-block h-2 w-2 shrink-0 rounded-full"
							style={`background-color: ${japaStageColors[j.stage] ?? 'var(--color-muted)'};`}
						></span>
						<div class="min-w-0 flex-1">
							<div class="flex flex-wrap items-center gap-x-2 gap-y-1">
								<span class="text-sm font-medium text-[var(--color-text)]">
									{j.label?.trim() || japaIntentLabel(t, j.intent)}
								</span>
								{#if j.label?.trim()}
									<span class="text-xs text-[var(--color-text-subtle)]">({japaIntentLabel(t, j.intent)})</span>
								{/if}
								<span
									class="rounded-full border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide"
									style={`border-color: ${statusColors[j.status]}; color: ${statusColors[j.status]};`}
								>
									{t(`journeys.status_${j.status}`, j.status)}
								</span>
								{#if j.in_window && (j.status === 'active' || j.status === 'planned')}
									<span class="text-[10px] font-medium uppercase tracking-wide text-green-600">
										{t('journeys.in_window', 'in travel window')}
									</span>
								{/if}
							</div>
							<p class="mt-1 text-xs text-[var(--color-muted)]">
								{japaStageLabel(t, j.stage)}
								{#if destLabel(j)}
									· {destLabel(j)}
								{/if}
								{#if windowText(j)}
									· {windowText(j)}
								{/if}
							</p>
						</div>
						<div class="flex shrink-0 items-center gap-1">
							{#if j.status === 'planned'}
								<button
									type="button"
									onclick={() => transition(j, 'active')}
									class="rounded-[var(--radius)] px-2 py-1 text-xs font-medium text-[var(--color-primary)] transition hover:bg-[var(--color-primary)]/10"
								>
									{t('journeys.start', 'Start')}
								</button>
								<button
									type="button"
									onclick={() => transition(j, 'cancelled')}
									class="rounded-[var(--radius)] px-2 py-1 text-xs text-[var(--color-muted)] transition hover:text-[var(--color-text)]"
								>
									{t('journeys.cancel', 'Cancel')}
								</button>
							{:else if j.status === 'active'}
								<button
									type="button"
									onclick={() => transition(j, 'completed')}
									class="rounded-[var(--radius)] px-2 py-1 text-xs font-medium text-[var(--color-primary)] transition hover:bg-[var(--color-primary)]/10"
								>
									{t('journeys.complete', 'Complete')}
								</button>
								<button
									type="button"
									onclick={() => transition(j, 'cancelled')}
									class="rounded-[var(--radius)] px-2 py-1 text-xs text-[var(--color-muted)] transition hover:text-[var(--color-text)]"
								>
									{t('journeys.cancel', 'Cancel')}
								</button>
							{:else if j.status === 'cancelled'}
								<button
									type="button"
									onclick={() => transition(j, 'planned')}
									class="rounded-[var(--radius)] px-2 py-1 text-xs font-medium text-[var(--color-primary)] transition hover:bg-[var(--color-primary)]/10"
								>
									{t('journeys.reopen', 'Reopen')}
								</button>
							{/if}
							<button
								type="button"
								onclick={() => remove(j)}
								class="rounded-[var(--radius)] p-1.5 text-[var(--color-text-subtle)] transition hover:bg-red-500/10 hover:text-red-500"
								aria-label={t('journeys.delete', 'Delete journey')}
							>
								<Trash2 size={14} />
							</button>
						</div>
					</div>
					{#if j.updated_at}
						<p class="mt-1.5 ps-4 text-[10px] text-[var(--color-text-subtle)]">
							{t('japa_stage.updated', 'Updated')} <RelativeTime iso={j.updated_at} />
						</p>
					{/if}
				</li>
			{/each}
		</ol>
	{/if}

	{#if errorKey && !showForm}
		<p class="mt-3 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
			{t(errorKey)}
		</p>
	{/if}
</div>
