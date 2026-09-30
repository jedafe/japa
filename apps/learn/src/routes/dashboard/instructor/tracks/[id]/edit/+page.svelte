<script lang="ts">
	// Track editor — Japa University instructor edits an existing track (ADR-063).
	// CSR ONLY: GET /learn/university/my/tracks (find by id), PATCH /learn/university/my/tracks/{id}.
	// PATCH accepts optional fields: title, description, difficulty, price_kobo,
	// currency, category, language, geo_space_id (internal/university/handlers.go).
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import { Save, ArrowLeft } from 'lucide-svelte';
	import { formatPriceKobo } from '$lib/format';
	import { loadCurrencies, currencyLabel, type CurrencyInfo } from '$lib/utils/currency';
	import type { InstructorTrack, GeoCountry, CatalogOption } from '$lib/types';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';

	const trackId = page.params.id;

	type Category = { id: string; slug: string; name: string; sort_order: number };

	let track = $state<InstructorTrack | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let error = $state<string | null>(null);
	let success = $state<string | null>(null);
	let categories = $state<Category[]>([]);

	// Form fields
	let title = $state('');
	let description = $state('');
	let difficulty = $state('');
	let difficultyLevels = $state<CatalogOption[]>([]);
	let priceMain = $state(0);
	let currency = $state('');
	// §58 — managed currency list for the currency picker (name + code labels).
	let currencies = $state<CurrencyInfo[]>([]);
	let category = $state('');
	let language = $state(page.data.locale ?? '');
	let languages = $state<string[]>([]);
	let subscriptionEligible = $state(false);

	// Geo targeting — country selector consumes GET /api/v1/geo/countries.
	let geoSpaceId = $state('');
	let countries = $state<GeoCountry[]>([]);

	// Fetch active categories, country spaces, and difficulty options from public APIs (Rule 16).
	onMount(async () => {
		const [catRes, geoRes, optRes, currencyList] = await Promise.all([
			api.get<Category[]>('/learn/university/categories'),
			api.get<GeoCountry[]>('/geo/countries'),
			api.get<{ difficulty_levels: CatalogOption[]; languages: string[] }>('/learn/university/catalog/options'),
			loadCurrencies()
		]);
		currencies = currencyList;
		if (!catRes.error && catRes.data) {
			categories = catRes.data;
		}
		if (!geoRes.error && geoRes.data) {
			countries = geoRes.data;
		}
		if (!optRes.error && optRes.data) {
			difficultyLevels = optRes.data.difficulty_levels ?? [];
			languages = optRes.data.languages ?? [];
		}
	});

	// Keep the current value selectable even when it isn't in the backend
	// list (e.g. a legacy language code no longer offered).
	const languageOptions = $derived.by(() => {
		const opts = languages.map((l) => ({ value: l, label: l.toUpperCase() }));
		if (language && !opts.some((o) => o.value === language)) {
			opts.unshift({ value: language, label: language.toUpperCase() });
		}
		return opts;
	});

	// §58 — "CODE — Name" currency options; keep a stored code that isn't in
	// the managed list selectable (e.g. a currency deactivated later).
	const currencyOptions = $derived.by(() => {
		const opts = currencies.map((c) => ({ value: c.code, label: currencyLabel(c.code, currencies) }));
		if (currency && !opts.some((o) => o.value === currency)) {
			opts.unshift({ value: currency, label: currencyLabel(currency, currencies) });
		}
		return opts;
	});

	// Autocomplete fetchers — filter the already-loaded categories client-side.
	async function fetchCategories(q: string): Promise<AutocompleteItem[]> {
		const lower = q.toLowerCase();
		return categories
			.filter((c) => c.name.toLowerCase().includes(lower))
			.map((c) => ({ id: c.slug, label: c.name }));
	}
	async function fetchCategoriesInitial(): Promise<AutocompleteItem[]> {
		return categories.map((c) => ({ id: c.slug, label: c.name }));
	}
	function getCategoryLabel(slug: string): string {
		return categories.find((c) => c.slug === slug)?.name ?? slug;
	}

	// Country Autocomplete — live-debounced client-side search over /geo/countries.
	async function fetchCountries(q: string): Promise<AutocompleteItem[]> {
		const lower = q.toLowerCase();
		return countries
			.filter((c) => c.name.toLowerCase().includes(lower) || c.geo_code.toLowerCase().startsWith(lower))
			.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
	}
	async function fetchCountriesInitial(): Promise<AutocompleteItem[]> {
		return countries.slice(0, 50).map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
	}
	function getCountryLabel(id: string): string {
		return countries.find((c) => c.id === id)?.name ?? '';
	}
	function onSelectCountry(item: AutocompleteItem) {
		geoSpaceId = item.id;
		const country = countries.find((c) => c.id === item.id);
		if (country?.currency_code) {
			currency = country.currency_code;
		}
	}
	function onClearCountry() {
		geoSpaceId = '';
	}

	$effect(() => {
		(async () => {
			const r = await api.get<InstructorTrack[]>('/learn/university/my/tracks');
			loading = false;
			if (r.data) {
				const found = r.data.find((tr) => tr.id === trackId) ?? null;
				track = found;
				if (found) {
					title = found.title ?? '';
					description = found.description ?? '';
					difficulty = found.level ?? '';
					priceMain = (found.price_kobo ?? 0) / 100;
					currency = found.currency ?? '';
					category = found.category ?? '';
					language = found.language ?? page.data.locale ?? '';
					geoSpaceId = found.geo_space_id ?? '';
					subscriptionEligible = found.subscription_eligible ?? false;
				}
			}
		})();
	});

	async function save(e: SubmitEvent) {
		e.preventDefault();
		if (saving) return;
		saving = true;
		error = null;
		success = null;

		const body: Record<string, unknown> = {
			title: title || undefined,
			description: description || undefined,
			difficulty: difficulty,
			price_kobo: Math.round(priceMain * 100),
			currency: currency || undefined,
			category: category || undefined,
			language: language || undefined,
			geo_space_id: geoSpaceId || undefined,
			subscription_eligible: subscriptionEligible
		};

		const r = await api.patch<InstructorTrack>(`/learn/university/my/tracks/${trackId}`, body);
		saving = false;

		if (r.error) {
			error = r.error.message;
		} else {
			success = t('university.track_saved', 'Track saved successfully.');
			// Redirect back to the instructor dashboard after a brief success flash.
			setTimeout(() => goto('/dashboard/instructor'), 800);
		}
	}
