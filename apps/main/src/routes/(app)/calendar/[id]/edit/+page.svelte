<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// Ref: japaconnect-engineering/sections/86-community-calendar.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { Calendar as CalendarIcon, Save, Loader2 } from 'lucide-svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { api } from '$lib/api/client';
	import type { CalendarCategory, CalendarEntry } from '$lib/types';
	import { destinationMarkets, originMarkets, pinMarketsFirst } from '$lib/utils/geoMarkets';
	import type { PageData } from './$types';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';

	let { data }: { data: PageData } = $props();

	const entry = $derived(data.entry);

	const pageTitle = $derived(t('calendar.edit_title', 'Edit Calendar Entry — {site}'));

	let title = $state(untrack(() => entry.title));
	let description = $state(untrack(() => entry.description ?? ''));
	const categoryValues = $derived(data.categories.map((c) => c.value as CalendarCategory));
	let category = $state<CalendarCategory>(
		untrack(() =>
			(categoryValues.includes(entry.category as CalendarCategory)
				? (entry.category as CalendarCategory)
				: (data.categories[0]?.value as CalendarCategory)) ?? 'other'
		)
	);
	let destinationSpaceId = $state(untrack(() => entry.destination_space_id ?? ''));
	let originSpaceId = $state(untrack(() => entry.origin_space_id ?? ''));
	let occursAt = $state(untrack(() => toLocalInput(entry.occurs_at)));
	let endsAt = $state(untrack(() => (entry.ends_at ? toLocalInput(entry.ends_at) : '')));
	let isRecurring = $state(untrack(() => entry.is_recurring));
	let recurrencePattern = $state(untrack(() => entry.recurrence_pattern ?? ''));
	let sourceURL = $state(untrack(() => entry.source_url ?? ''));

	let submitting = $state(false);
	let errorMsg = $state('');

	const categoryOptions = $derived(
		(data.categories ?? []).map((c) => ({ value: c.value as CalendarCategory, label: c.label }))
	);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	function toLocalInput(iso: string | undefined): string {
		if (!iso) return '';
		try {
			const d = new Date(iso);
			// Convert to a YYYY-MM-DDTHH:mm string in the local browser timezone.
			const pad = (n: number) => String(n).padStart(2, '0');
			return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
		} catch {
			return '';
		}
	}

	function fromLocalInput(value: string): string | undefined {
		if (!value) return undefined;
		try {
			return new Date(value).toISOString();
		} catch {
			return undefined;
		}
	}

	function validate(): boolean {
		errorMsg = '';
		if (!title.trim() || !occursAt || !sourceURL.trim()) {
			errorMsg = t('calendar.new.error_required', 'Title, start time, and a verifiable source URL are required.');
			return false;
		}
		if (!sourceURL.startsWith('http://') && !sourceURL.startsWith('https://')) {
			errorMsg = t('calendar.new.error_invalid_url', 'Please enter a valid source URL.');
			return false;
		}
		return true;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!validate()) return;

		const payload: Record<string, unknown> = {
			title: title.trim(),
			description: description.trim(),
			category,
			destination_space_id: destinationSpaceId,
			occurs_at: fromLocalInput(occursAt),
			source_url: sourceURL.trim(),
			is_recurring: isRecurring
		};
		if (originSpaceId) payload.origin_space_id = originSpaceId;
		if (endsAt) payload.ends_at = fromLocalInput(endsAt);
		if (recurrencePattern.trim()) payload.recurrence_pattern = recurrencePattern.trim();

		submitting = true;
		const res = await api.patch<CalendarEntry>(`/calendar/${entry.id}`, payload);
		submitting = false;

		if (res.data) {
			await goto(`/calendar/${res.data.id}`);
		} else {
			errorMsg = res.error?.message ?? t('calendar.new.error_failed', 'Failed to submit event. Please try again.');
		}
	}

	// ADR-064 curated markets (public settings) — pinned first in the
	// country pickers; every seeded country stays selectable.
	const destCountries = $derived(
		pinMarketsFirst(data.countries ?? [], destinationMarkets(data.siteSettings), (c) => c.geo_code)
	);
	const originCountries = $derived(
		pinMarketsFirst(data.countries ?? [], originMarkets(data.siteSettings), (c) => c.geo_code)
	);

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('common.all_countries', 'All countries');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...destCountries.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = destCountries.filter(
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
			return Promise.resolve([allItem, ...originCountries.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = originCountries.filter(
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
	<title>{pageTitle}</title>
	<meta name="description" content={t('calendar.edit_description', 'Update this community calendar event.')} />
	<meta name="robots" content="noindex" />
	<meta property="og:title" content={pageTitle} />
	<meta property="og:description" content={t('calendar.edit_description', 'Update this community calendar event.')} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-2xl pb-24">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<CalendarIcon size={20} /> {t('calendar.edit_heading', 'Edit Calendar Entry')}
		</h1>
	</div>

	<div class="p-4">
		<form onsubmit={submit} class="space-y-5 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6" novalidate>
			<!-- Title -->
			<div>
				<label for="title" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('calendar.new.label_title', 'Title')} <span class="text-red-500">*</span>
				
				<HintIcon text={t('calendar.new.title_hint', 'A short event name shown on the calendar.')}  />
</label>
				<input
					id="title"
					type="text"
					bind:value={title}
					placeholder={t('calendar.new.placeholder_title', 'e.g. UK visa application window opens')}
					class={inputClass}
				/>
			</div>

			<!-- Description -->
			<AiBodyField
				id="description"
				label={t('calendar.new.label_description', 'Description')}
				contentType="calendar"
				{title}
				body={description}
				onBody={(v) => (description = v)}
				user={data.user}
				features={data.features ?? {}}
				placeholder={t('calendar.new.placeholder_description', 'Add details about this event…')}
				rows={5}
			/>

			<!-- Category -->
			<div>
				<label for="category" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('calendar.new.label_category', 'Category')}
				
				<HintIcon text={t('calendar.new.category_hint', 'What kind of event — deadline, window, meetup.')}  />
</label>
				<AutocompleteSelect
					id="category"
					value={category}
					options={categoryOptions}
					onValueChange={(v) => (category = v as CalendarCategory)}
				/>
			</div>

			<!-- Destination + Origin -->
			<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
				<div>
					<label for="dest" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('calendar.new.label_destination', 'Destination country')}
					
					<HintIcon text={t('calendar.new.destination_hint', 'The country this event applies to.')}  />

					<HintIcon text={t('calendar.new.destination_hint', 'The country this event applies to.')}  /></label>)}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchDestinationCountries}
						initialFetcher={fetchDestinationCountriesInitial}
						selectedId={destinationSpaceId || null}
						selectedLabel={selectedDestinationLabel}
						onSelect={(item) => (destinationSpaceId = item.id)}
						onClear={() => (destinationSpaceId = '')}
					/>
				</div>
				<div>
					<label for="origin" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('calendar.new.label_origin', 'Origin country (optional)')}
					
					<HintIcon text={t('calendar.new.origin_hint', 'Optional — restrict to a specific origin country.')}  />
