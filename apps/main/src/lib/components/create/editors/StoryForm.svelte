<script lang="ts">
	// Ref: japaconnect-engineering/sections/79-community-story-wall.md
	// Story submissions live in the /create composer (type selector → "Story")
	// but still POST /stories — the dedicated endpoint creates the contents
	// row plus story_meta and places the story in the moderation queue
	// (status=in_review) per §79. Draft/publish does not apply to stories.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';
	import { ArrowLeft, Send, Loader2, CheckCircle } from 'lucide-svelte';
	import type { Story, GeoCountry, SessionUser } from '$lib/types';

	interface Props {
		user?: SessionUser | null;
		features?: Record<string, boolean>;
	}

	let { user = null, features = {} }: Props = $props();

	let title = $state('');
	let body = $state('');
	let originSpaceId = $state('');
	let destinationSpaceId = $state('');
	let relocation_year = $state('');
	let visa_category = $state('');
	let profession = $state('');
	let biggest_challenge = $state('');
	let most_useful_resource = $state('');
	let best_advice = $state('');

	let countries = $state<GeoCountry[]>([]);
	let loading = $state(false);
	let errors = $state<Record<string, string>>({});
	let submitError = $state('');
	let submitted = $state(false);

	// Country pickers come from /geo/countries (Rule 16 — no hardcoded lists).
	$effect(() => {
		void (async () => {
			const res = await api.get<GeoCountry[]>('/geo/countries');
			if (res.data) countries = res.data;
		})();
	});

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
				origin_space_id: originSpaceId || undefined,
				destination_space_id: destinationSpaceId,
				relocation_year: parseInt(relocation_year, 10) || undefined,
				visa_category: visa_category.trim() || undefined,
				profession: profession.trim() || undefined,
				biggest_challenge: biggest_challenge.trim() || undefined,
				most_useful_resource: most_useful_resource.trim() || undefined,
				best_advice: best_advice.trim() || undefined
			};

			const res = await api.post<Story>('/stories', payload);
			if (res.data) {
				submitted = true;
			} else {
				submitError = res.error?.message ?? t('stories.errors.submit_failed', 'Failed to submit your story');
			}
		} catch {
			submitError = t('stories.errors.submit_failed', 'Failed to submit your story');
		} finally {
			loading = false;
		}
	}

	function fetchCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('common.all_countries', 'All countries');
		const query = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (query === '') {
			return Promise.resolve([allItem, ...countries.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(query);
		const filtered = countries.filter(
			(c) => c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
		);
		const items = filtered.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
		return Promise.resolve(matchesAll ? [allItem, ...items] : items);
	}
	function fetchCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchCountries('');
	}
	const selectedOriginLabel = $derived(
		originSpaceId ? countries.find((c) => c.id === originSpaceId)?.name ?? null : null
	);
	const selectedDestinationLabel = $derived(
		destinationSpaceId ? countries.find((c) => c.id === destinationSpaceId)?.name ?? null : null
	);
</script>

<div class="story-form">
	{#if submitted}
		<div class="rounded-[var(--radius)] border border-[var(--color-primary)]/30 bg-[var(--color-primary)]/5 p-4">
			<div class="flex items-start gap-3">
				<CheckCircle size={20} class="mt-0.5 shrink-0 text-[var(--color-primary)]" />
				<div class="min-w-0">
					<h2 class="text-sm font-semibold text-[var(--color-text)]">
						{t('stories.submitted_title', 'Story submitted for review')}
					</h2>
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
						{t('stories.submitted_body', 'Your story has been submitted and is pending moderation review. Once approved, it will appear on the public Story Wall.')}
					</p>
				</div>
			</div>
			<div class="mt-3 flex gap-2">
				<a
					href="/stories"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					<ArrowLeft size={14} /> {t('stories.back_to_wall', 'Back to stories')}
				</a>
			</div>
		</div>
	{:else}
		<form onsubmit={submit} class="story-form-fields" novalidate>
			<!-- Title -->
			<div class="flex flex-col gap-1.5">
				<label for="story-title" class="text-sm font-medium text-[var(--color-text)]">
					{t('stories.field_title', 'Title')} <span class="text-red-500">*</span>
				</label>
				<input id="story-title" type="text" bind:value={title} class={inputClass} placeholder={t('stories.field_title_ph', 'e.g. My journey from Lagos to London')} />
				{#if errors.title}<p class="text-xs text-red-500">{errors.title}</p>{/if}
			</div>

			<!-- Body -->
			<div class="flex flex-col gap-1.5">
				<AiBodyField
					id="story-body"
					label={t('stories.field_body', 'Your Story')}
					contentType="story"
					{title}
					{body}
					onBody={(v) => (body = v)}
					onTitle={(v) => (title = v)}
					{user}
					{features}
					required
					placeholder={t('stories.field_body_ph', 'Tell us about your relocation experience…')}
					rows={8}
				/>
				{#if errors.body}<p class="text-xs text-red-500">{errors.body}</p>{/if}
			</div>

			<!-- Origin / Destination -->
			<div class="grid gap-4 sm:grid-cols-2">
				<div class="flex flex-col gap-1.5">
					<label for="story-origin" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_origin', 'Origin Country')}</label>
					<Autocomplete
						placeholder={t('stories.field_origin_ph', 'e.g. Nigeria')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchCountries}
						initialFetcher={fetchCountriesInitial}
						selectedId={originSpaceId || null}
						selectedLabel={selectedOriginLabel}
						onSelect={(item) => (originSpaceId = item.id)}
						onClear={() => (originSpaceId = '')}
					/>
				</div>
				<div class="flex flex-col gap-1.5">
					<label for="story-dest" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_destination', 'Destination Country')}</label>
					<Autocomplete
						placeholder={t('stories.field_destination_ph', 'e.g. United Kingdom')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchCountries}
						initialFetcher={fetchCountriesInitial}
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
					<label for="story-year" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_year', 'Relocation Year')}</label>
					<input id="story-year" type="number" bind:value={relocation_year} class={inputClass} placeholder={t('stories.field_year_ph', '2020')} />
					{#if errors.relocation_year}<p class="text-xs text-red-500">{errors.relocation_year}</p>{/if}
				</div>
				<div class="flex flex-col gap-1.5">
					<label for="story-visa" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_visa', 'Visa Category')}</label>
					<input id="story-visa" type="text" bind:value={visa_category} class={inputClass} placeholder={t('stories.field_visa_ph', 'e.g. Skilled Worker')} />
				</div>
				<div class="flex flex-col gap-1.5">
					<label for="story-prof" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_profession', 'Profession')}</label>
					<input id="story-prof" type="text" bind:value={profession} class={inputClass} placeholder={t('stories.field_profession_ph', 'e.g. Software Engineer')} />
				</div>
			</div>

			<!-- Biggest challenge -->
			<div class="flex flex-col gap-1.5">
				<label for="story-challenge" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_challenge', 'Biggest Challenge')}</label>
				<textarea id="story-challenge" rows={4} bind:value={biggest_challenge} class={inputClass} placeholder={t('stories.field_challenge_ph', 'What was the hardest part of your journey?')}></textarea>
			</div>

			<!-- Most useful resource -->
			<div class="flex flex-col gap-1.5">
				<label for="story-resource" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_resource', 'Most Useful Resource')}</label>
				<input id="story-resource" type="text" bind:value={most_useful_resource} class={inputClass} placeholder={t('stories.field_resource_ph', 'A website, community, document, etc.')} />
			</div>

			<!-- Best advice -->
			<div class="flex flex-col gap-1.5">
				<label for="story-advice" class="text-sm font-medium text-[var(--color-text)]">{t('stories.field_advice', 'Best Advice')}</label>
				<textarea id="story-advice" rows={4} bind:value={best_advice} class={inputClass} placeholder={t('stories.field_advice_ph', 'What advice would you give to someone starting this journey?')}></textarea>
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
						{t('stories.submitting', 'Submitting…')}
					{:else}
						<Send size={16} />
						{t('stories.submit', 'Share Story')}
					{/if}
				</button>
			</div>
		</form>
	{/if}
</div>

<style>
	.story-form {
		width: 100%;
		max-width: 44rem;
		margin-inline: auto;
	}
	.story-form-fields {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		margin-top: 1rem;
	}
</style>
