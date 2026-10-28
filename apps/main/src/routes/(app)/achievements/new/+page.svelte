<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// Ref: japaconnect-engineering/sections/89-achievement-feed.md
	// Create achievement form. Submits via the browser API client (POST /achievements,
	// auth required). On success navigates to the new achievement's detail page.
	// The milestone_type select options mirror the schema-defined enum (Rule 16).
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { Trophy, Plus, Loader2 } from 'lucide-svelte';
	import type { Achievement, AchievementMilestoneType } from '$lib/types';
	import { milestoneOptions } from '$lib/achievement';
	import { destinationMarkets, originMarkets, pinMarketsFirst } from '$lib/utils/geoMarkets';
	import type { PageData } from './$types';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';

	let { data }: { data: PageData } = $props();

	let title = $state('');
	let body = $state('');
	let milestoneType = $state<AchievementMilestoneType>('visa_approved');
	let destinationSpaceId = $state('');
	let originSpaceId = $state('');
	let isAnonymous = $state(false);
	let loading = $state(false);
	let errorKey = $state('');

	const pageTitle = $derived(t('achievements.new_title', 'Share an Achievement — {site}'));
	const pageDescription = $derived(
		t('achievements.new_description', 'Share a relocation milestone with the community.')
	);

	const milestoneOpts = $derived(milestoneOptions(t));

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	function validate(): boolean {
		if (!title.trim()) {
			errorKey = 'achievements.errors.title_required';
			return false;
		}
		if (!body.trim()) {
			errorKey = 'achievements.errors.body_required';
			return false;
		}
		if (!destinationSpaceId) {
			errorKey = 'achievements.errors.destination_required';
			return false;
		}
		return true;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!validate()) return;
		loading = true;
		try {
			const res = await api.post<Achievement>('/achievements', {
				title: title.trim(),
				body: body.trim(),
				milestone_type: milestoneType,
				destination_space_id: destinationSpaceId,
				origin_space_id: originSpaceId || undefined,
				is_anonymous: isAnonymous
			});
			if (res.data?.slug) {
				await goto(`/achievements/${res.data.slug}`);
			} else {
				errorKey = 'achievements.errors.submit_failed';
			}
		} catch {
			errorKey = 'achievements.errors.submit_failed';
		} finally {
			loading = false;
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
	<meta name="description" content={pageDescription} />
	<meta property="og:title" content={pageTitle} />
	<meta property="og:description" content={pageDescription} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="mx-auto w-full max-w-2xl px-4 py-6">
	<!-- Header -->
	<div class="mb-5 flex items-center gap-2">
		<Trophy size={20} class="text-[var(--color-primary)]" />
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{t('achievements.new_heading', 'Share an Achievement')}
		</h1>
	</div>
	<p class="mb-5 text-sm text-[var(--color-text-subtle)]">
		{t('achievements.new_subtitle', 'Celebrate a relocation milestone with the community.')}
	</p>

	<form
		onsubmit={submit}
		class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-6"
		novalidate
	>
		<!-- Geo-aware tool pattern: destination country selector is the first
		     input (§89 Geo Awareness, geo-aware-tool-pattern.md). -->
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
			<div class="flex flex-col gap-1.5">
				<label for="destination_country" class="text-sm font-medium text-[var(--color-text)]">
					{t('achievements.field_destination', 'Destination country')}
					<span class="text-red-500" aria-hidden="true">*</span>
				
				<HintIcon text={t('achievements.destination_hint', 'The country this achievement is about.')}  />
</label>
				<Autocomplete
					placeholder={t('achievements.destination_placeholder', 'e.g. Canada')}
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
				<label for="origin_country" class="text-sm font-medium text-[var(--color-text)]">
					{t('achievements.field_origin', 'Origin country')}
				
				<HintIcon text={t('achievements.origin_hint', 'Where you started from.')}  />
</label>
				<Autocomplete
					placeholder={t('achievements.origin_placeholder', 'e.g. Nigeria')}
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

		<div class="flex flex-col gap-1.5">
			<label for="title" class="text-sm font-medium text-[var(--color-text)]">
				{t('achievements.field_title', 'Title')}
				<span class="text-red-500" aria-hidden="true">*</span>
			
			<HintIcon text={t('achievements.title_hint', 'A short, celebratory headline.')}  />
</label>
			<input
				id="title"
				type="text"
				value={title}
				oninput={(e) => (title = e.currentTarget.value)}
				placeholder={t('achievements.title_placeholder', 'e.g. Visa approved!')}
				class={inputClass}
			/>
		</div>

		<AiBodyField
			id="body"
			label={t('achievements.field_body', 'Story')}
			contentType="achievement"
			{title}
			body={body}
			onBody={(v) => (body = v)}
			onTitle={(v) => (title = v)}
			user={data.user}
			features={data.features ?? {}}
			required
			placeholder={t('achievements.body_placeholder', 'Share your journey…')}
			rows={6}
		/>

		<div class="flex flex-col gap-1.5">
			<label for="milestone_type" class="text-sm font-medium text-[var(--color-text)]">
				{t('achievements.field_milestone_type', 'Milestone type')}
				<span class="text-red-500" aria-hidden="true">*</span>
			
			<HintIcon text={t('achievements.milestone_hint', 'The kind of milestone — e.g. visa approved, landed.')}  />
</label>
			<AutocompleteSelect
				id="milestone_type"
				value={milestoneType}
				options={milestoneOpts}
				onValueChange={(v) => (milestoneType = v as AchievementMilestoneType)}
			/>
		</div>

		<label class="flex cursor-pointer items-center gap-2 text-sm text-[var(--color-text-subtle)]">
			<input
				type="checkbox"
				checked={isAnonymous}
				onchange={(e) => (isAnonymous = e.currentTarget.checked)}
				class="h-4 w-4 accent-[var(--color-primary)]"
			/>
			{t('achievements.field_anonymous', 'Post anonymously')}
		
		<HintIcon text={t('achievements.anonymous_hint', 'Shows as "Anonymous member from [origin] → [destination]".')}  />
</label>

		{#if errorKey}
			<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
				{t(errorKey)}
			</p>
		{/if}

		<div class="flex justify-end pt-2">
			<button
				type="submit"
				disabled={loading}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				{#if loading}
					<Loader2 size={16} class="animate-spin" />
				{:else}
					<Plus size={16} />
				{/if}
				{loading ? t('common.submitting', 'Submitting…') : t('achievements.submit', 'Share Achievement')}
			</button>
		</div>
	</form>
</div>
