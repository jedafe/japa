<script lang="ts">
	// Ref: japaconnect-engineering/sections/79-community-story-wall.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { ArrowLeft, Send, Loader2 } from 'lucide-svelte';
	import type { Story } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// Form fields are seeded once from the server-loaded story. Using untrack()
	// signals that capturing the initial value is intentional — the form owns
	// its own state after that and should not reactively reset on navigation.
	let title = $state(untrack(() => data.story.title));
	let body = $state(untrack(() => data.story.body));
	let originSpaceId = $state(untrack(() => data.story.origin_space_id ?? ''));
	let destinationSpaceId = $state(untrack(() => data.story.destination_space_id ?? ''));
	let relocation_year = $state(untrack(() => data.story.relocation_year ? String(data.story.relocation_year) : ''));
	let visa_category = $state(untrack(() => data.story.visa_category ?? ''));
	let profession = $state(untrack(() => data.story.profession ?? ''));
	let biggest_challenge = $state(untrack(() => data.story.biggest_challenge ?? ''));
	let most_useful_resource = $state(untrack(() => data.story.most_useful_resource ?? ''));
	let best_advice = $state(untrack(() => data.story.best_advice ?? ''));

	let loading = $state(false);
	let errors = $state<Record<string, string>>({});
	let submitError = $state('');

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	function validate(): boolean {
		errors = {};
		if (!title.trim()) errors.title = t('stories.errors.title_required', 'Title is required');
		if (!body.trim()) errors.body = t('stories.errors.body_required', 'Please share your story');
		if (!destinationSpaceId)
			errors.destination_space_id = t('stories.errors.destination_required', 'Destination country is required');
		if (relocation_year && isNaN(parseInt(relocation_year, 10)))
			errors.relocation_year = t('stories.errors.year_invalid', 'Enter a valid year');
		return Object.keys(errors).length === 0;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		submitError = '';
		if (!validate()) return;
		loading = true;
		try {
			const payload: Record<string, unknown> = {
				title: title.trim(),
				body: body.trim(),
				destination_space_id: destinationSpaceId,
			};
			if (originSpaceId) payload.origin_space_id = originSpaceId;
			if (relocation_year) payload.relocation_year = parseInt(relocation_year, 10);
			if (visa_category.trim()) payload.visa_category = visa_category.trim();
			if (profession.trim()) payload.profession = profession.trim();
			if (biggest_challenge.trim()) payload.biggest_challenge = biggest_challenge.trim();
			if (most_useful_resource.trim()) payload.most_useful_resource = most_useful_resource.trim();
			if (best_advice.trim()) payload.best_advice = best_advice.trim();

			const res = await api.patch<Story>(`/stories/${data.story.id}`, payload);
			if (res.error === null) {
				goto(`/stories/${data.story.slug}`);
			} else {
				submitError = res.error?.message ?? t('stories.errors.submit_failed', 'Failed to save your story');
			}
		} catch {
			submitError = t('stories.errors.submit_failed', 'Failed to save your story');
		} finally {
			loading = false;
		}
	}

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
</script>

<svelte:head>
	<title>{t('stories.edit_title', 'Edit Story — JapaConnect')}</title>
	<meta name="description" content={t('stories.edit_description', 'Edit your relocation story.')} />
</svelte:head>

