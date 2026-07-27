<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Button } from "$lib/components/ui/button";
	import { adSlotTypesApi, adsAdminApi, geoApi, spacesAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { goto } from "$app/navigation";
	import { ArrowLeft, Save } from "lucide-svelte";

	type SlotType = { id: string; name: string; description?: string; width: number; height: number };
	type GeoSpace = { id: string; name: string; slug: string; geo_code?: string | null; level: string };

	const canManage = $derived(hasPermission('ad.campaign.manage'));

	let name = $state('');
	let slotTypeName = $state('');
	let targetUrl = $state('');
	let budget = $state('');
	let priority = $state(0);
	let frequencyCap = $state('');
	let startDate = $state('');
	let endDate = $state('');
	let contentHeadline = $state('');
	let contentBody = $state('');
	let contentCta = $state('');

	// Geo targeting
	let targetGeo = $state<string[]>([]);
	let targetSpaceIds = $state<string[]>([]);
	let geoInput = $state('');
	let countries = $state<GeoSpace[]>([]);
	let geoSpaces = $state<GeoSpace[]>([]);
	let spaceSearch = $state('');

	let slotTypes = $state<SlotType[]>([]);
	let saving = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');

	const filteredSpaces = $derived(
		spaceSearch
			? geoSpaces.filter((s) => s.name.toLowerCase().includes(spaceSearch.toLowerCase()) || s.slug.toLowerCase().includes(spaceSearch.toLowerCase()))
			: geoSpaces.slice(0, 50)
	);

	$effect(() => {
		(async () => {
			try {
				const [slotsResult, countriesResult, spacesResult] = await Promise.all([
					adSlotTypesApi.list(),
					geoApi.listCountries(),
					spacesAdminApi.list('level=country&limit=200')
				]);
				if (slotsResult.ok) {
					const data = slotsResult.data as { data?: SlotType[] } | SlotType[];
					slotTypes = Array.isArray(data) ? data : (data.data ?? []);
				}
				if (countriesResult.ok) {
					countries = (countriesResult.data as GeoSpace[]) ?? [];
				}
				if (spacesResult.ok) {
					const data = spacesResult.data as { data?: GeoSpace[] } | GeoSpace[];
					geoSpaces = Array.isArray(data) ? data : (data.data ?? []);
				}
			} catch {
				errorMessage = t('ads.load_error', 'Failed to load form data');
			}
		})();
	});

	function addGeoCode() {
		const code = geoInput.trim().toUpperCase();
		if (code && !targetGeo.includes(code)) {
			targetGeo = [...targetGeo, code];
		}
		geoInput = '';
	}

	function removeGeoCode(code: string) {
		targetGeo = targetGeo.filter((c) => c !== code);
	}

	function toggleSpace(id: string) {
		if (targetSpaceIds.includes(id)) {
			targetSpaceIds = targetSpaceIds.filter((s) => s !== id);
		} else {
			targetSpaceIds = [...targetSpaceIds, id];
		}
	}

	async function save() {
		if (!name || !slotTypeName) {
			errorMessage = t('ads.name_required', 'Name and slot type are required');
			return;
		}
		saving = true;
		errorMessage = '';
		successMessage = '';
		try {
			const body: Record<string, unknown> = {
				name,
				slot_type_name: slotTypeName,
				content: {
					headline: contentHeadline,
					body: contentBody,
					cta: contentCta
				},
				priority: Number(priority) || 0
			};
			if (targetUrl) body.target_url = targetUrl;
			if (budget) body.budget = Number(budget);
			if (frequencyCap) body.frequency_cap_per_user = Number(frequencyCap);
			if (startDate) body.start_date = new Date(startDate).toISOString();
			if (endDate) body.end_date = new Date(endDate).toISOString();
			if (targetGeo.length > 0) body.target_geo = targetGeo;
			if (targetSpaceIds.length > 0) body.target_space_ids = targetSpaceIds;

			const result = await adsAdminApi.createCampaign(body);
			if (result.ok) {
				successMessage = t('ads.created', 'Campaign created successfully');
				setTimeout(() => goto('/ads'), 1000);
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('ads.create_error', 'Failed to create campaign');
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>{t('ads.new', 'New Campaign')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('ads.new', 'New Campaign')}
	description={t('ads.new_desc', 'Create a new ad campaign with optional geo targeting')}
	actions={[
		{ label: t('common.back', 'Back'), icon: ArrowLeft, href: '/ads' }
	]}
/>

{#if !canManage}
	<div class="rounded-lg border p-6 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<p class="text-sm" style="color: var(--color-muted)">{t('ads.no_permission', 'You need ad.campaign.manage permission to create campaigns.')}</p>
	</div>
{:else}
	<div class="mx-auto max-w-3xl space-y-6">
		{#if errorMessage}
			<div class="rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
				{errorMessage}
			</div>
		{/if}
		{#if successMessage}
			<div class="rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
				{successMessage}
			</div>
		{/if}

		<!-- Basic Info -->
		<div class="rounded-lg border p-6 space-y-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('ads.basic_info', 'Basic Information')}</h2>

			<div>
				<label for="ad-name" class="mb-1 block text-sm font-medium">{t('ads.campaign_name', 'Campaign Name')} *</label>
				<input id="ad-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={name} placeholder="Summer Travel Promo" />
			</div>

			<div>
				<label for="ad-slot" class="mb-1 block text-sm font-medium">{t('ads.slot_type', 'Slot Type')} *</label>
				<select id="ad-slot" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={slotTypeName}>
					<option value="">{t('ads.select_slot', 'Select a slot type...')}</option>
					{#each slotTypes as slot}
						<option value={slot.name}>{slot.name} ({slot.width}x{slot.height})</option>
					{/each}
				</select>
			</div>

			<div>
				<label for="ad-url" class="mb-1 block text-sm font-medium">{t('ads.target_url', 'Target URL')}</label>
				<input id="ad-url" type="url" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={targetUrl} placeholder="https://..." />
			</div>
		</div>

		<!-- Content -->
		<div class="rounded-lg border p-6 space-y-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('ads.content', 'Ad Content')}</h2>

			<div>
				<label for="ad-headline" class="mb-1 block text-sm font-medium">{t('ads.headline', 'Headline')}</label>
				<input id="ad-headline" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={contentHeadline} placeholder="Discover Nigeria" />
			</div>

			<div>
				<label for="ad-body" class="mb-1 block text-sm font-medium">{t('ads.body', 'Body Text')}</label>
				<textarea id="ad-body" rows="3" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={contentBody} placeholder="Explore the vibrant culture and landscapes..."></textarea>
			</div>

			<div>
				<label for="ad-cta" class="mb-1 block text-sm font-medium">{t('ads.cta', 'Call to Action')}</label>
				<input id="ad-cta" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={contentCta} placeholder="Learn More" />
			</div>
		</div>

		<!-- Budget & Schedule -->
		<div class="rounded-lg border p-6 space-y-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('ads.budget_schedule', 'Budget & Schedule')}</h2>

			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="ad-budget" class="mb-1 block text-sm font-medium">{t('ads.budget', 'Budget (credits)')}</label>
					<input id="ad-budget" type="number" min="0" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={budget} placeholder="1000" />
				</div>
				<div>
					<label for="ad-priority" class="mb-1 block text-sm font-medium">{t('ads.priority', 'Priority')}</label>
					<input id="ad-priority" type="number" min="0" max="100" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={priority} />
				</div>
				<div>
					<label for="ad-freq" class="mb-1 block text-sm font-medium">{t('ads.frequency_cap', 'Frequency Cap / User')}</label>
					<input id="ad-freq" type="number" min="0" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={frequencyCap} placeholder="5" />
				</div>
				<div></div>
				<div>
					<label for="ad-start" class="mb-1 block text-sm font-medium">{t('ads.start_date', 'Start Date')}</label>
					<input id="ad-start" type="datetime-local" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={startDate} />
				</div>
				<div>
					<label for="ad-end" class="mb-1 block text-sm font-medium">{t('ads.end_date', 'End Date')}</label>
					<input id="ad-end" type="datetime-local" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={endDate} />
				</div>
			</div>
		</div>

		<!-- Geo Targeting -->
		<div class="rounded-lg border p-6 space-y-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('ads.geo_targeting', 'Geo Targeting (Optional)')}</h2>
			<p class="text-xs" style="color: var(--color-muted)">{t('ads.geo_helper', 'Restrict this campaign to specific countries or spaces. Leave empty to serve to all users.')}</p>

			<!-- Target Geo (country codes) -->
			<div>
				<label for="ad-geo-input" class="mb-1 block text-sm font-medium">{t('ads.target_countries', 'Target Countries (ISO codes)')}</label>
				<div class="flex gap-2">
					<input id="ad-geo-input" type="text" class="flex-1 rounded-md border px-3 py-2 text-sm font-mono uppercase" style="border-color: var(--color-border); background-color: var(--color-background)" placeholder="NG" bind:value={geoInput} onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addGeoCode(); } }} />
					<Button variant="outline" size="sm" onclick={addGeoCode}>{t('common.add', 'Add')}</Button>
				</div>
				{#if targetGeo.length > 0}
					<div class="mt-2 flex flex-wrap gap-2">
						{#each targetGeo as code}
							<span class="inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-medium" style="background-color: var(--color-primary); color: white">
								{code}
								<button onclick={() => removeGeoCode(code)} class="hover:opacity-70" aria-label={t('common.remove', 'Remove')}>×</button>
							</span>
						{/each}
					</div>
				{/if}
				{#if countries.length > 0}
					<div class="mt-2 flex flex-wrap gap-1">
						{#each countries.slice(0, 20) as c}
							<button class="rounded px-2 py-0.5 text-xs hover:opacity-70" style="background-color: var(--color-muted); color: var(--color-background)" onclick={() => { const code = (c.geo_code ?? c.slug).toUpperCase(); if (!targetGeo.includes(code)) targetGeo = [...targetGeo, code]; }}>
								{c.name} ({c.geo_code ?? c.slug})
							</button>
						{/each}
					</div>
				{/if}
			</div>

			<!-- Target Space IDs -->
			<div>
				<label for="ad-space-search" class="mb-1 block text-sm font-medium">{t('ads.target_spaces', 'Target Specific Spaces')}</label>
				<input id="ad-space-search" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" placeholder={t('ads.search_spaces', 'Search spaces...')} bind:value={spaceSearch} />
				{#if targetSpaceIds.length > 0}
					<div class="mt-2 flex flex-wrap gap-2">
						{#each targetSpaceIds as id}
							{@const space = geoSpaces.find((s) => s.id === id)}
							<span class="inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-medium" style="background-color: var(--color-info); color: white">
								{space?.name ?? id.slice(0, 8)}
								<button onclick={() => toggleSpace(id)} class="hover:opacity-70" aria-label={t('common.remove', 'Remove')}>×</button>
							</span>
						{/each}
					</div>
				{/if}
				{#if spaceSearch && filteredSpaces.length > 0}
					<div class="mt-2 max-h-40 overflow-y-auto rounded-md border" style="border-color: var(--color-border)">
						{#each filteredSpaces.slice(0, 20) as s}
							<button class="flex w-full items-center justify-between px-3 py-2 text-left text-sm hover:opacity-70" style="border-bottom: 1px solid var(--color-border)" onclick={() => toggleSpace(s.id)}>
								<span>{s.name}</span>
								<span class="text-xs font-mono" style="color: var(--color-muted)">{s.slug}</span>
							</button>
						{/each}
					</div>
				{/if}
			</div>
		</div>

		<!-- Actions -->
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => goto('/ads')}>{t('common.cancel', 'Cancel')}</Button>
			<Button onclick={save} disabled={saving || !name || !slotTypeName}>
				<Save class="h-3.5 w-3.5" />
				{saving ? t('common.saving', 'Saving...') : t('ads.create_campaign', 'Create Campaign')}
			</Button>
		</div>
	</div>
{/if}
