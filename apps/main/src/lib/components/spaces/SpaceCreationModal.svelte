<script lang="ts">
	// Ref: japaconnect-engineering/sections/05-geo-first-space-system.md
	// Modal dialog for creating a new space (POST /spaces).
	//
	// The space being created is a community space (e.g. "Frontend Dev in Lagos,
	// Nigeria"). The cascading Country → State → City selectors pick the parent
	// geo location, then the user types the community name.
	//
	// Flow by space type:
	//   - Country type: pick Country → type community name
	//   - State type:   pick Country → pick State → type community name
	//   - City type:    pick Country → pick State → pick City (or new) → type community name
	//   - Custom type:  multi-select primary spaces → type community name
	//
	// Level is inferred from the type slug. The parent_id is set to the deepest
	// selected geo entity. If a new city is typed, it is created first.
	import { Dialog } from 'bits-ui';
	import { X, Rocket, Loader2, Plus, Check } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { suggestSlug } from '../create/slug';

	interface SpaceType {
		id: string;
		name: string;
		slug: string;
	}

	interface SpaceListItem {
		id: string;
		name: string;
		slug: string;
		level?: string;
	}

	interface PermissionPreset {
		name: string;
		visibility: string;
		posting_perm: string;
		replying_perm: string;
		commenting_perm: string;
		joining_perm: string;
	}

	interface Props {
		open?: boolean;
		/** When provided, called with the created space instead of navigating to it. */
		onCreated?: (space: { id: string; slug: string; name: string }) => void;
	}
	let { open = $bindable(false), onCreated }: Props = $props();

	let name = $state('');
	let slug = $state('');
	let slugTouched = $state(false);
	let description = $state('');
	let selectedSpaceTypeId = $state('');
	let spaceTypes = $state<SpaceType[]>([]);

	// Cascading geo selections.
	let selectedCountry = $state<SpaceListItem | null>(null);
	let selectedState = $state<SpaceListItem | null>(null);
	let selectedCity = $state<SpaceListItem | null>(null);
	// For new city: when user types a city name that doesn't exist.
	let newCityName = $state<string | null>(null);

	// Multi-parent selection (custom type).
	let multiParents = $state<SpaceListItem[]>([]);

	// Search state (shared by all comboboxes — only one open at a time).
	let searchResults = $state<SpaceListItem[]>([]);
	let searchQuery = $state('');
	let searchLoading = $state(false);
	let searchTimer: ReturnType<typeof setTimeout> | null = null;
	// Which combobox is currently active: 'country' | 'state' | 'city' | 'parent' | 'name'
	let activeSearch = $state<'country' | 'state' | 'city' | 'parent' | 'name' | null>(null);

	// Name auto-suggest state (to prevent duplicate spaces).
	let nameSuggestions = $state<SpaceListItem[]>([]);
	let nameSearchLoading = $state(false);
	let nameSearchTimer: ReturnType<typeof setTimeout> | null = null;
	let showNameSuggestions = $state(false);

	let submitting = $state(false);
	let error = $state('');

	// Quota display (BUG-008): fetch the user's owned spaces count to show
	// how many spaces they have created.
	let ownedSpacesCount = $state<number | null>(null);

	// Permission presets (BUG-009): fetch the available presets so the user
	// can choose one when creating a custom space.
	let presets = $state<PermissionPreset[]>([]);
	let selectedPresetName = $state('');

	$effect(() => {
		if (open) {
			fetchSpaceTypes();
			fetchOwnedSpacesCount();
			fetchPresets();
		}
	});

	async function fetchSpaceTypes() {
		try {
			const res = await api.get<SpaceType[]>('/space-types');
			if (res.data) {
				spaceTypes = res.data;
				if (!selectedSpaceTypeId) {
					// Default to "global" type — spaces with no geo parent.
					const global = spaceTypes.find((st) => st.slug === 'global');
					selectedSpaceTypeId = global?.id ?? spaceTypes[0]?.id ?? '';
				}
			}
		} catch {
			spaceTypes = [];
		}
	}

	async function fetchOwnedSpacesCount() {
		try {
			const res = await api.get<{ id: string }[]>('/me/spaces/owned');
			ownedSpacesCount = res.data?.length ?? 0;
		} catch {
			ownedSpacesCount = null;
		}
	}

	async function fetchPresets() {
		try {
			const res = await api.get<PermissionPreset[]>('/space-permission-presets');
			presets = res.data ?? [];
		} catch {
			presets = [];
		}
	}

	// Infer level from the selected space type slug.
	// Primary types (country/state/city) have slugs matching level names.
	// "global" slug → global level (no geo parent needed).
	// Everything else (community, university, research, etc.) → custom level.
	const selectedSpaceType = $derived(
		spaceTypes.find((st) => st.id === selectedSpaceTypeId) ?? null
	);
	const typeSlug = $derived(selectedSpaceType?.slug ?? '');
	const level = $derived(
		typeSlug === 'country' ? 'country'
		: typeSlug === 'state' ? 'state'
		: typeSlug === 'city' ? 'city'
		: typeSlug === 'global' ? 'global'
		: 'custom'
	);

	// Whether this type requires geo cascade at all.
	// Global type: no cascade. University type: cascade required.
	// Other custom types (Community, Research): cascade optional.
	const isGlobalType = $derived(typeSlug === 'global');
	const isUniversityType = $derived(typeSlug === 'university');

	// Which cascading selectors to show.
	//   Country type: Country only (required)
	//   State type:   Country → State (both required)
	//   City type:    Country → State → City (all required, new city allowed)
	//   University:   Country → State → City (country required, state/city optional)
	//   Global:       No cascade
	//   Other custom (Community, Research): cascade optional at every level
	const showCountrySelect = $derived(!isGlobalType);
	const showStateSelect = $derived(level === 'state' || level === 'city' || level === 'custom');
	const showCitySelect = $derived(level === 'city' || level === 'custom');
	// SPACES-010: enable multi-parent selector for custom-level spaces.
	// Custom spaces must link to at least one primary space (country/state/city)
	// via the space_parent_spaces junction (Section 05 line 14, 190).
	const showMultiParent = $derived(level === 'custom' && !isUniversityType);

	// Whether each level is required.
	// University: country required (universities have physical locations).
	// Other custom types: everything optional (global by default).
	const countryRequired = $derived(level === 'country' || level === 'state' || level === 'city' || isUniversityType);
	const stateRequired = $derived(level === 'state' || level === 'city');
	const cityRequired = $derived(level === 'city');

	// ---- Search function (shared) ----
	// searchLevel: filter by level (used for country search).
	// parentId: filter by parent (used for state/city cascade — only show children of the selected parent).
	function searchSpaces(q: string, target: 'country' | 'state' | 'city' | 'parent', searchLevel: string | null = null, parentId: string | null = null) {
		searchQuery = q;
		activeSearch = target;
		if (searchTimer) clearTimeout(searchTimer);
		if (q.trim().length < 1) {
			searchResults = [];
			return;
		}
		searchLoading = true;
		searchTimer = setTimeout(async () => {
			try {
				const params = new URLSearchParams({ q: q.trim() });
				if (parentId) {
					params.set('parent', parentId);
				} else if (searchLevel) {
					params.set('level', searchLevel);
				}
				const res = await api.get<SpaceListItem[]>(`/spaces?${params.toString()}`);
				let results = res.data ?? [];
				// Filter out already-selected items.
				if (target === 'parent') {
					results = results.filter((s) => !multiParents.some((p) => p.id === s.id));
				}
				searchResults = results;
			} catch {
				searchResults = [];
			} finally {
				searchLoading = false;
			}
		}, 250);
	}

	// Wrappers for each combobox.
	// Country: search by level=country (no parent — top of hierarchy).
	// State: search by parent={selectedCountry.id} (only states in that country).
	// City: search by parent={selectedState.id} (only cities in that state).
	function searchCountries(q: string) { searchSpaces(q, 'country', 'country', null); }
	function searchStates(q: string) { searchSpaces(q, 'state', null, selectedCountry?.id ?? null); }
	function searchCities(q: string) { searchSpaces(q, 'city', null, selectedState?.id ?? null); }
	function searchParents(q: string) { searchSpaces(q, 'parent', null, null); }

	function selectCountry(s: SpaceListItem) {
		selectedCountry = s;
		// Reset downstream selections.
		selectedState = null;
		selectedCity = null;
		newCityName = null;
		searchQuery = '';
		searchResults = [];
		activeSearch = null;
	}
	function selectState(s: SpaceListItem) {
		selectedState = s;
		selectedCity = null;
		newCityName = null;
		searchQuery = '';
		searchResults = [];
		activeSearch = null;
	}
	function selectCity(s: SpaceListItem) {
		selectedCity = s;
		newCityName = null;
		searchQuery = '';
		searchResults = [];
		activeSearch = null;
	}
	function selectParent(s: SpaceListItem) {
		if (!multiParents.some((p) => p.id === s.id)) {
			multiParents = [...multiParents, s];
		}
		searchQuery = '';
		searchResults = [];
		activeSearch = null;
	}

	function removeCountry() { selectedCountry = null; selectedState = null; selectedCity = null; newCityName = null; }
	function removeState() { selectedState = null; selectedCity = null; newCityName = null; }
	function removeCity() { selectedCity = null; newCityName = null; }
	function removeParent(id: string) { multiParents = multiParents.filter((p) => p.id !== id); }

	// Handle new city input — when user types a city name that doesn't match any result.
	function onCityInput(q: string) {
		searchCities(q);
		// Track the typed name as a potential new city.
		if (q.trim()) {
			newCityName = q.trim();
			selectedCity = null;
		} else {
			newCityName = null;
		}
	}

	// Name auto-suggest — search existing spaces to prevent duplicates.
	function searchNames(q: string) {
		if (nameSearchTimer) clearTimeout(nameSearchTimer);
		if (q.trim().length < 1) {
			nameSuggestions = [];
			showNameSuggestions = false;
			return;
		}
		nameSearchLoading = true;
		showNameSuggestions = true;
		nameSearchTimer = setTimeout(async () => {
			try {
				const res = await api.get<SpaceListItem[]>(`/spaces?q=${encodeURIComponent(q.trim())}`);
				nameSuggestions = res.data ?? [];
			} catch {
				nameSuggestions = [];
			} finally {
				nameSearchLoading = false;
			}
		}, 250);
	}

	function selectExistingName(space: SpaceListItem) {
		// User selected an existing space — use it directly instead of creating a duplicate.
		open = false;
		if (onCreated) {
			onCreated({ id: space.id, slug: space.slug, name: space.name });
		} else {
			goto(`/spaces/${space.slug}`);
		}
	}

	function onNameInput(e: Event) {
		name = (e.currentTarget as HTMLInputElement).value;
		if (!slugTouched) slug = suggestSlug(name);
		searchNames(name);
	}
	function onSlugInput(e: Event) {
		slug = (e.currentTarget as HTMLInputElement).value;
		slugTouched = true;
	}

	// Clear all selections when space type changes.
	$effect(() => {
		void selectedSpaceTypeId;
		selectedCountry = null;
		selectedState = null;
		selectedCity = null;
		newCityName = null;
		multiParents = [];
		searchQuery = '';
		searchResults = [];
		activeSearch = null;
		nameSuggestions = [];
		showNameSuggestions = false;
	});

	// Close dropdown when clicking outside.
	function onWindowClick(e: MouseEvent) {
		const target = e.target as HTMLElement;
		if (!target.closest('.geo-combobox') && !target.closest('.geo-dropdown') && !target.closest('.name-combobox')) {
			activeSearch = null;
			searchResults = [];
			showNameSuggestions = false;
		}
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (!name.trim() || !slug.trim()) {
			error = t('spaces.create_error_fields');
			return;
		}
		if (!selectedSpaceTypeId) {
			error = t('spaces.no_types');
			return;
		}
		// Validate geo selections based on level.
		if (countryRequired && !selectedCountry) {
			error = t('spaces.create_error_select_country', 'Please select a country');
			return;
		}
		if (stateRequired && !selectedState) {
			error = t('spaces.create_error_select_state', 'Please select a state');
			return;
		}
		if (cityRequired && !selectedCity && !newCityName) {
			error = t('spaces.create_error_select_city', 'Please select or type a city name');
			return;
		}

		submitting = true;
		try {
			let parentId: string | undefined;

			if (level === 'country') {
				parentId = selectedCountry!.id;
			} else if (level === 'state') {
				parentId = selectedState!.id;
			} else if (level === 'city' || level === 'custom') {
				if (selectedCity) {
					// Use existing city as parent.
					parentId = selectedCity.id;
				} else if (newCityName) {
					// Create the new city first, then use it as parent.
					const citySlug = suggestSlug(newCityName);
					const cityRes = await api.post<{ id: string; slug: string; name: string }>('/spaces', {
						name: newCityName,
						slug: citySlug,
						space_type_id: spaceTypes.find((st) => st.slug === 'city')?.id ?? '',
						level: 'city',
						parent_id: selectedState!.id
					});
					if (!cityRes.data?.id) {
						error = t('spaces.create_error_city', 'Failed to create city');
						submitting = false;
						return;
					}
					parentId = cityRes.data.id;
				} else if (level === 'custom' && selectedState) {
					// Custom type: user stopped at state level.
					parentId = selectedState.id;
				} else if (level === 'custom' && selectedCountry) {
					// Custom type: user stopped at country level.
					parentId = selectedCountry.id;
				}
			}

			const body: Record<string, unknown> = {
				name: name.trim(),
				slug: slug.trim(),
				space_type_id: selectedSpaceTypeId,
				level,
				description: description.trim() || undefined
			};
			if (parentId) body.parent_id = parentId;
			// SPACES-010: send multi-parent IDs for custom spaces so the
			// backend populates the space_parent_spaces junction.
			if (multiParents.length > 0) {
				body.parent_space_ids = multiParents.map((p) => p.id);
			}
			if (selectedPresetName) body.preset_name = selectedPresetName;

			const res = await api.post<{ id: string; slug: string; name: string }>('/spaces', body);
			if (res.data?.slug) {
				open = false;
				if (onCreated && res.data.id) {
					onCreated(res.data);
				} else {
					await goto(`/spaces/${res.data.slug}`);
				}
			} else {
				error = t('spaces.create_error');
			}
		} catch {
			error = t('spaces.create_error');
		} finally {
			submitting = false;
		}
	}

	function reset() {
		name = '';
		slug = '';
		slugTouched = false;
		description = '';
		selectedSpaceTypeId = '';
		selectedCountry = null;
		selectedState = null;
		selectedCity = null;
		newCityName = null;
		multiParents = [];
		searchResults = [];
		searchQuery = '';
		activeSearch = null;
		nameSuggestions = [];
		showNameSuggestions = false;
		error = '';
		selectedPresetName = '';
	}