<div class="mx-auto w-full max-w-2xl">
	<div class="p-4">
		<a
			href="/stories/{data.story.slug}"
			class="inline-flex items-center gap-1 text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={16} /> {t('stories.back_to_story', 'Back to story')}
		</a>
	</div>

	<div class="px-4 pb-8">
		<h1 class="text-2xl font-bold text-[var(--color-text)]">{t('stories.edit_heading', 'Edit Your Story')}</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">
			{t('stories.edit_subheading', 'Update your relocation journey.')}
		</p>

		<form onsubmit={submit} class="mt-6 space-y-4" novalidate>
			<!-- Title -->
			<div class="flex flex-col gap-1.5">
				<label for="f-title" class="text-sm font-medium text-[var(--color-text)]">
					{t('stories.field_title', 'Title')} <span class="text-red-500">*</span>
				</label>
				<input id="f-title" type="text" bind:value={title} class={inputClass} placeholder={t('stories.field_title_ph', 'e.g. My journey from Lagos to London')} />
				{#if errors.title}<p class="text-xs text-red-500">{errors.title}</p>{/if}
			</div>

			<!-- Body -->
			<div class="flex flex-col gap-1.5">
				<label for="f-body" class="text-sm font-medium text-[var(--color-text)]">
					{t('stories.field_body', 'Your Story')} <span class="text-red-500">*</span>
				</label>
				<textarea id="f-body" rows={8} bind:value={body} class={inputClass} placeholder={t('stories.field_body_ph', 'Tell us about your relocation experience…')}></textarea>
				{#if errors.body}<p class="text-xs text-red-500">{errors.body}</p>{/if}
			</div>

			<!-- Origin / Destination -->
			<div class="grid gap-4 sm:grid-cols-2">
				<div class="flex flex-col gap-1.5">
					<label for="f-origin" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_origin', 'Origin Country')}</label>
					<Autocomplete
						placeholder={t('stories.field_origin_ph', 'e.g. Nigeria')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchOriginCountries}
						initialFetcher={fetchOriginCountriesInitial}
						selectedId={originSpaceId || null}
						selectedLabel={selectedOriginLabel}
						onSelect={(item) => (originSpaceId = item.id)}
						onClear={() => (originSpaceId = '')}
					/>
				</div>
				<div class="flex flex-col gap-1.5">
					<label for="f-dest" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_destination', 'Destination Country')}</label>
					<Autocomplete
						placeholder={t('stories.field_destination_ph', 'e.g. United Kingdom')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchDestinationCountries}
						initialFetcher={fetchDestinationCountriesInitial}
						selectedId={destinationSpaceId || null}
						selectedLabel={selectedDestinationLabel}
						onSelect={(item) => (destinationSpaceId = item.id)}
						onClear={() => (destinationSpaceId = '')}
					/>
					{#if errors.destination_space_id}<p class="text-xs text-red-500">{errors.destination_space_id}</p>{/if}
				</div>
			</div>

			<!-- Year / Visa / Profession -->
			<div class="grid gap-4 sm:grid-cols-3">
				<div class="flex flex-col gap-1.5">
					<label for="f-year" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_year', 'Relocation Year')}</label>
					<input id="f-year" type="number" bind:value={relocation_year} class={inputClass} placeholder="2020" />
					{#if errors.relocation_year}<p class="text-xs text-red-500">{errors.relocation_year}</p>{/if}
				</div>
				<div class="flex flex-col gap-1.5">
					<label for="f-visa" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_visa', 'Visa Category')}</label>
					<input id="f-visa" type="text" bind:value={visa_category} class={inputClass} placeholder={t('stories.field_visa_ph', 'e.g. Skilled Worker')} />
				</div>
				<div class="flex flex-col gap-1.5">
					<label for="f-prof" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_profession', 'Profession')}</label>
					<input id="f-prof" type="text" bind:value={profession} class={inputClass} placeholder={t('stories.field_profession_ph', 'e.g. Software Engineer')} />
				</div>
			</div>

			<!-- Biggest challenge -->
			<div class="flex flex-col gap-1.5">
				<label for="f-challenge" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_challenge', 'Biggest Challenge')}</label>
				<textarea id="f-challenge" rows={4} bind:value={biggest_challenge} class={inputClass} placeholder={t('stories.field_challenge_ph', 'What was the hardest part of your journey?')}></textarea>
			</div>

			<!-- Most useful resource -->
			<div class="flex flex-col gap-1.5">
				<label for="f-resource" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_resource', 'Most Useful Resource')}</label>
				<input id="f-resource" type="text" bind:value={most_useful_resource} class={inputClass} placeholder={t('stories.field_resource_ph', 'A website, community, document, etc.')} />
			</div>

			<!-- Best advice -->
			<div class="flex flex-col gap-1.5">
				<label for="f-advice" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_advice', 'Best Advice')}</label>
				<textarea id="f-advice" rows={4} bind:value={best_advice} class={inputClass} placeholder={t('stories.field_advice_ph', 'What advice would you give to someone starting this journey?')}></textarea>
			</div>

			{#if submitError}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">{submitError}</p>
			{/if}

			<div class="flex justify-end pt-2">
				<button
					type="submit"
					disabled={loading}
					class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:opacity-90 disabled:opacity-50"
				>
					{#if loading}
						<Loader2 size={16} class="animate-spin" />
						{t('stories.saving', 'Saving…')}
					{:else}
						<Send size={16} />
						{t('stories.save', 'Save Changes')}
					{/if}
				</button>
			</div>
		</form>
	</div>
</div>
