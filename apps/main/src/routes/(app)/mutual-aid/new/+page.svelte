<script lang="ts">
	// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { ArrowLeft, Send } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { mutualAidPostTypeOptions, mutualAidCategoryOptions } from '$lib/enumOptions';
	import type { MutualAidPost, MutualAidPostType, MutualAidCategory, Space } from '$lib/types';
	import type { PageData } from './$types';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';

	let { data }: { data: PageData } = $props();

	// Auth guard — the root layout exposes isAuthed (never overridden by page
	// loads). Redirect unauthenticated users to login with a return path.
	const authed = $derived(data.isAuthed);

	const postTypeOptions = $derived(mutualAidPostTypeOptions(t, false, data.postTypes));
	const categoryOptions = $derived(mutualAidCategoryOptions(t, false, data.categories));

	// Pre-select post_type from the ?type= query param (e.g. /mutual-aid/new?type=offer).
	const initialType = (() => {
		const q = page.url.searchParams.get('type');
		if (data.postTypes.includes(q as MutualAidPostType)) return q as MutualAidPostType;
		return data.postTypes[0] ?? 'need';
	})();

	let postType = $state<MutualAidPostType>(untrack(() => initialType));
	let category = $state<MutualAidCategory>(untrack(() => data.categories[0] ?? 'housing'));
	let title = $state('');
	let description = $state('');
	let destinationSpaceId = $state('');
	let citySpaceId = $state('');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	// City list — loaded when a destination country is selected (BUG-011).
	// The spaces API supports ?level=city&parent=<country_id> to fetch
	// city-level spaces under a given country.
	let cities = $state<Space[]>([]);
	let loadingCities = $state(false);

	// When the destination changes, fetch the city list for that country.
	$effect(() => {
		const destId = destinationSpaceId;
		if (!destId) {
			cities = [];
			citySpaceId = '';
			return;
		}
		// Reset the selected city when the destination changes.
		citySpaceId = '';
		loadingCities = true;
		api
			.get<{ items: Space[] }>(`/spaces?level=city&parent=${destId}`)
			.then((res) => {
				cities = res.data?.items ?? [];
			})
			.catch(() => {
				cities = [];
			})
			.finally(() => {
				loadingCities = false;
			});
	});

	const canSubmit = $derived(
		title.trim().length > 0 &&
			description.trim().length > 0 &&
			destinationSpaceId.length > 0 &&
			!submitting
	);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!canSubmit) return;
		submitting = true;
		error = null;
		const res = await api.post<MutualAidPost>('/mutual-aid', {
			post_type: postType,
			category,
			title: title.trim(),
			description: description.trim(),
			destination_space_id: destinationSpaceId,
			city_space_id: citySpaceId || undefined
		});
		submitting = false;
		if (res.error) {
			error = res.error.message || t('mutual_aid.create_error', 'Failed to create post.');
			return;
		}
		if (res.data?.id) {
			goto(`/mutual-aid/${res.data.id}`);
		} else {
			goto('/mutual-aid');
		}
	}

	// Redirect guests to login (client-side guard; the backend also rejects
	// unauthenticated POST /mutual-aid).
	$effect(() => {
		if (!authed) {
			goto(`/login?redirect=${encodeURIComponent(page.url.pathname + page.url.search)}`);
		}
	});

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
	<title>{t('mutual_aid.new_title', 'New Mutual Aid Post — JapaConnect')}</title>
	<meta name="description" content={t('mutual_aid.new_description', 'Post a help request or offer.')} />
	<meta name="robots" content="noindex" />
</svelte:head>