</script>

<svelte:head>
	<title>{t('university.edit_track', 'Edit Track')} · {t('app.name')}</title>
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<header class="mb-6">
		<a
			href="/dashboard/instructor"
			class="mb-3 inline-flex items-center gap-1 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={16} />
			{t('common.back', 'Back')}
		</a>
		<h1 class="text-2xl font-bold text-[var(--color-text)]">
			{t('university.edit_track', 'Edit Track')}
		</h1>
	</header>

	{#if loading}
		<div class="max-w-2xl space-y-4">
			{#each Array(6) as _}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<LoadingSkeleton class="mb-2 h-4 w-24" />
					<LoadingSkeleton class="h-10 w-full" />
				</div>
			{/each}
		</div>
	{:else if !track}
		<EmptyState
			title={t('university.track_not_found', 'Track not found')}
			description={t('university.track_not_found_description', 'This track could not be found or you do not have access.')}
		/>
	{:else}
		<form onsubmit={save} class="max-w-2xl space-y-5">
			<!-- Title -->
			<div>
				<label for="title" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.title_label', 'Title')}
				</label>
				<input
					id="title"
					type="text"
					bind:value={title}
					required
					placeholder={t('university.title_placeholder', 'Course title')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>

			<!-- Description -->
			<div>
				<label for="description" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.description_label', 'Description')}
				</label>
				<textarea
					id="description"
					bind:value={description}
					rows="4"
					placeholder={t('university.description_placeholder', 'Short summary of the course')}
					class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				></textarea>
			</div>

			<!-- Difficulty -->
			<div>
				<label for="difficulty" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.difficulty_label', 'Difficulty')}
				</label>
				<AutocompleteSelect
					id="difficulty"
					bind:value={difficulty}
					options={difficultyLevels.map((opt) => ({ value: opt.value, label: t(`level.${opt.value}`) }))}
					class="w-full"
				/>
			</div>

			<!-- Price (main currency unit → kobo/cents) -->
			<div>
				<label for="price" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.price_label', 'Price')}
				</label>
				<input
					id="price"
					type="number"
					min="0"
					step="0.01"
					bind:value={priceMain}
					placeholder="0.00"
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
				<p class="mt-1 text-xs text-[var(--color-text-subtle)]">
					{priceMain <= 0 ? t('university.track_price_free_hint') : t('university.track_price_preview', 'Preview: {price}').replace('{price}', formatPriceKobo(Math.round(priceMain * 100), currency, page.data.locale, page.data.currencies))}
				</p>
			</div>

			<!-- Country (primary geo space) -->
			<div>
				<label for="country" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.track_country_label', 'Country / primary market')}
				</label>
				<Autocomplete
					id="country"
					placeholder={t('university.track_country_placeholder', 'Search countries…')}
					emptyMessage={t('common.no_results', 'No countries found')}
					fetcher={fetchCountries}
					initialFetcher={fetchCountriesInitial}
					selectedId={geoSpaceId || null}
					selectedLabel={geoSpaceId ? getCountryLabel(geoSpaceId) : null}
					onSelect={onSelectCountry}
					onClear={onClearCountry}
				/>
				<p class="mt-1 text-xs text-[var(--color-text-subtle)]">
					{t('university.track_country_hint', 'Select the country this track is primarily for. This defaults the currency.')}
				</p>
			</div>

			<!-- Currency (auto-filled from selected country; editable override) -->
			<div>
				<label for="currency" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.currency_label', 'Currency')}
				</label>
				{#if currencyOptions.length > 0}
					<AutocompleteSelect
						id="currency"
						bind:value={currency}
						options={currencyOptions}
						placeholder={t('university.track_currency_placeholder', 'e.g. USD')}
						clearable
						class="w-full"
					/>
				{:else}
					<input
						id="currency"
						type="text"
						bind:value={currency}
						placeholder={t('university.track_currency_placeholder', 'e.g. USD')}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
					/>
				{/if}
			</div>

			<!-- Category (from API) -->
			<div>
				<label for="category" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.category_label', 'Category')}
				</label>
				<Autocomplete
					id="category"
					placeholder={t('university.category_label', 'Category')}
					emptyMessage={t('common.no_results', 'No categories found')}
					fetcher={fetchCategories}
					initialFetcher={fetchCategoriesInitial}
					selectedId={category || null}
					selectedLabel={category ? getCategoryLabel(category) : null}
					onSelect={(item) => (category = item.id)}
					onClear={() => (category = '')}
				/>
			</div>

			<!-- Language — options come from the backend catalog/options endpoint -->
			<div>
				<label for="language" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.language_label', 'Language')}
				</label>
				<AutocompleteSelect
					id="language"
					bind:value={language}
					options={languageOptions}
					placeholder={page.data.locale || 'en'}
					clearable
					class="w-full"
				/>
			</div>

			<!-- Subscription eligible -->
			<div class="flex items-start gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4">
				<input id="subscription-eligible" type="checkbox" bind:checked={subscriptionEligible} class="mt-0.5 h-4 w-4 rounded" />
				<div>
					<label for="subscription-eligible" class="block text-sm font-medium text-[var(--color-text)]">
						{t('university.track_subscription_eligible_label', 'Subscription eligible')}
					</label>
					<p class="mt-0.5 text-xs text-[var(--color-muted)]">
						{t('university.track_subscription_eligible_hint', 'Allow subscribers to access this course as part of their plan. You earn a stream-rate revenue share when they enroll.')}
					</p>
				</div>
			</div>

			{#if error}
				<p class="text-sm text-[var(--color-destructive)]">{error}</p>
			{/if}
			{#if success}
				<p class="text-sm text-[var(--color-progress)]">{success}</p>
			{/if}

			<button
				type="submit"
				disabled={saving}
				class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-60"
			>
				<Save size={16} />
				{saving ? t('common.loading') : t('common.save', 'Save')}
			</button>
		</form>
	{/if}
</div>
