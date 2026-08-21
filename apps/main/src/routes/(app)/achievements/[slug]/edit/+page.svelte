<script lang="ts">
	// Ref: japaconnect-engineering/sections/89-achievement-feed.md
	// Edit achievement form. Pre-fills from the SSR-loaded achievement and
	// submits PATCH /achievements/{id} (by content id) via the browser API
	// client. On success navigates to the achievement's detail page.
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { Trophy, Loader2, Save } from 'lucide-svelte';
	import type { Achievement, AchievementMilestoneType } from '$lib/types';
	import { milestoneOptions } from '$lib/achievement';
	import type { PageData } from './$types';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';

	let { data }: { data: PageData } = $props();

	const a = $derived(data.achievement);

	let title = $state(untrack(() => a.title));
	let body = $state(untrack(() => a.body));
	let milestoneType = $state<AchievementMilestoneType>(untrack(() => (a.milestone_type as AchievementMilestoneType) ?? 'visa_approved'));
	let destinationSpaceId = $state(untrack(() => a.destination_space_id ?? ''));
	let originSpaceId = $state(untrack(() => a.origin_space_id ?? ''));
	let isAnonymous = $state(untrack(() => a.is_anonymous));
	let loading = $state(false);
	let errorKey = $state('');

	const pageTitle = $derived(t('achievements.edit_title', 'Edit Achievement — JapaConnect'));

	const milestoneOpts = $derived(milestoneOptions(t));

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!title.trim()) {
			errorKey = 'achievements.errors.title_required';
			return;
		}
		if (!body.trim()) {
			errorKey = 'achievements.errors.body_required';
			return;
		}
		if (!destinationSpaceId) {
			errorKey = 'achievements.errors.destination_required';
			return;
		}
		loading = true;
		try {
			const res = await api.patch<Achievement>(`/achievements/${a.id}`, {
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
				errorKey = 'achievements.errors.edit_failed';
			}
		} catch {
			errorKey = 'achievements.errors.edit_failed';
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
	<title>{pageTitle}</title>
	<meta name="description" content={pageTitle} />
	<meta property="og:title" content={pageTitle} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-2xl px-4 py-6">
	<div class="mb-5 flex items-center gap-2">
		<Trophy size={20} class="text-[var(--color-primary)]" />
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{t('achievements.edit_heading', 'Edit Achievement')}
		</h1>
	</div>

	<form
		onsubmit={submit}
		class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-6"
		novalidate
	>
		<div class="flex flex-col gap-1.5">
			<label for="title" class="text-sm font-medium text-[var(--color-text)]">
				{t('achievements.field_title', 'Title')}
				<span class="text-red-500" aria-hidden="true">*</span>
			</label>
			<input
				id="title"
				type="text"
				value={title}
				oninput={(e) => (title = e.currentTarget.value)}
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
			placeholder={t('achievements.body_placeholder', 'Share your journey…')}
			rows={6}
		/>

		<div class="flex flex-col gap-1.5">
			<label for="milestone_type" class="text-sm font-medium text-[var(--color-text)]">
				{t('achievements.field_milestone_type', 'Milestone type')}
				<span class="text-red-500" aria-hidden="true">*</span>
			</label>
			<select
				id="milestone_type"
				value={milestoneType}
				onchange={(e) => (milestoneType = e.currentTarget.value as AchievementMilestoneType)}
				class={inputClass}
			>
				{#each milestoneOpts as opt (opt.value)}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</div>

		<div class="grid gap-4 sm:grid-cols-2">
			<div class="flex flex-col gap-1.5">
				<label for="destination_country" class="text-sm font-medium text-[var(--color-text)]">
					{t('achievements.field_destination', 'Destination country')}
					<span class="text-red-500" aria-hidden="true">*</span>
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

		<label class="flex cursor-pointer items-center gap-2 text-sm text-[var(--color-text-subtle)]">
			<input
				type="checkbox"
				checked={isAnonymous}
				onchange={(e) => (isAnonymous = e.currentTarget.checked)}
				class="h-4 w-4 accent-[var(--color-primary)]"
			/>
			{t('achievements.field_anonymous', 'Post anonymously')}
		</label>

		{#if errorKey}
			<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
				{t(errorKey)}
			</p>
		{/if}

		<div class="flex justify-end gap-2 pt-2">
			<a
				href={`/achievements/${a.slug}`}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{t('common.cancel', 'Cancel')}
			</a>
			<button
				type="submit"
				disabled={loading}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				{#if loading}
					<Loader2 size={16} class="animate-spin" />
				{:else}
					<Save size={16} />
				{/if}
				{loading ? t('common.saving', 'Saving…') : t('common.save', 'Save')}
			</button>
		</div>
	</form>
</div>