{#if authed}
	<div class="mx-auto w-full max-w-full">
		<div class="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
			<a
				href="/mutual-aid"
				class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
				aria-label={t('mutual_aid.back', 'Back to mutual aid')}
			>
				<ArrowLeft size={18} />
			</a>
			<h1 class="text-lg font-semibold text-[var(--color-text)]">
				{t('mutual_aid.new_heading', 'New Mutual Aid Post')}
			</h1>
		</div>

		<form onsubmit={submit} class="mx-auto max-w-2xl space-y-5 p-4" novalidate>
			{#if error}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-600" role="alert">
					{error}
				</p>
			{/if}

			<!-- Destination country (first input per geo-aware tool pattern —
			     BUG-010. The destination selector is always the first UI
			     input on geo-aware create forms.) -->
			<div class="flex flex-col gap-1.5">
				<label for="destination_country" class="text-sm font-medium text-[var(--color-text)]">
					{t('mutual_aid.field_destination', 'Destination country')}
					<span class="text-red-500" aria-hidden="true">*</span>
				</label>
				<Autocomplete
					placeholder={t('mutual_aid.field_destination_placeholder', 'Select a country')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinationCountries}
					initialFetcher={fetchDestinationCountriesInitial}
					selectedId={destinationSpaceId || null}
					selectedLabel={selectedDestinationLabel}
					onSelect={(item) => (destinationSpaceId = item.id)}
					onClear={() => (destinationSpaceId = '')}
				/>
			</div>

			<!-- City (optional — populated from /spaces?level=city&parent=<dest>) -->
			<div class="flex flex-col gap-1.5">
				<label for="city" class="text-sm font-medium text-[var(--color-text)]">
					{t('mutual_aid.field_city', 'City')}
				</label>
				<select
					id="city"
					bind:value={citySpaceId}
					disabled={!destinationSpaceId || loadingCities}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none disabled:opacity-50"
				>
					<option value="">
						{loadingCities
							? t('common.loading', 'Loading…')
							: t('mutual_aid.field_city_placeholder', 'Select a city (optional)')}
					</option>
					{#each cities as c (c.id)}
						<option value={c.id}>{c.name}</option>
					{/each}
				</select>
			</div>

			<!-- Post type -->
			<div class="flex flex-col gap-1.5">
				<label for="post_type" class="text-sm font-medium text-[var(--color-text)]">
					{t('mutual_aid.field_post_type', 'Post type')}
					<span class="text-red-500" aria-hidden="true">*</span>
				</label>
				<select
					id="post_type"
					bind:value={postType}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
				>
					{#each postTypeOptions as opt (opt.value)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>

			<!-- Category -->
			<div class="flex flex-col gap-1.5">
				<label for="category" class="text-sm font-medium text-[var(--color-text)]">
					{t('mutual_aid.field_category', 'Category')}
					<span class="text-red-500" aria-hidden="true">*</span>
				</label>
				<select
					id="category"
					bind:value={category}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
				>
					{#each categoryOptions as opt (opt.value)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>

			<!-- Title -->
			<div class="flex flex-col gap-1.5">
				<label for="title" class="text-sm font-medium text-[var(--color-text)]">
					{t('mutual_aid.field_title', 'Title')}
					<span class="text-red-500" aria-hidden="true">*</span>
				</label>
				<input
					id="title"
					type="text"
					bind:value={title}
					maxlength="120"
					placeholder={t('mutual_aid.field_title_placeholder', 'Brief summary of what you need or offer')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>

			<!-- Description -->
			<AiBodyField
				id="description"
				label={t('mutual_aid.field_description', 'Description')}
				contentType="mutual_aid"
				{title}
				body={description}
				onBody={(v) => (description = v)}
				onTitle={(v) => (title = v)}
				user={data.user}
				features={data.features ?? {}}
				required
				placeholder={t('mutual_aid.field_description_placeholder', 'Provide details about your request or offer…')}
				rows={6}
			/>

			<!-- Submit -->
			<div class="flex items-center justify-end gap-2 border-t border-[var(--color-border)] pt-4">
				<a
					href="/mutual-aid"
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					{t('common.cancel', 'Cancel')}
				</a>
				<button
					type="submit"
					disabled={!canSubmit}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					<Send size={16} /> {submitting ? '…' : t('mutual_aid.submit', 'Post')}
				</button>
			</div>
		</form>
	</div>
{/if}
