<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// Ref: japaconnect-engineering/sections/86-community-calendar.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { ArrowLeft, Send, Loader2, Lock } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import type { CalendarCategory } from '$lib/types';
	import { destinationMarkets, originMarkets, pinMarketsFirst } from '$lib/utils/geoMarkets';
	import type { PageData } from './$types';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';

	let { data }: { data: PageData } = $props();

	// Auth is required to submit. The app layout exposes `user` in page.data.
	const authed = $derived(!!page.data.user);

	// BUG-009: categories come from the API (calendar.categories system_setting),
	// not a hardcoded array (Rule 16). The server load fetches GET /calendar/categories.
	const categories: { value: CalendarCategory; label: string }[] = $derived(
		(data.categories ?? []).map((c: { value: string; label: string }) => ({
			value: c.value as CalendarCategory,
			label: c.label
		}))
	);

	let title = $state('');
	let description = $state('');
	let category = $state<CalendarCategory>(
		untrack(() => (data.categories?.[0]?.value as CalendarCategory) ?? 'other')
	);
	let destinationSpaceId = $state('');
	let originSpaceId = $state('');
	let occursAt = $state('');
	let endsAt = $state('');
	let isRecurring = $state(false);
	let recurrencePattern = $state('');
	let sourceUrl = $state('');
	let submitting = $state(false);
	let errorMsg = $state('');

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	function toIso(value: string): string | null {
		if (!value) return null;
		// datetime-local values are already ISO-ish (no timezone); send as-is.
		const d = new Date(value);
		return isNaN(d.getTime()) ? null : d.toISOString();
	}

	const canSubmit = $derived(
		title.trim().length > 0 &&
			!!sourceUrl.trim() &&
			!!occursAt &&
			destinationSpaceId.length > 0 &&
			!submitting
	);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorMsg = '';
		if (!authed) {
			goto('/login?redirect=/calendar/new');
			return;
		}
		if (!canSubmit) {
			errorMsg = t('calendar.new.error_required', 'Title, start time, and a verifiable source URL are required.');
			return;
		}
		// source_url is REQUIRED and must be a verifiable official source.
		// BUG-011: validate scheme is http/https to prevent javascript: XSS.
		try {
			const parsed = new URL(sourceUrl.trim());
			if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
				errorMsg = t('calendar.new.error_invalid_url', 'Please enter a valid source URL.');
				return;
			}
		} catch {
			errorMsg = t('calendar.new.error_invalid_url', 'Please enter a valid source URL.');
			return;
		}

		submitting = true;
		try {
			const body = {
				title: title.trim(),
				description: description.trim() || undefined,
				category,
				destination_space_id: destinationSpaceId,
				origin_space_id: originSpaceId || undefined,
				occurs_at: toIso(occursAt),
				ends_at: endsAt ? toIso(endsAt) : undefined,
				is_recurring: isRecurring,
				recurrence_pattern: isRecurring ? recurrencePattern.trim() || undefined : undefined,
				source_url: sourceUrl.trim()
			};
			const res = await api.post<{ id: string }>('/calendar', body);
			if (res.data && res.data.id) {
				await goto(`/calendar/${res.data.id}`);
			} else {
				errorMsg = res.error?.message ?? t('calendar.new.error_failed', 'Failed to submit event. Please try again.');
			}
		} catch {
			errorMsg = t('calendar.new.error_failed', 'Failed to submit event. Please try again.');
		} finally {
			submitting = false;
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
	<title>{t('calendar.new_title', 'Submit Event — {site}')}</title>
	<meta name="description" content={t('calendar.new_description', 'Submit a community calendar event with a verifiable official source.')} />
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-2xl pb-24">
	<div class="mx-4 flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/calendar"
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('calendar.back', 'Back to calendar')}
		>
			<ArrowLeft class="rtl:-scale-x-100" size={18} />
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">{t('calendar.new_heading', 'Submit Event')}</h1>
	</div>

	{#if !authed}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('calendar.new.login_required', 'Login required')}
				body={t('calendar.new.login_required_body', 'You must be signed in to submit a calendar event.')}
				action={{ label: t('auth.sign_in', 'Sign in'), href: '/login?redirect=/calendar/new' }}
			/>
		</div>
	{:else}
		<form onsubmit={submit} class="space-y-5 p-4" novalidate>
			{#if errorMsg}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					{errorMsg}
				</p>
			{/if}

			<!-- BUG-004: Destination / Origin FIRST per geo-aware tool pattern
			     (§86 Geo Awareness — destination country is the primary data
			     axis and must be the first UI input). -->
			<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
				<div>
					<label for="destination_country" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">
						{t('calendar.new.label_destination', 'Destination country')} <span class="text-red-500" aria-hidden="true">*</span>
					
					<HintIcon text={t('calendar.new.destination_hint', 'The country this event applies to.')}  />
</label>
					<Autocomplete
						placeholder={t('calendar.new.placeholder_destination', 'Select a country')}
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
					<label for="origin_country" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">
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

			<!-- Title -->
			<div>
				<label for="title" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">
					{t('calendar.new.label_title', 'Title')} <span class="text-red-500" aria-hidden="true">*</span>
				
				<HintIcon text={t('calendar.new.title_hint', 'A short event name shown on the calendar.')}  />
</label>
				<input
					id="title"
					type="text"
					bind:value={title}
					maxlength="160"
					placeholder={t('calendar.new.placeholder_title', 'e.g. UK visa application window opens')}
					class={inputClass}
				/>
			</div>

			<!-- Category -->
			<div>
				<label for="category" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">
					{t('calendar.new.label_category', 'Category')}
				
				<HintIcon text={t('calendar.new.category_hint', 'What kind of event — deadline, window, meetup.')}  />
</label>
				<AutocompleteSelect
					id="category"
					value={category}
					options={categories}
					onValueChange={(v) => (category = v as CalendarCategory)}
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
				onTitle={(v) => (title = v)}
				user={data.user}
				features={data.features ?? {}}
				placeholder={t('calendar.new.placeholder_description', 'Add details about this event…')}
				rows={5}
			/>
				<HintIcon text={t('calendar.new.desc_hint', 'What people need to know — links and deadlines help.')} />

			<!-- Dates -->
			<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
				<div>
					<label for="occurs_at" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">
						{t('calendar.new.label_occurs_at', 'Starts')} <span class="text-red-500" aria-hidden="true">*</span>
					
					<HintIcon text={t('calendar.new.occurs_hint', 'When the event starts (your local time).')}  />
</label>
					<input id="occurs_at" type="datetime-local" bind:value={occursAt} class={inputClass} />
				</div>
				<div>
					<label for="ends_at" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">
						{t('calendar.new.label_ends_at', 'Ends (optional)')}
					
					<HintIcon text={t('calendar.new.ends_hint', 'Optional — when the event ends.')}  />
</label>
					<input id="ends_at" type="datetime-local" bind:value={endsAt} class={inputClass} />
				</div>
			</div>

			<!-- Recurrence -->
			<div class="space-y-2">
				<label class="flex cursor-pointer items-center gap-2 text-sm text-[var(--color-text)]">
					<input type="checkbox" bind:checked={isRecurring} class="h-4 w-4 accent-[var(--color-primary)]" />
					{t('calendar.new.label_recurring', 'Recurring event')}
				</label>
				{#if isRecurring}
					<input
						type="text"
						bind:value={recurrencePattern}
						placeholder={t('calendar.new.placeholder_recurrence', 'e.g. annual')}
						class={inputClass}
					/>
						<HintIcon text={t('calendar.new.recurrence_hint', 'How often it repeats — e.g. annual, monthly.')} />
				{/if}
			</div>

			<!-- Source URL (REQUIRED) -->
			<div>
				<label for="source_url" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">
					{t('calendar.new.label_source', 'Official source URL')} <span class="text-red-500" aria-hidden="true">*</span>
				
				<HintIcon text={t('calendar.new.source_hint', 'Link to the official announcement — required for verification.')}  />
</label>
				<input
					id="source_url"
					type="url"
					bind:value={sourceUrl}
					placeholder={t('calendar.new.placeholder_source', 'https://official-source.example/…')}
					class={inputClass}
				/>
				<p class="mt-1 text-xs text-[var(--color-muted)]">
					{t('calendar.new.source_help', 'Must link to a verifiable official source.')}
				</p>
			</div>

			<!-- Submit -->
			<div class="flex items-center justify-end gap-2 border-t border-[var(--color-border)] pt-4">
				<a
					href="/calendar"
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					{t('common.cancel', 'Cancel')}
				</a>
				<button
					type="submit"
					disabled={!canSubmit}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					{#if submitting}
						<Loader2 size={16} class="animate-spin" />
					{:else}
						<Send class="rtl:-scale-x-100" size={16} />
					{/if}
					{t('calendar.new.submit', 'Submit event')}
				</button>
			</div>
		</form>
	{/if}
</div>
