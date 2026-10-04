<script lang="ts">
	import { untrack, onMount } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import {
		Users,
		Plus,
		X,
		Loader2,
		BadgeCheck,
		ChevronUp,
		ChevronDown,
		MapPin,
		Globe,
		Mail,
		Navigation,
		CheckCircle2,
		Hand,
		Lock,
		MapPinOff,
		Map as MapIcon
	} from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton, PaginationControls, Modal } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { CommunityOrgWithAi, GeoCountry, GeoSpaceOption, OrgTypeOption } from '$lib/types';
	import OrgMap from '$lib/components/tools/OrgMap.svelte';
	import 'leaflet/dist/leaflet.css';
	import type { PageData } from './$types';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';

	/**
	 * Diaspora Community Locator tool — §101, ADR-088.
	 * Destination selector → server-side org filters (type / origin / city)
	 * → paginated list of community org cards. Authed users can submit a
	 * new org or claim an existing one.
	 */
	let { data }: { data: PageData } = $props();

	const ORG_PAGE_SIZE = 20;

	const title = $derived(t('community_locator.title', 'Diaspora Community Locator'));
	const description = $derived(
		t(
			'community_locator.subtitle',
			'Find diaspora organisations near your destination — cultural groups, religious communities, and social clubs.'
		)
	);
	const disabled = $derived(data.disabled);
	const authed = $derived(data.authed);

	let destinations = $state<GeoCountry[]>(untrack(() => data.destinations));
	let orgs = $state<CommunityOrgWithAi[]>(untrack(() => data.orgs));
	// BUG-005: org-type allow-list from the backend (Rule 16).
	let orgTypes = $state<OrgTypeOption[]>(untrack(() => data.orgTypes ?? []));

	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	// Server-side list filters — forwarded as ?type=&origin=&city= (§101).
	let typeFilter = $state<string>(untrack(() => data.initialType ?? ''));
	let originFilter = $state<string>(untrack(() => data.initialOrigin ?? ''));
	let cityFilter = $state<string>(untrack(() => data.initialCity ?? ''));
	let currentPage = $state<number>(untrack(() => data.initialPage ?? 1));
	let orgsTotal = $state<number>(untrack(() => data.orgsTotal ?? 0));
	let listError = $state<string | null>(null);
	let loading = $state(false);
	let expandedId = $state<string | null>(null);

	const totalPages = $derived(Math.max(1, Math.ceil(orgsTotal / ORG_PAGE_SIZE)));

	// City list for the submit form + city filter — loaded from the API when
	// a destination is selected (Rule 16: data from API, not hardcoded).
	let cities = $state<GeoSpaceOption[]>([]);

	// Submit-new-org form state (toggle section)
	let showSubmitForm = $state(false);
	let submitting = $state(false);
	let submitError = $state<string | null>(null);
	let submitSuccess = $state(false);
	let form = $state({
		name: '',
		org_type: '',
		origin_community: '',
		city_space_id: '',
		address: '',
		website_url: '',
		meeting_frequency: '',
		contact_info: ''
	});

	// Claim state — claimErrorId scopes the error to the org it came from
	// (previously a single error showed under every expanded card).
	let claimingId = $state<string | null>(null);
	let claimErrorId = $state<string | null>(null);
	let claimError = $state<string | null>(null);
	let claimedIds = $state<Set<string>>(new Set());

	// Registration prompt
	let showRegisterPrompt = $state(false);

	async function loadOrgs(pageNum = currentPage) {
		if (!selectedDestId) {
			orgs = [];
			orgsTotal = 0;
			return;
		}
		loading = true;
		listError = null;
		const qs = new URLSearchParams({
			dest: selectedDestId,
			limit: String(ORG_PAGE_SIZE),
			page: String(pageNum)
		});
		if (typeFilter) qs.set('type', typeFilter);
		if (originFilter.trim()) qs.set('origin', originFilter.trim());
		if (cityFilter) qs.set('city', cityFilter);
		const res = await api.getEnvelope<CommunityOrgWithAi[]>(`/community-locator?${qs}`);
		loading = false;
		if (res.error) {
			orgs = [];
			orgsTotal = 0;
			listError = res.error.message;
			return;
		}
		orgs = res.data ?? [];
		const pag = res.envelope?.pagination as { total?: number } | undefined;
		orgsTotal = pag?.total ?? orgs.length;
		currentPage = pageNum;
	}

	// BUG-B20: Load cities from the API when a destination is selected.
	// page_size=100 — the default (20) silently truncated larger cities lists.
	async function loadCities(destId: string) {
		if (!destId) {
			cities = [];
			return;
		}
		try {
			const res = await api.get<GeoSpaceOption[]>(`/spaces?level=city&parent=${destId}&page_size=100`);
			cities = res.data ?? [];
		} catch {
			cities = [];
		}
	}

	// Keep the URL in sync so filtered/paginated views are shareable.
	function syncUrl() {
		const url = new URL(page.url);
		const set = (k: string, v: string) => (v ? url.searchParams.set(k, v) : url.searchParams.delete(k));
		set('dest', selectedDestId);
		set('type', typeFilter);
		set('origin', originFilter.trim());
		set('city', cityFilter);
		set('page', currentPage > 1 ? String(currentPage) : '');
		replaceState(url, {});
	}

	function onDestChange(id: string) {
		selectedDestId = id;
		expandedId = null;
		form.city_space_id = '';
		// New destination — reset filters and pagination.
		typeFilter = '';
		originFilter = '';
		cityFilter = '';
		currentPage = 1;
		loadOrgs(1);
		loadCities(id);
		syncUrl();
	}

	function applyFilters() {
		currentPage = 1;
		expandedId = null;
		loadOrgs(1);
		syncUrl();
	}

	// Debounce the free-text origin filter so each keystroke does not fire
	// a request.
	let originTimer: ReturnType<typeof setTimeout> | null = null;
	function onOriginInput() {
		if (originTimer) clearTimeout(originTimer);
		originTimer = setTimeout(applyFilters, 350);
	}

	function onPageChange(p: number) {
		loadOrgs(p);
		syncUrl();
	}

	function toggleExpand(id: string) {
		expandedId = expandedId === id ? null : id;
	}

	async function submitOrg() {
		if (!authed) {
			showRegisterPrompt = true;
			return;
		}
		if (!form.name || !form.org_type || !form.origin_community || !selectedDestId) return;
		submitting = true;
		submitError = null;
		submitSuccess = false;
		const res = await api.post<CommunityOrgWithAi>('/community-locator', {
			name: form.name,
			org_type: form.org_type,
			origin_community: form.origin_community,
			destination_space_id: selectedDestId,
			city_space_id: form.city_space_id || undefined,
			address: form.address || undefined,
			website_url: form.website_url || undefined,
			meeting_frequency: form.meeting_frequency || undefined,
			contact_info: form.contact_info || undefined
		});
		submitting = false;
		if (res.error) {
			submitError = res.error.message;
			return;
		}
		// The submission lands in the moderation queue as status='pending' —
		// do NOT append it to the published list (it would vanish on reload).
		submitSuccess = true;
		form = {
			name: '',
			org_type: '',
			origin_community: '',
			city_space_id: '',
			address: '',
			website_url: '',
			meeting_frequency: '',
			contact_info: ''
		};
		showSubmitForm = false;
	}

	async function claimOrg(id: string) {
		if (!authed) {
			showRegisterPrompt = true;
			return;
		}
		claimingId = id;
		claimErrorId = null;
		claimError = null;
		const res = await api.post<{ claim_status: string }>(
			`/community-locator/${id}/claim`
		);
		claimingId = null;
		if (res.error) {
			claimErrorId = id;
			claimError = res.error.message;
			return;
		}
		if (res.data) {
			claimedIds = new Set([...claimedIds, id]);
		}
	}

	function mapsUrl(lat: number, lng: number): string {
		// ADR-109: No Google Maps dependency at any layer — use OpenStreetMap.
		return `https://www.openstreetmap.org/?mlat=${lat}&mlon=${lng}#map=16/${lat}/${lng}`;
	}

	function prettifyEnum(value: string): string {
		return value.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}
	function orgTypeLabel(value: string): string {
		return t(`community_locator.type_${value}`, prettifyEnum(value));
	}
	function claimStatusLabel(value: string): string {
		return t(`community_locator.claim_status_${value}`, prettifyEnum(value));
	}

	// On mount: hydrate the cities list for the SSR-selected destination.
	// The org list itself was fetched server-side.
	onMount(() => {
		if (selectedDestId) {
			loadCities(selectedDestId);
		}
	});

	// Autocomplete fetchers — filter the already-loaded destinations client-side.
	function fetchDestinations(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? destinations
			: destinations.filter((d) => d.name.toLowerCase().includes(query) || d.geo_code.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((d) => ({ id: d.id, label: d.name, sublabel: d.geo_code })));
	}
	function fetchDestinationsInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinations('');
	}
	const selectedDestLabel = $derived(
		selectedDestId ? destinations.find((d) => d.id === selectedDestId)?.name ?? null : null
	);

	// Org-type dropdown items — sourced from the backend allow-list (Rule 16).
	function fetchOrgTypes(q: string): Promise<AutocompleteItem[]> {
		const needle = q.trim().toLowerCase();
		const filtered = needle === ''
			? orgTypes
			: orgTypes.filter((o) => o.value.includes(needle) || orgTypeLabel(o.value).toLowerCase().includes(needle));
		return Promise.resolve(filtered.map((o) => ({ id: o.value, label: orgTypeLabel(o.value) })));
	}
	function fetchOrgTypesInitial(): Promise<AutocompleteItem[]> {
		return fetchOrgTypes('');
	}

	// City dropdown items — from the API-loaded cities for this destination.
	function fetchCities(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? cities
			: cities.filter((c) => c.name.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((c) => ({ id: c.id, label: c.name })));
	}
	function fetchCitiesInitial(): Promise<AutocompleteItem[]> {
		return fetchCities('');
	}
	function getCityLabel(id: string): string {
		return cities.find((c) => c.id === id)?.name ?? id;
	}