</script>

<svelte:window onclick={onWindowClick} />

<Dialog.Root bind:open onOpenChange={(isOpen) => { if (!isOpen) reset(); }}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm" />
		<Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-[560px] -translate-x-1/2 -translate-y-1/2 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-xl max-h-[90vh] overflow-y-auto">
			<Dialog.Close class="absolute right-4 top-4 text-[var(--color-muted)] hover:text-[var(--color-text)]">
				<X size={20} />
			</Dialog.Close>

			<div class="mb-4 flex items-center gap-2">
				<Rocket size={20} class="text-[var(--color-primary)]" />
				<h2 class="text-lg font-semibold text-[var(--color-text)]">{t('spaces.create_title')}</h2>
			</div>

			{#if spaceTypes.length === 0}
				<p class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-muted)]">
					{t('spaces.no_types')}
				</p>
			{:else}
				<form onsubmit={submit} class="space-y-3">
					<!-- Space type selector -->
					<div>
						<label for="space-type" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
							{t('spaces.type_label', 'Space type')}
						</label>
						<select
							id="space-type"
							bind:value={selectedSpaceTypeId}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
							disabled={submitting}
						>
							{#each spaceTypes as st (st.id)}
								<option value={st.id}>{st.name}</option>
							{/each}
						</select>
					</div>

					<!-- Cascading geo selectors -->

					<!-- Country selector (for country/state/city types) -->
					{#if showCountrySelect}
						<div class="geo-combobox relative">
							<label for="geo-country" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
								{t('spaces.parent_country_label', 'Country')}
							</label>
							{#if selectedCountry}
								<div class="flex flex-wrap gap-1.5">
									<span class="inline-flex items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2 py-1 text-xs text-[var(--color-primary)]">
										<Check size={11} />
										{selectedCountry.name}
										<button type="button" onclick={removeCountry} class="text-[var(--color-muted)] hover:text-[var(--color-text)]">
											<X size={11} />
										</button>
									</span>
								</div>
							{:else}
								<input
									id="geo-country"
									type="text"
									class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
									placeholder={t('spaces.parent_country_placeholder', 'Search countries…')}
									value={activeSearch === 'country' ? searchQuery : ''}
									oninput={(e) => searchCountries(e.currentTarget.value)}
									onfocus={() => { activeSearch = 'country'; searchResults = []; }}
									disabled={submitting}
								/>
								{#if activeSearch === 'country' && searchLoading}
									<div class="absolute right-3 top-[26px]">
										<Loader2 size={14} class="animate-spin text-[var(--color-muted)]" />
									</div>
								{/if}
								{#if activeSearch === 'country' && searchResults.length > 0}
									<div class="geo-dropdown absolute z-20 mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-lg max-h-48 overflow-y-auto">
										{#each searchResults as result (result.id)}
											<button type="button" class="flex w-full items-center justify-between px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]" onclick={() => selectCountry(result)}>
												<span>{result.name}</span>
												<Plus size={14} class="text-[var(--color-muted)]" />
											</button>
										{/each}
									</div>
								{/if}
							{/if}
						</div>
					{/if}

					<!-- State selector — optional for custom types -->
					{#if showStateSelect && selectedCountry}
						<div class="geo-combobox relative">
							<label for="geo-state" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
								{t('spaces.parent_state_label', 'State / Province')}
								{#if !stateRequired}
									<span class="font-normal text-[var(--color-muted)]">({t('spaces.optional_label', 'optional')})</span>
								{/if}
							</label>
							{#if selectedState}
								<div class="flex flex-wrap gap-1.5">
									<span class="inline-flex items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2 py-1 text-xs text-[var(--color-primary)]">
										<Check size={11} />
										{selectedState.name}
										<button type="button" onclick={removeState} class="text-[var(--color-muted)] hover:text-[var(--color-text)]">
											<X size={11} />
										</button>
									</span>
								</div>
							{:else}
								<input
									id="geo-state"
									type="text"
									class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
									placeholder={t('spaces.parent_state_placeholder', 'Search states…')}
									value={activeSearch === 'state' ? searchQuery : ''}
									oninput={(e) => searchStates(e.currentTarget.value)}
									onfocus={() => { activeSearch = 'state'; searchResults = []; }}
									disabled={submitting}
								/>
								{#if activeSearch === 'state' && searchLoading}
									<div class="absolute right-3 top-[26px]">
										<Loader2 size={14} class="animate-spin text-[var(--color-muted)]" />
									</div>
								{/if}
								{#if activeSearch === 'state' && searchResults.length > 0}
									<div class="geo-dropdown absolute z-20 mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-lg max-h-48 overflow-y-auto">
										{#each searchResults as result (result.id)}
											<button type="button" class="flex w-full items-center justify-between px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]" onclick={() => selectState(result)}>
												<span>{result.name}</span>
												<Plus size={14} class="text-[var(--color-muted)]" />
											</button>
										{/each}
									</div>
								{/if}
							{/if}
						</div>
					{/if}

					<!-- City selector — accepts new city input. Optional for custom types. -->
					{#if showCitySelect && selectedState}
						<div class="geo-combobox relative">
							<label for="geo-city" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
								{t('spaces.parent_city_label', 'City')}
								{#if !cityRequired}
									<span class="font-normal text-[var(--color-muted)]">({t('spaces.optional_label', 'optional')})</span>
								{/if}
							</label>
							{#if selectedCity}
								<div class="flex flex-wrap gap-1.5">
									<span class="inline-flex items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2 py-1 text-xs text-[var(--color-primary)]">
										<Check size={11} />
										{selectedCity.name}
										<button type="button" onclick={removeCity} class="text-[var(--color-muted)] hover:text-[var(--color-text)]">
											<X size={11} />
										</button>
									</span>
								</div>
							{:else}
								<input
									id="geo-city"
									type="text"
									class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
									placeholder={t('spaces.parent_city_placeholder', 'Search cities or type a new one…')}
									value={activeSearch === 'city' ? searchQuery : ''}
									oninput={(e) => onCityInput(e.currentTarget.value)}
									onfocus={() => { activeSearch = 'city'; searchResults = []; }}
									disabled={submitting}
								/>
								{#if activeSearch === 'city' && searchLoading}
									<div class="absolute right-3 top-[26px]">
										<Loader2 size={14} class="animate-spin text-[var(--color-muted)]" />
									</div>
								{/if}
								{#if activeSearch === 'city' && searchResults.length > 0}
									<div class="geo-dropdown absolute z-20 mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-lg max-h-48 overflow-y-auto">
										{#each searchResults as result (result.id)}
											<button type="button" class="flex w-full items-center justify-between px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]" onclick={() => selectCity(result)}>
												<span>{result.name}</span>
												<Plus size={14} class="text-[var(--color-muted)]" />
											</button>
										{/each}
									</div>
								{/if}
								{#if activeSearch === 'city' && !searchLoading && newCityName && searchResults.length === 0}
									<div class="geo-dropdown absolute z-20 mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-lg">
										<div class="px-3 py-2 text-sm text-[var(--color-muted)]">
											{t('spaces.city_new_hint', 'No match — a new city will be created.')}: <span class="font-medium text-[var(--color-text)]">{newCityName}</span>
										</div>
									</div>
								{/if}
							{/if}
						</div>
					{/if}

					<!-- Multi-parent selector (custom type) -->
					{#if showMultiParent}
						<div class="geo-combobox relative">
							<label for="geo-parent" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
								{t('spaces.parent_label', 'Parent space(s)')}
							</label>
							{#if multiParents.length > 0}
								<div class="mb-2 flex flex-wrap gap-1.5">
									{#each multiParents as parent (parent.id)}
										<span class="inline-flex items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2 py-1 text-xs text-[var(--color-primary)]">
											{parent.name}
											<button type="button" onclick={() => removeParent(parent.id)} class="text-[var(--color-muted)] hover:text-[var(--color-text)]">
												<X size={11} />
											</button>
										</span>
									{/each}
								</div>
							{/if}
							<input
								id="geo-parent"
								type="text"
								class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
								placeholder={t('spaces.parent_search_placeholder', 'Search primary spaces…')}
								value={activeSearch === 'parent' ? searchQuery : ''}
								oninput={(e) => searchParents(e.currentTarget.value)}
								onfocus={() => { activeSearch = 'parent'; searchResults = []; }}
								disabled={submitting}
							/>
							{#if activeSearch === 'parent' && searchLoading}
								<div class="absolute right-3 top-[26px]">
									<Loader2 size={14} class="animate-spin text-[var(--color-muted)]" />
								</div>
							{/if}
							{#if activeSearch === 'parent' && searchResults.length > 0}
								<div class="geo-dropdown absolute z-20 mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-lg max-h-48 overflow-y-auto">
									{#each searchResults as result (result.id)}
										<button type="button" class="flex w-full items-center justify-between px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]" onclick={() => selectParent(result)}>
											<span>{result.name}</span>
											<Plus size={14} class="text-[var(--color-muted)]" />
										</button>
									{/each}
								</div>
							{/if}
							<p class="mt-1 text-xs text-[var(--color-muted)]">
								{t('spaces.parent_help', 'Select at least one primary space (country, state, or city).')}
							</p>
						</div>
					{/if}

					<!-- Divider before space details -->
					{#if showCountrySelect || showMultiParent}
						<div class="border-t border-[var(--color-border)] pt-3">
							<p class="mb-2 text-xs font-medium text-[var(--color-muted)]">
								{t('spaces.details_section', 'Space details')}
							</p>
						</div>
					{/if}

					<!-- Space name — auto-suggests existing spaces to prevent duplicates -->
					<div class="name-combobox relative">
						<label for="space-name" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
							{t('spaces.name_label', 'Space name')}
						</label>
						<input
							id="space-name"
							value={name}
							oninput={onNameInput}
							onfocus={() => { if (name.trim()) showNameSuggestions = true; }}
							onblur={() => { setTimeout(() => (showNameSuggestions = false), 200); }}
							placeholder={t('spaces.name_community_placeholder', 'e.g. Frontend Devs in Lagos, Nigeria')}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
							disabled={submitting}
						/>
						{#if nameSearchLoading}
							<div class="absolute right-3 top-[26px]">
								<Loader2 size={14} class="animate-spin text-[var(--color-muted)]" />
							</div>
						{/if}
						<!-- Existing space suggestions — click to use instead of creating a duplicate -->
						{#if showNameSuggestions && nameSuggestions.length > 0}
							<div class="absolute z-20 mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-lg max-h-48 overflow-y-auto">
								{#each nameSuggestions as suggestion (suggestion.id)}
									<button
										type="button"
										class="flex w-full items-center justify-between px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
										onclick={() => selectExistingName(suggestion)}
									>
										<span>{suggestion.name}</span>
										<span class="text-xs text-[var(--color-muted)]">{t('spaces.exists_label', 'exists')}</span>
									</button>
								{/each}
							</div>
						{/if}
					</div>

					<div>
						<label for="space-slug" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('spaces.slug_label')}</label>
						<input
							id="space-slug"
							value={slug}
							oninput={onSlugInput}
							placeholder="my-space"
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
							disabled={submitting}
						/>
					</div>

					<div>
						<label for="space-desc" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('spaces.description_label')}</label>
						<textarea
							id="space-desc"
							bind:value={description}
							rows={2}
							placeholder={t('spaces.description_placeholder')}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
							disabled={submitting}
						></textarea>
					</div>

					{#if presets.length > 0}
						<div>
							<label for="space-preset" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
								{t('spaces.preset_label', 'Permission preset')}
								<span class="font-normal text-[var(--color-muted)]">({t('spaces.optional_label', 'optional')})</span>
							</label>
							<select
								id="space-preset"
								bind:value={selectedPresetName}
								class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
								disabled={submitting}
							>
								<option value="">{t('spaces.preset_default', 'Default (members can post)')}</option>
								{#each presets as p (p.name)}
									<option value={p.name}>{p.name}</option>
								{/each}
							</select>
						</div>
					{/if}

					{#if ownedSpacesCount !== null}
						<p class="text-xs text-[var(--color-muted)]">
							{t('spaces.quota_used', 'Spaces created')}: {ownedSpacesCount}
						</p>
					{/if}

					{#if error}
						<p class="text-sm text-red-500" role="alert">{error}</p>
					{/if}

					<div class="flex justify-end gap-2 pt-1">
						<button type="button" onclick={() => (open = false)} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">
							{t('spaces.create_cancel')}
						</button>
						<button type="submit" disabled={submitting} class="flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50">
							{#if submitting}
								<Loader2 size={14} class="animate-spin" />
								{t('spaces.creating', 'Creating…')}
							{:else}
								{t('spaces.create_submit')}
							{/if}
						</button>
					</div>
				</form>
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
