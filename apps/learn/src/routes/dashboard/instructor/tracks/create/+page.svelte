<script lang="ts">
	// Track creation — Japa University instructor creates a new track (ADR-063).
	// CSR ONLY: POST /learn/university/my/tracks.
	// Payload matches backend CreateTrackInput (internal/university/store.go):
	//   title (required), slug (required), description, difficulty,
	//   price_kobo, currency, category, language, geo_space_id.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { Plus, Save, ArrowLeft } from 'lucide-svelte';
	import { formatPriceKobo } from '$lib/format';
	import { loadCurrencies, currencyLabel, type CurrencyInfo } from '$lib/utils/currency';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import type { GeoCountry, CatalogOption } from '$lib/types';

	type Category = { id: string; slug: string; name: string; sort_order: number };

	let title = $state('');
	let slug = $state('');
	let slugTouched = $state(false);
	let description = $state('');
	let difficulty = $state('');
	let difficultyLevels = $state<CatalogOption[]>([]);
	let priceMain = $state(0);
	let currency = $state('');
	// §58 — managed currency list for the currency picker (name + code labels).
	let currencies = $state<CurrencyInfo[]>([]);
	let category = $state('');
	let categories = $state<Category[]>([]);
	let language = $state(page.data.locale ?? '');
	let languages = $state<string[]>([]);
	let subscriptionEligible = $state(false);
	let saving = $state(false);
	let error = $state<string | null>(null);

	// Geo targeting — country selector consumes GET /api/v1/geo/countries.
	let geoSpaceId = $state('');
	let countries = $state<GeoCountry[]>([]);
	let countriesLoading = $state(false);

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
			if (difficulty === '' && difficultyLevels.length > 0) {
				difficulty = difficultyLevels[0].value;
			}
		}
	});

	// Auto-generate slug from title unless the user has manually edited the slug.
	$effect(() => {
		if (!slugTouched) {
			slug = title
				.toLowerCase()
				.trim()
				.replace(/[^a-z0-9\s-]/g, '')
				.replace(/\s+/g, '-')
				.replace(/-+/g, '-')
				.replace(/^-|-$/g, '');
		}
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

	// Keep the current value selectable even when it isn't in the backend
	// list yet (e.g. first track in a new language).
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

	const formValid = $derived(title.trim().length > 0 && slug.trim().length > 0);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (saving || !formValid) return;
		saving = true;
		error = null;
		const r = await api.post('/learn/university/my/tracks', {
			title: title.trim(),
			slug: slug.trim(),
			description: description || undefined,
			difficulty: difficulty || undefined,
			price_kobo: Math.round(priceMain * 100),
			currency: currency || undefined,
			category: category || undefined,
			language: language || undefined,
			geo_space_id: geoSpaceId || undefined,
			subscription_eligible: subscriptionEligible
		});
		saving = false;
		if (r.error) {
			error = r.error.message;
		} else {
			goto('/dashboard/instructor');
		}
	}
</script>

<svelte:head>
	<title>{t('university.create_track')} · {t('app.name')}</title>
</svelte:head>

<div class="flex justify-center px-4 py-6 md:px-6 lg:px-8">
	<form onsubmit={submit} class="w-full max-w-2xl space-y-5">
		<header class="mb-2">
			<a
				href="/dashboard/instructor"
				class="mb-3 inline-flex items-center gap-1 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
			>
				<ArrowLeft size={16} />
				{t('common.back')}
			</a>
			<h1 class="flex items-center gap-2 text-2xl font-bold text-[var(--color-text)]">
				<Plus size={24} class="text-[var(--color-primary)]" />
				{t('university.create_track')}
			</h1>
			<p class="mt-1 text-sm text-[var(--color-muted)]">
				{t('university.create_track_description')}
			</p>
		</header>

		<!-- Title (required) -->
		<div>
			<label for="title" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.track_title_label')}
			</label>
			<input
				id="title"
				type="text"
				bind:value={title}
				required
				placeholder={t('university.title_placeholder', 'e.g. Mastering Go for Backend Engineers')}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
		</div>

		<!-- Slug (required, auto-generated) -->
		<div>
			<label for="slug" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.track_slug_label')}
			</label>
			<input
				id="slug"
				type="text"
				bind:value={slug}
				oninput={() => (slugTouched = true)}
				required
				placeholder={t('university.slug_placeholder', 'mastering-go-for-backend-engineers')}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
			<p class="mt-1 text-xs text-[var(--color-text-subtle)]">
				{t('university.track_slug_hint')}
			</p>
		</div>

		<!-- Description (optional) -->
		<div>
			<label for="description" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.track_description_label')}
			</label>
			<textarea
				id="description"
				bind:value={description}
				rows="4"
				placeholder={t('university.description_placeholder', 'A short summary of what learners will gain from this track…')}
				class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			></textarea>
		</div>

		<!-- Difficulty (optional) -->
		<div>
			<label for="difficulty" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.track_difficulty_label')}
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
				{t('university.track_price_label')}
			</label>
			<input
				id="price"
				type="number"
				min="0"
				step="0.01"
				bind:value={priceMain}
				placeholder="0"
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
				{t('university.track_currency_label')}
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

		<!-- Category (optional, from API) -->
		<div>
			<label for="category" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.track_category_label')}
			</label>
			<Autocomplete
			id="category"
			placeholder={t('university.track_category_label')}
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
				{t('university.track_language_label')}
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

		<button
			type="submit"
			disabled={saving || !formValid}
			class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-60"
		>
			<Save size={16} />
			{saving ? t('common.loading') : t('university.create_track')}
		</button>
	</form>
</div>