</label>
					<Autocomplete
						placeholder={t('calendar.new.placeholder_origin', 'Leave blank for all origins')}
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

			<!-- Start + End -->
			<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
				<div>
					<label for="occurs_at" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('calendar.new.label_occurs_at', 'Starts')} <span class="text-red-500">*</span>
					
					<HintIcon text={t('calendar.new.occurs_hint', 'When the event starts (your local time).')}  />
</label>
					<input
						id="occurs_at"
						type="datetime-local"
						bind:value={occursAt}
						class={inputClass}
					/>
				</div>
				<div>
					<label for="ends_at" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('calendar.new.label_ends_at', 'Ends (optional)')}
					
					<HintIcon text={t('calendar.new.ends_hint', 'Optional — when the event ends.')}  />

					<HintIcon text={t('calendar.new.ends_hint', 'Optional — when the event ends.')}  /></label>
					<inpinputClass}
					/>
				</div>
			</div>

			<!-- Recurring -->
			<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
				<input
					type="checkbox"
					checked={isRecurring}
					onchange={(e) => (isRecurring = e.currentTarget.checked)}
					class="h-4 w-4 rounded border-[var(--color-border)]"
				/>
				{t('calendar.new.label_recurring', 'Recurring event')}
			</label>

			{#if isRecurring}
				<div>
					<label for="recurrence_pattern" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('calendar.new.label_recurrence_pattern', 'Recurrence pattern')}
					
					<HintIcon text={t('calendar.new.recurrence_hint', 'How often it repeats — e.g. annual, monthly.')}  />
</label>
					<input
						id="recurrence_pattern"
						type="text"
						bind:value={recurrencePattern}
						placeholder={t('calendar.new.placeholder_recurrence', 'e.g. annual')}
						class={inputClass}
					/>
				</div>
			{/if}

			<!-- Source URL -->
			<div>
				<label for="source_url" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('calendar.new.label_source', 'Official source URL')} <span class="text-red-500">*</span>
				
				<HintIcon text={t('calendar.new.source_hint', 'Link to the official announcement — required for verification.')}  />
</label>
				<input
					id="source_url"
					type="url"
					bind:value={sourceURL}
					placeholder={t('calendar.new.placeholder_source', 'https://official-source.example/…')}
					class={inputClass}
				/>
				<p class="mt-1 text-xs text-[var(--color-muted)]">{t('calendar.new.source_help', 'Must link to a verifiable official source.')}</p>
			</div>

			<!-- Error -->
			{#if errorMsg}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">{errorMsg}</p>
			{/if}

			<!-- Submit -->
			<div class="flex justify-end">
				<button
					type="submit"
					disabled={submitting}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					{#if submitting}
						<Loader2 size={16} class="animate-spin" />
					{:else}
						<Save size={16} />
					{/if}
					{submitting ? t('common.saving', 'Saving…') : t('common.save', 'Save')}
				</button>
			</div>
		</form>
	</div>
</div>