</script>

<svelte:head>
	<title>{title} — JapaConnect Tools</title>
	<meta
		name="description"
		content={t(
			'community_locator.seo_description',
			'Find diaspora organisations near your destination — cultural groups, religious communities, and social clubs.'
		)}
	/>
	<link rel="canonical" href={page.url.href} />
	<meta property="og:title" content="{title} — JapaConnect Tools" />
	<meta
		property="og:description"
		content={t(
			'community_locator.seo_description',
			'Find diaspora organisations near your destination — cultural groups, religious communities, and social clubs.'
		)}
	/>
	<meta property="og:url" content={page.url.href} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content="{title} — JapaConnect Tools" />
	<meta
		name="twitter:description"
		content={t(
			'community_locator.seo_description',
			'Find diaspora organisations near your destination — cultural groups, religious communities, and social clubs.'
		)}
	/>
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Users size={20} /> {title}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else if destinations.length === 0}
		<div class="p-4">
			<EmptyState
				icon={MapPinOff}
				title={t('community_locator.no_destinations', 'No destinations available')}
				body={t(
					'community_locator.no_destinations_desc',
					"Destination data hasn't been configured yet. Please check back later."
				)}
			/>
		</div>
	{:else}
		<!-- Destination selector (geo-aware pattern — first input) -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<label
				for="cl-dest"
				class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]"
			>
				{t('geo.destination', 'Destination:')}
			</label>
			<div class="mt-2">
				<Autocomplete
					placeholder={t('geo.select_destination', 'Select a country…')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinations}
					initialFetcher={fetchDestinationsInitial}
					selectedId={selectedDestId || null}
					selectedLabel={selectedDestLabel}
					onSelect={(item) => onDestChange(item.id)}
					onClear={() => onDestChange('')}
				/>
			</div>
		</div>

		{#if !selectedDestId}
			<div class="p-4">
				<EmptyState
					icon={MapIcon}
					title={t('community_locator.select_destination', 'Select a destination')}
					body={t(
						'community_locator.select_destination_desc',
						'Choose a country above to find diaspora organisations in that destination.'
					)}
				/>
			</div>
		{:else}
			<div class="p-4">
				<!-- Server-side filters — ?type=&origin=&city= per §101 -->
				<div class="mb-6 grid grid-cols-1 sm:grid-cols-3 gap-3">
					<div>
						<label for="cl-type-filter" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)] mb-1">
							{t('community_locator.field_org_type', 'Organisation type')}
						</label>
						<Autocomplete
							id="cl-type-filter"
							placeholder={t('community_locator.filter_type', 'All types…')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={fetchOrgTypes}
							initialFetcher={fetchOrgTypesInitial}
							selectedId={typeFilter || null}
							selectedLabel={typeFilter ? orgTypeLabel(typeFilter) : null}
							onSelect={(item) => { typeFilter = item.id; applyFilters(); }}
							onClear={() => { typeFilter = ''; applyFilters(); }}
						/>
					</div>
					<div>
						<label for="cl-origin-filter" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)] mb-1">
							{t('community_locator.field_origin_community', 'Origin community')}
						</label>
						<input
							id="cl-origin-filter"
							type="text"
							bind:value={originFilter}
							oninput={onOriginInput}
							placeholder={t('community_locator.filter_origin', 'e.g. Nigerian')}
							class="w-full h-10 px-3 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
						/>
					</div>
					<div>
						<label for="cl-city-filter" class="block text-xs font-medium uppercase tracking-wide text-[var(--color-muted)] mb-1">
							{t('community_locator.field_city', 'City')}
						</label>
						<Autocomplete
							id="cl-city-filter"
							placeholder={t('community_locator.filter_city', 'All cities…')}
							emptyMessage={t('community_locator.no_cities', 'No cities configured for this destination.')}
							fetcher={fetchCities}
							initialFetcher={fetchCitiesInitial}
							selectedId={cityFilter || null}
							selectedLabel={cityFilter ? getCityLabel(cityFilter) : null}
							onSelect={(item) => { cityFilter = item.id; applyFilters(); }}
							onClear={() => { cityFilter = ''; applyFilters(); }}
						/>
					</div>
				</div>

				{#if submitSuccess}
					<div class="mb-6 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-3 text-sm text-[var(--color-text)] flex items-center gap-2">
						<CheckCircle2 size={16} class="text-[var(--color-primary)]" />
						{t('community_locator.submit_success', 'Organisation submitted — it will appear here once reviewed by our moderators.')}
					</div>
				{/if}

				{#if orgs.some((o) => o.latitude != null && o.longitude != null)}
					<div class="mb-6">
						<OrgMap {orgs} />
					</div>
				{/if}

				<!-- Add organisation toggle -->
				<div class="mb-6 flex items-center justify-between">
					<h2 class="text-sm font-semibold text-[var(--color-muted)] uppercase tracking-wide">
						{t('community_locator.organisations', 'Organisations')}
					</h2>
					{#if authed}
						<button
							type="button"
							onclick={() => (showSubmitForm = !showSubmitForm)}
							class="inline-flex items-center gap-1.5 px-4 py-2 bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] text-sm font-medium hover:bg-[var(--color-primary-hover)] transition-colors"
						>
							{#if showSubmitForm}
								<X size={16} />
							{:else}
								<Plus size={16} />
							{/if}
							{showSubmitForm
								? t('community_locator.cancel_add', 'Cancel')
								: t('community_locator.add_organisation', 'Add organisation')}
						</button>
					{:else}
						<button
							type="button"
							onclick={() => (showRegisterPrompt = true)}
							class="inline-flex items-center gap-1.5 px-4 py-2 bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] text-sm font-medium hover:bg-[var(--color-primary-hover)] transition-colors"
						>
							<Plus size={16} />
							{t('community_locator.add_organisation', 'Add organisation')}
						</button>
					{/if}
				</div>

				<!-- Submit new org form (toggle section) -->
				{#if showSubmitForm && authed}
					<div class="mb-6 p-4 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)]">
						<h3 class="text-sm font-semibold text-[var(--color-text)] mb-4">
							{t('community_locator.submit_form_title', 'Submit a new organisation')}
						</h3>
						{#if submitError}
							<p class="text-sm text-red-500 mb-3">{submitError}</p>
						{/if}
						<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
							<div>
								<label for="form-name" class="block text-sm font-medium text-[var(--color-text)] mb-1">
									{t('community_locator.field_name', 'Name')} *
								</label>
								<input
									id="form-name"
									type="text"
									bind:value={form.name}
									class="w-full px-3 py-2 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
								/>
							</div>
							<div>
								<label for="form-org-type" class="block text-sm font-medium text-[var(--color-text)] mb-1">
									{t('community_locator.field_org_type', 'Organisation type')} *
								</label>
								<Autocomplete
									id="form-org-type"
									placeholder={t('common.select', 'Select…')}
									emptyMessage={t('common.no_results', 'No results found')}
									fetcher={fetchOrgTypes}
									initialFetcher={fetchOrgTypesInitial}
									selectedId={form.org_type || null}
									selectedLabel={form.org_type ? orgTypeLabel(form.org_type) : null}
									onSelect={(item) => (form.org_type = item.id)}
									onClear={() => (form.org_type = '')}
								/>
							</div>
							<div>
								<label for="form-origin" class="block text-sm font-medium text-[var(--color-text)] mb-1">
									{t('community_locator.field_origin_community', 'Origin community')} *
								</label>
								<input
									id="form-origin"
									type="text"
									bind:value={form.origin_community}
									placeholder={t('community_locator.field_origin_community_placeholder', 'e.g. Nigerian, Ghanaian, West African')}
									class="w-full px-3 py-2 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
								/>
							</div>
							<div>
								<label for="form-city" class="block text-sm font-medium text-[var(--color-text)] mb-1">
									{t('community_locator.field_city', 'City')}
								</label>
								<Autocomplete
									id="form-city"
									placeholder={t('community_locator.select_city', 'Select a city…')}
									emptyMessage={t('community_locator.no_cities', 'No cities configured for this destination.')}
									fetcher={fetchCities}
									initialFetcher={fetchCitiesInitial}
									selectedId={form.city_space_id || null}
									selectedLabel={form.city_space_id ? getCityLabel(form.city_space_id) : null}
									onSelect={(item) => (form.city_space_id = item.id)}
									onClear={() => (form.city_space_id = '')}
									disabled={cities.length === 0}
								/>
								{#if cities.length === 0 && selectedDestId}
									<p class="text-xs text-[var(--color-muted)] mt-1">{t('community_locator.no_cities', 'No cities configured for this destination.')}</p>
								{/if}
							</div>
							<div class="sm:col-span-2">
								<label for="form-address" class="block text-sm font-medium text-[var(--color-text)] mb-1">
									{t('community_locator.field_address', 'Address')}
								</label>
								<input
									id="form-address"
									type="text"
									bind:value={form.address}
									class="w-full px-3 py-2 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
								/>
							</div>
							<div>
								<label for="form-website" class="block text-sm font-medium text-[var(--color-text)] mb-1">
									{t('community_locator.field_website', 'Website URL')}
								</label>
								<input
									id="form-website"
									type="text"
									bind:value={form.website_url}
									placeholder="https://"
									class="w-full px-3 py-2 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
								/>
							</div>
							<div>
								<label for="form-frequency" class="block text-sm font-medium text-[var(--color-text)] mb-1">
									{t('community_locator.field_meeting_frequency', 'Meeting frequency')}
								</label>
								<input
									id="form-frequency"
									type="text"
									bind:value={form.meeting_frequency}
									placeholder={t('community_locator.field_meeting_frequency_placeholder', 'e.g. Weekly, Monthly')}
									class="w-full px-3 py-2 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
								/>
							</div>
							<div class="sm:col-span-2">
								<label for="form-contact" class="block text-sm font-medium text-[var(--color-text)] mb-1">
									{t('community_locator.field_contact_info', 'Contact info')}
								</label>
								<input
									id="form-contact"
									type="text"
									bind:value={form.contact_info}
									class="w-full px-3 py-2 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius)] text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
								/>
							</div>
						</div>
						<div class="mt-4 flex items-center gap-3">
							<button
								type="button"
								onclick={submitOrg}
								disabled={submitting || !form.name || !form.org_type || !form.origin_community}
								class="inline-flex items-center gap-1.5 px-4 py-2 bg-[var(--color-primary)] text-[var(--color-primary-contrast)] rounded-[var(--radius)] text-sm font-medium hover:bg-[var(--color-primary-hover)] transition-colors disabled:opacity-50"
							>
								{#if submitting}
									<Loader2 size={16} class="animate-spin" />
									{t('community_locator.submitting', 'Submitting...')}
								{:else}
									{t('community_locator.submit', 'Submit')}
								{/if}
							</button>
						</div>
					</div>
				{/if}

				<!-- Org list -->
				{#if loading}
					<div class="space-y-3">
						<LoadingSkeleton variant="card" count={3} />
					</div>
				{:else if listError}
					<EmptyState
						icon={MapPinOff}
						title={t('common.error', 'Something went wrong')}
						body={listError}
					/>
				{:else if orgs.length === 0}
					<EmptyState
						icon={Users}
						title={t('community_locator.no_orgs', 'No organisations')}
						body={t(
							'community_locator.no_orgs_desc',
							'No diaspora organisations have been listed for this destination yet.'
						)}
					/>
				{:else}
					<div class="space-y-3">
						{#each orgs as org (org.id)}
							<div class="bg-[var(--color-surface)] border border-[var(--color-border)] rounded-[var(--radius-card)] overflow-hidden">
								<!-- Card header (click to expand) -->
								<button
									type="button"
									onclick={() => toggleExpand(org.id)}
									class="w-full p-4 text-left flex items-start justify-between gap-3 hover:bg-[var(--color-surface-subtle)] transition-colors"
								>
									<div class="min-w-0">
										<div class="flex items-center gap-2 flex-wrap">
											<span class="text-sm font-semibold text-[var(--color-text)]">{org.name}</span>
											{#if org.is_verified}
												<span class="inline-flex items-center gap-1 text-xs px-1.5 py-0.5 bg-[var(--color-primary)]/10 text-[var(--color-primary)] rounded">
													<BadgeCheck size={12} />
													{t('community_locator.verified', 'Verified')}
												</span>
											{/if}
											{#if org.is_community_submitted}
												<span class="text-xs px-1.5 py-0.5 bg-[var(--color-surface-subtle)] text-[var(--color-muted)] rounded">
													{t('community_locator.community_submitted', 'Community')}
												</span>
											{/if}
										</div>
										<p class="text-sm text-[var(--color-muted)] mt-1">
											{orgTypeLabel(org.org_type)}
											{#if org.origin_community}
												· {org.origin_community}
											{/if}
											{#if org.city_space_name}
												· {org.city_space_name}
											{/if}
										</p>
										{#if org.meeting_frequency}
											<p class="text-xs text-[var(--color-muted)] mt-0.5">
												{t('community_locator.meets', 'Meets:')} {org.meeting_frequency}
											</p>
										{/if}
									</div>
									{#if expandedId === org.id}
										<ChevronUp size={18} class="text-[var(--color-muted)] flex-shrink-0 mt-1" />
									{:else}
										<ChevronDown size={18} class="text-[var(--color-muted)] flex-shrink-0 mt-1" />
									{/if}
								</button>

								<!-- Expanded details -->
								{#if expandedId === org.id}
									<div class="px-4 pb-4 border-t border-[var(--color-border)] pt-3 space-y-2">
										{#if org.address}
											<div class="flex items-start gap-2 text-sm">
												<MapPin size={16} class="text-[var(--color-muted)] flex-shrink-0 mt-0.5" />
												<span class="text-[var(--color-text)]">{org.address}</span>
											</div>
										{/if}
										{#if org.website_url}
											<div class="flex items-start gap-2 text-sm">
												<Globe size={16} class="text-[var(--color-muted)] flex-shrink-0 mt-0.5" />
												<a
													href={org.website_url}
													target="_blank"
													rel="noopener noreferrer"
													class="text-[var(--color-primary)] hover:underline"
												>
													{org.website_url}
												</a>
											</div>
										{/if}
										{#if org.contact_info}
											<div class="flex items-start gap-2 text-sm">
												<Mail size={16} class="text-[var(--color-muted)] flex-shrink-0 mt-0.5" />
												<span class="text-[var(--color-text)]">{org.contact_info}</span>
											</div>
										{/if}
										{#if org.latitude != null && org.longitude != null}
											<div class="flex items-start gap-2 text-sm">
												<Navigation size={16} class="text-[var(--color-muted)] flex-shrink-0 mt-0.5" />
												<a
													href={mapsUrl(org.latitude, org.longitude)}
													target="_blank"
													rel="noopener noreferrer"
													class="text-[var(--color-primary)] hover:underline"
												>
													{t('community_locator.view_on_map', 'View on map')}
												</a>
											</div>
										{/if}
										<div class="flex items-center gap-2 text-xs text-[var(--color-muted)] pt-1">
											<span>
												{t('community_locator.claim_status', 'Claim status:')} {claimStatusLabel(org.claim_status)}
											</span>
										</div>
										<a
											href="/tools/community-locator/{org.slug}"
											class="inline-flex items-center gap-1.5 text-sm text-[var(--color-primary)] hover:underline"
										>
											{t('common.view_details', 'View details')}
										</a>

									{#if org.claim_status === 'claimed' && (org.japaconnect_space_slug || org.japaconnect_space_id)}
										<div class="pt-2 border-t border-[var(--color-border)] mt-2">
											<p class="text-xs font-medium uppercase tracking-wider text-[var(--color-muted)] mb-1">
												{t('community_locator.space_integration', 'JapaConnect Space')}
											</p>
											<a
												href={`/spaces/${org.japaconnect_space_slug || org.japaconnect_space_id}`}
												class="inline-flex items-center gap-1.5 text-sm text-[var(--color-primary)] hover:underline"
											>
												<Users size={14} />
												{t('community_locator.view_space', 'View Space — posts, members & events')}
											</a>
										</div>
									{/if}

										<!-- Claim action — only meaningful while unclaimed -->
										<div class="pt-2">
											{#if claimedIds.has(org.id)}
												<span class="inline-flex items-center gap-1.5 text-sm text-[var(--color-primary)]">
													<CheckCircle2 size={16} />
													{t('community_locator.claimed_locally', 'Claim submitted — pending review')}
												</span>
											{:else if org.claim_status === 'claimed'}
												<span class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)]">
													<CheckCircle2 size={16} />
													{t('community_locator.claim_status_claimed', 'Claimed')}
												</span>
											{:else if org.claim_status === 'pending'}
												<span class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)]">
													<CheckCircle2 size={16} />
													{t('community_locator.claim_status_pending', 'Claim pending review')}
												</span>
											{:else if authed}
												<button
													type="button"
													onclick={() => claimOrg(org.id)}
													disabled={claimingId === org.id}
													class="inline-flex items-center gap-1.5 px-3 py-1.5 border border-[var(--color-border)] rounded-[var(--radius)] text-sm font-medium text-[var(--color-text)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary)] transition-colors disabled:opacity-50"
												>
													{#if claimingId === org.id}
														<Loader2 size={14} class="animate-spin" />
														{t('community_locator.claiming', 'Claiming...')}
													{:else}
														<Hand size={14} />
														{t('community_locator.claim', 'Claim this organisation')}
													{/if}
												</button>
												{#if claimError && claimErrorId === org.id}
													<span class="text-xs text-red-500 ml-2">{claimError}</span>
												{/if}
											{:else}
												<button
													type="button"
													onclick={() => (showRegisterPrompt = true)}
													class="inline-flex items-center gap-1.5 px-3 py-1.5 border border-[var(--color-border)] rounded-[var(--radius)] text-sm font-medium text-[var(--color-text)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary)] transition-colors"
												>
													<Hand size={14} />
													{t('community_locator.claim', 'Claim this organisation')}
												</button>
											{/if}
										</div>
									</div>
								{/if}
							</div>
						{/each}
					</div>

					{#if totalPages > 1}
						<div class="mt-6 flex justify-center">
							<PaginationControls
								page={currentPage}
								{totalPages}
								onPageChange={onPageChange}
							/>
						</div>
					{/if}
				{/if}
			</div>
		{/if}
	{/if}
</div>

<!-- Register prompt for unauthenticated claim/submit attempts -->
<Modal bind:open={showRegisterPrompt} title={t('community_locator.register_prompt_title', 'Create a free account')}>
	<p class="text-sm text-[var(--color-muted)]">
		{t(
			'community_locator.register_prompt_body',
			'You need an account to submit or claim organisations.'
		)}
	</p>
	<div class="mt-4 flex items-center justify-end gap-2">
		<a
			href="/login"
			class="px-4 py-2 text-sm font-medium border border-[var(--color-border)] text-[var(--color-text)] rounded-[var(--radius)] hover:bg-[var(--color-surface-subtle)]"
		>
			{t('nav.login', 'Log in')}
		</a>
		<a
			href="/register"
			class="px-4 py-2 text-sm font-medium bg-[var(--color-primary)] text-white rounded-[var(--radius)] hover:bg-[var(--color-primary-hover)]"
		>
			{t('nav.register', 'Register')}
		</a>
	</div>
</Modal>
