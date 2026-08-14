<script lang="ts">
	import { untrack, onMount } from 'svelte';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { mainAppUrl, buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import OrgMap from '$lib/components/tools/OrgMap.svelte';
	import type { CommunityOrgWithAi, GeoSpaceOption } from '$lib/types';
	import 'leaflet/dist/leaflet.css';

	/**
	 * Diaspora Community Locator tool — §101, ADR-088.
	 * Destination selector → optional org_type filter → list of community org
	 * cards. Authed users can submit a new org or claim an existing one.
	 */
	let { data } = $props();

	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let orgs = $state<CommunityOrgWithAi[]>(untrack(() => data.orgs));
	let user = $state(untrack(() => data.user));

	let selectedDestId = $state<string>(untrack(() => data.initialDestId ?? ''));
	let orgTypeFilter = $state('');
	let loading = $state(false);
	let expandedId = $state<string | null>(null);

	// City list for the submit form — loaded from the API when a
	// destination is selected (Rule 16: data from API, not hardcoded).
	let cities = $state<GeoSpaceOption[]>([]);

	// Submit-new-org form state (toggle section)
	let showSubmitForm = $state(false);
	let submitting = $state(false);
	let submitError = $state<string | null>(null);
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

	// Claim state
	let claimingId = $state<string | null>(null);
	let claimError = $state<string | null>(null);
	let claimedIds = $state<Set<string>>(new Set());

	// Registration prompt
	let showRegisterPrompt = $state(false);

	const seo = $derived(
		buildSeoMeta({
			title: t(
				'community_locator.seo_title',
				'Diaspora Community Locator — JapaConnect Tools'
			),
			description: t(
				'community_locator.seo_description',
				'Find diaspora organisations near your destination — cultural groups, religious communities, and social clubs.'
			),
			path: '/community-locator',
			origin: page.url.origin
		})
	);

	// Filtered orgs by the optional org_type text filter (client-side).
	const filteredOrgs = $derived.by(() => {
		const q = orgTypeFilter.trim().toLowerCase();
		if (!q) return orgs;
		return orgs.filter(
			(o) =>
				o.org_type.toLowerCase().includes(q) ||
				o.origin_community.toLowerCase().includes(q)
		);
	});

	async function loadOrgs() {
		if (!selectedDestId) {
			orgs = [];
			return;
		}
		loading = true;
		const res = await api.get<CommunityOrgWithAi[]>(
			`/community-locator?dest=${selectedDestId}`
		);
		loading = false;
		if (!res.error) orgs = res.data ?? [];
		else orgs = [];
	}

	// BUG-B20: Load cities from the API when a destination is selected,
	// so the submit form's city field is a select populated from
	// /spaces?level=city&parent={destId} instead of a raw UUID text input.
	async function loadCities(destId: string) {
		if (!destId) {
			cities = [];
			return;
		}
		try {
			const res = await api.get<GeoSpaceOption[]>(`/spaces?level=city&parent=${destId}`);
			cities = res.data ?? [];
		} catch {
			cities = [];
		}
	}

	function onDestChange(id: string) {
		selectedDestId = id;
		expandedId = null;
		form.city_space_id = '';
		// Update URL for shareability (SvelteKit replaceState, not window.history)
		const url = new URL(page.url);
		if (id) url.searchParams.set('dest', id);
		else url.searchParams.delete('dest');
		replaceState(url, {});
		loadOrgs();
		loadCities(id);
	}

	function toggleExpand(id: string) {
		expandedId = expandedId === id ? null : id;
	}

	async function submitOrg() {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		if (!form.name || !form.org_type || !form.origin_community || !selectedDestId) return;
		submitting = true;
		submitError = null;
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
		if (res.data) {
			orgs = [...orgs, res.data];
		}
		// Reset form
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
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		claimingId = id;
		claimError = null;
		const res = await api.post<{ claim_status: string }>(
			`/community-locator/${id}/claim`
		);
		claimingId = null;
		if (res.error) {
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

	// On mount: if an initial dest was loaded server-side, no client fetch is
	// needed. If there's no initial dest but one is present in state (e.g. from
	// a shared URL without SSR match), load it. Uses onMount so it runs once.
	onMount(() => {
		if (selectedDestId && orgs.length === 0) {
			loadOrgs();
		}
		if (selectedDestId) {
			loadCities(selectedDestId);
		}
	});
</script>

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	<meta property="og:url" content={seo.canonical} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={seo.title} />
	<meta name="twitter:description" content={seo.description} />
</svelte:head>

<div class="mx-auto max-w-5xl px-4 py-8">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
		<div class="mb-8">
			<h1 class="text-2xl sm:text-3xl font-bold text-text">
				{t('community_locator.title', 'Diaspora Community Locator')}
			</h1>
			<p class="mt-2 text-text-subtle">
				{t(
					'community_locator.subtitle',
					'Find diaspora organisations near your destination — cultural groups, religious communities, and social clubs.'
				)}
			</p>
		</div>

		{#if destinations.length === 0}
			<EmptyState
				icon="map-pin-off"
				title={t('community_locator.no_destinations', 'No destinations available')}
				description={t(
					'community_locator.no_destinations_desc',
					"Destination data hasn't been configured yet. Please check back later."
				)}
			/>
		{:else}
			<!-- Destination selector (geo-aware pattern — first input) -->
			<div class="mb-6">
				<DestinationSelector {destinations} bind:selectedId={selectedDestId} onchange={onDestChange} />
			</div>

			{#if !selectedDestId}
				<EmptyState
					icon="map"
					title={t('community_locator.select_destination', 'Select a destination')}
					description={t(
						'community_locator.select_destination_desc',
						'Choose a country above to find diaspora organisations in that destination.'
					)}
				/>
			{:else}
				<!-- Optional org_type / origin filter -->
				<div class="mb-6 flex items-center gap-3">
					<label for="org-type-filter" class="text-sm font-medium text-text whitespace-nowrap">
						{t('community_locator.filter', 'Filter:')}
					</label>
					<input
						id="org-type-filter"
						type="text"
						bind:value={orgTypeFilter}
						placeholder={t('community_locator.filter_placeholder', 'Organisation type or community…')}
						class="flex-1 px-4 py-2 bg-surface border border-border rounded-lg text-text focus:border-primary focus:outline-none"
					/>
				</div>

				{#if filteredOrgs.some((o) => o.latitude != null && o.longitude != null)}
					<div class="mb-6">
						<OrgMap orgs={filteredOrgs} />
					</div>
				{/if}

				<!-- Add organisation toggle -->
				<div class="mb-6 flex items-center justify-between">
					<h2 class="text-sm font-semibold text-muted uppercase tracking-wide">
						{t('community_locator.organisations', 'Organisations')}
					</h2>
					{#if user}
						<button
							onclick={() => (showSubmitForm = !showSubmitForm)}
							class="inline-flex items-center gap-1.5 px-4 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors"
						>
							<Icon name={showSubmitForm ? 'x' : 'plus'} size={16} />
							{showSubmitForm
								? t('community_locator.cancel_add', 'Cancel')
								: t('community_locator.add_organisation', 'Add organisation')}
						</button>
					{:else}
						<button
							onclick={() => (showRegisterPrompt = true)}
							class="inline-flex items-center gap-1.5 px-4 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors"
						>
							<Icon name="plus" size={16} />
							{t('community_locator.add_organisation', 'Add organisation')}
						</button>
					{/if}
				</div>

				<!-- Submit new org form (toggle section) -->
				{#if showSubmitForm && user}
					<div class="mb-6 p-4 bg-surface border border-border rounded-lg">
						<h3 class="text-sm font-semibold text-text mb-4">
							{t('community_locator.submit_form_title', 'Submit a new organisation')}
						</h3>
						{#if submitError}
							<p class="text-sm text-red-500 mb-3">{submitError}</p>
						{/if}
						<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
							<div>
								<label for="form-name" class="block text-sm font-medium text-text mb-1">
									{t('community_locator.field_name', 'Name')} *
								</label>
								<input
									id="form-name"
									type="text"
									bind:value={form.name}
									class="w-full px-3 py-2 bg-surface border border-border rounded text-text focus:border-primary focus:outline-none"
								/>
							</div>
							<div>
								<label for="form-org-type" class="block text-sm font-medium text-text mb-1">
									{t('community_locator.field_org_type', 'Organisation type')} *
								</label>
								<select
									id="form-org-type"
									bind:value={form.org_type}
									class="w-full px-3 py-2 bg-surface border border-border rounded text-text focus:border-primary focus:outline-none"
								>
									<option value="">{t('common.select', 'Select…')}</option>
									<option value="cultural_association">{t('community_locator.type_cultural_association', 'Cultural Association')}</option>
									<option value="church">{t('community_locator.type_church', 'Church')}</option>
									<option value="mosque">{t('community_locator.type_mosque', 'Mosque')}</option>
									<option value="professional_network">{t('community_locator.type_professional_network', 'Professional Network')}</option>
									<option value="student_union">{t('community_locator.type_student_union', 'Student Union')}</option>
									<option value="informal_group">{t('community_locator.type_informal_group', 'Informal Group')}</option>
									<option value="other">{t('community_locator.type_other', 'Other')}</option>
								</select>
							</div>
							<div>
								<label for="form-origin" class="block text-sm font-medium text-text mb-1">
									{t('community_locator.field_origin_community', 'Origin community')} *
								</label>
								<input
									id="form-origin"
									type="text"
									bind:value={form.origin_community}
									placeholder={t('community_locator.field_origin_community_placeholder', 'e.g. Nigerian, Ghanaian, West African')}
									class="w-full px-3 py-2 bg-surface border border-border rounded text-text focus:border-primary focus:outline-none"
								/>
							</div>
							<div>
								<label for="form-city" class="block text-sm font-medium text-text mb-1">
									{t('community_locator.field_city', 'City')}
								</label>
								<select
									id="form-city"
									bind:value={form.city_space_id}
									disabled={cities.length === 0}
									class="w-full px-3 py-2 bg-surface border border-border rounded text-text focus:border-primary focus:outline-none disabled:opacity-50"
								>
									<option value="">{t('community_locator.select_city', 'Select a city…')}</option>
									{#each cities as city (city.id)}
										<option value={city.id}>{city.name}</option>
									{/each}
								</select>
								{#if cities.length === 0 && selectedDestId}
									<p class="text-xs text-muted mt-1">{t('community_locator.no_cities', 'No cities configured for this destination.')}</p>
								{/if}
							</div>
							<div class="sm:col-span-2">
								<label for="form-address" class="block text-sm font-medium text-text mb-1">
									{t('community_locator.field_address', 'Address')}
								</label>
								<input
									id="form-address"
									type="text"
									bind:value={form.address}
									class="w-full px-3 py-2 bg-surface border border-border rounded text-text focus:border-primary focus:outline-none"
								/>
							</div>
							<div>
								<label for="form-website" class="block text-sm font-medium text-text mb-1">
									{t('community_locator.field_website', 'Website URL')}
								</label>
								<input
									id="form-website"
									type="text"
									bind:value={form.website_url}
									placeholder="https://"
									class="w-full px-3 py-2 bg-surface border border-border rounded text-text focus:border-primary focus:outline-none"
								/>
							</div>
							<div>
								<label for="form-frequency" class="block text-sm font-medium text-text mb-1">
									{t('community_locator.field_meeting_frequency', 'Meeting frequency')}
								</label>
								<input
									id="form-frequency"
									type="text"
									bind:value={form.meeting_frequency}
									placeholder={t('community_locator.field_meeting_frequency_placeholder', 'e.g. Weekly, Monthly')}
									class="w-full px-3 py-2 bg-surface border border-border rounded text-text focus:border-primary focus:outline-none"
								/>
							</div>
							<div class="sm:col-span-2">
								<label for="form-contact" class="block text-sm font-medium text-text mb-1">
									{t('community_locator.field_contact_info', 'Contact info')}
								</label>
								<input
									id="form-contact"
									type="text"
									bind:value={form.contact_info}
									class="w-full px-3 py-2 bg-surface border border-border rounded text-text focus:border-primary focus:outline-none"
								/>
							</div>
						</div>
						<div class="mt-4 flex items-center gap-3">
							<button
								onclick={submitOrg}
								disabled={submitting || !form.name || !form.org_type || !form.origin_community}
								class="inline-flex items-center gap-1.5 px-4 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50"
							>
								{#if submitting}
									<Icon name="loader-2" size={16} class="animate-spin" />
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
						<LoadingSkeleton height="80px" />
						<LoadingSkeleton height="80px" />
						<LoadingSkeleton height="80px" />
					</div>
				{:else if filteredOrgs.length === 0}
					<EmptyState
						icon="users"
						title={t('community_locator.no_orgs', 'No organisations')}
						description={t(
							'community_locator.no_orgs_desc',
							'No diaspora organisations have been listed for this destination yet.'
						)}
					/>
				{:else}
					<div class="space-y-3">
						{#each filteredOrgs as org (org.id)}
							<div class="bg-surface border border-border rounded-lg overflow-hidden">
								<!-- Card header (click to expand) -->
								<button
									onclick={() => toggleExpand(org.id)}
									class="w-full p-4 text-left flex items-start justify-between gap-3 hover:bg-surface-subtle transition-colors"
								>
									<div class="min-w-0">
										<div class="flex items-center gap-2 flex-wrap">
											<span class="text-sm font-semibold text-text">{org.name}</span>
											{#if org.is_verified}
												<span class="inline-flex items-center gap-1 text-xs px-1.5 py-0.5 bg-[var(--tool-accent)]/10 text-[var(--tool-accent)] rounded">
													<Icon name="badge-check" size={12} />
													{t('community_locator.verified', 'Verified')}
												</span>
											{/if}
											{#if org.is_community_submitted}
												<span class="text-xs px-1.5 py-0.5 bg-surface-subtle text-muted rounded">
													{t('community_locator.community_submitted', 'Community')}
												</span>
											{/if}
										</div>
										<p class="text-sm text-muted mt-1">
											{org.org_type}
											{#if org.origin_community}
												· {org.origin_community}
											{/if}
											{#if org.city_space_name}
												· {org.city_space_name}
											{/if}
										</p>
										{#if org.meeting_frequency}
											<p class="text-xs text-text-subtle mt-0.5">
												{t('community_locator.meets', 'Meets:')} {org.meeting_frequency}
											</p>
										{/if}
									</div>
									<Icon
										name={expandedId === org.id ? 'chevron-up' : 'chevron-down'}
										size={18}
										class="text-muted flex-shrink-0 mt-1"
									/>
								</button>

								<!-- Expanded details -->
								{#if expandedId === org.id}
									<div class="px-4 pb-4 border-t border-border pt-3 space-y-2">
										{#if org.address}
											<div class="flex items-start gap-2 text-sm">
												<Icon name="map-pin" size={16} class="text-muted flex-shrink-0 mt-0.5" />
												<span class="text-text">{org.address}</span>
											</div>
										{/if}
										{#if org.website_url}
											<div class="flex items-start gap-2 text-sm">
												<Icon name="globe" size={16} class="text-muted flex-shrink-0 mt-0.5" />
												<a
													href={org.website_url}
													target="_blank"
													rel="noopener noreferrer"
													class="text-[var(--tool-accent)] hover:underline"
												>
													{org.website_url}
												</a>
											</div>
										{/if}
										{#if org.contact_info}
											<div class="flex items-start gap-2 text-sm">
												<Icon name="mail" size={16} class="text-muted flex-shrink-0 mt-0.5" />
												<span class="text-text">{org.contact_info}</span>
											</div>
										{/if}
										{#if org.latitude != null && org.longitude != null}
											<div class="flex items-start gap-2 text-sm">
												<Icon name="navigation" size={16} class="text-muted flex-shrink-0 mt-0.5" />
												<a
													href={mapsUrl(org.latitude, org.longitude)}
													target="_blank"
													rel="noopener noreferrer"
													class="text-[var(--tool-accent)] hover:underline"
												>
													{t('community_locator.view_on_map', 'View on map')}
												</a>
											</div>
										{/if}
										<div class="flex items-center gap-2 text-xs text-muted pt-1">
											<span>
												{t('community_locator.claim_status', 'Claim status:')} {org.claim_status}
											</span>
										</div>

									{#if org.claim_status === 'claimed' && org.japaconnect_space_id}
										<div class="pt-2 border-t border-border mt-2">
											<p class="text-xs font-medium uppercase tracking-wider text-muted mb-1">
												{t('community_locator.space_integration', 'JapaConnect Space')}
											</p>
											<a
												href={`/spaces/${org.japaconnect_space_id}`}
												class="inline-flex items-center gap-1.5 text-sm text-[var(--tool-accent)] hover:underline"
											>
												<Icon name="users" size={14} />
												{t('community_locator.view_space', 'View Space — posts, members & events')}
											</a>
										</div>
									{/if}

										<!-- Claim button -->
										<div class="pt-2">
											{#if claimedIds.has(org.id)}
												<span class="inline-flex items-center gap-1.5 text-sm text-[var(--tool-accent)]">
													<Icon name="check-circle" size={16} />
													{t('community_locator.claim_submitted', 'Claim submitted')}
												</span>
											{:else if user}
												<button
													onclick={() => claimOrg(org.id)}
													disabled={claimingId === org.id}
													class="inline-flex items-center gap-1.5 px-3 py-1.5 border border-border rounded-lg text-sm font-medium text-text hover:border-[var(--tool-accent)] hover:text-[var(--tool-accent)] transition-colors disabled:opacity-50"
												>
													{#if claimingId === org.id}
														<Icon name="loader-2" size={14} class="animate-spin" />
														{t('community_locator.claiming', 'Claiming...')}
													{:else}
														<Icon name="hand" size={14} />
														{t('community_locator.claim', 'Claim this organisation')}
													{/if}
												</button>
												{#if claimError && claimingId === null}
													<span class="text-xs text-red-500 ml-2">{claimError}</span>
												{/if}
											{:else}
												<button
													onclick={() => (showRegisterPrompt = true)}
													class="inline-flex items-center gap-1.5 px-3 py-1.5 border border-border rounded-lg text-sm font-medium text-text hover:border-[var(--tool-accent)] hover:text-[var(--tool-accent)] transition-colors"
												>
													<Icon name="hand" size={14} />
													{t('community_locator.claim', 'Claim this organisation')}
												</button>
											{/if}
										</div>
									</div>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			{/if}
		{/if}

		{#if showRegisterPrompt}
			<RegistrationPrompt
				open={showRegisterPrompt}
				onClose={() => (showRegisterPrompt = false)}
				icon="users"
				title={t('register.community_locator', 'Join the community')}
				description={t(
					'register.community_locator_desc',
					'Create a free account to submit new diaspora organisations and claim existing ones.'
				)}
			/>
		{/if}
	{/if}
</div>
