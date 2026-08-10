<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Button } from "$lib/components/ui/button";
	import { adSlotTypesApi, adsAdminApi, geoApi } from "$lib/api";
	import SpaceAutocomplete from "$lib/components/ui/SpaceAutocomplete.svelte";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { goto } from "$app/navigation";
	import { ArrowLeft, Save, Image as ImageIcon, X, Code, FileText, Type, Eye } from "lucide-svelte";
	import MediaPickerModal from "$lib/components/ui/MediaPickerModal.svelte";

	// Ref: japaconnect-engineering/sections/25-ad-placement.md — the backend
	// AdSlotType sqlc struct exposes `slot_name` and `max_display_per_page`
	// (NOT `name`/`width`/`height`). The previous type caused the dropdown to
	// render blank `(x)` options.
	type SlotType = {
		id: string;
		slot_name: string;
		description?: string | null;
		max_display_per_page?: number;
		is_active?: boolean;
	};
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
	let contentHtml = $state('');
	let contentMarkdown = $state('');
	let contentMode = $state<'text' | 'html' | 'markdown' | 'image' | 'preview'>('text');
	let imageMediaId = $state('');
	let imageUrl = $state('');
	let showMediaPicker = $state(false);

	// Geo targeting
	let targetGeo = $state<string[]>([]);
	let targetSpaceIds = $state<string[]>([]);
	let geoInput = $state('');
	let countries = $state<GeoSpace[]>([]);

	let slotTypes = $state<SlotType[]>([]);
	let saving = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');
	let formDataLoading = $state(true);

	$effect(() => {
		(async () => {
			try {
				const [slotsResult, countriesResult] = await Promise.all([
					adSlotTypesApi.list('page_size=100'),
					geoApi.listCountries()
				]);
				if (slotsResult.ok) {
					// §6.2: guard null data; backend returns {data: [...]} envelope
					// which apiFetch already unwraps, so result.data is the array.
					const data = (slotsResult.data ?? []) as { data?: SlotType[] } | SlotType[];
					slotTypes = Array.isArray(data) ? data : (data.data ?? []);
				} else {
					errorMessage = slotsResult.error || t('ads.load_error', 'Failed to load form data');
				}
				if (countriesResult.ok) {
					countries = (countriesResult.data as GeoSpace[]) ?? [];
				}
			} catch {
				errorMessage = t('ads.load_error', 'Failed to load form data');
			} finally {
				formDataLoading = false;
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

	function handleMediaSelect(media: { id: string; url: string; filename: string }) {
		imageMediaId = media.id;
		imageUrl = media.url;
	}

	function removeImage() {
		imageMediaId = '';
		imageUrl = '';
	}

	// Minimal markdown-to-HTML for the preview tab. Handles headings, bold,
	// italic, code, links, lists, and paragraphs — enough for a realistic
	// ad preview without pulling in a full markdown library.
	function renderMarkdown(md: string): string {
		if (!md.trim()) return '';
		const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
		const lines = md.split('\n');
		let html = '';
		let inList = false;
		for (const raw of lines) {
			const line = raw.trim();
			if (!line) {
				if (inList) { html += '</ul>'; inList = false; }
				continue;
			}
			let processed = esc(line)
				.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
				.replace(/\*(.+?)\*/g, '<em>$1</em>')
				.replace(/`(.+?)`/g, '<code>$1</code>')
				.replace(/\[(.+?)\]\((.+?)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
			if (/^#{1,3}\s/.test(line)) {
				if (inList) { html += '</ul>'; inList = false; }
				const level = line.match(/^#+/)?.[0].length ?? 1;
				processed = processed.replace(/^#+\s/, '');
				html += `<h${level}>${processed}</h${level}>`;
			} else if (/^[-*]\s/.test(line)) {
				if (!inList) { html += '<ul>'; inList = true; }
				processed = processed.replace(/^[-*]\s/, '');
				html += `<li>${processed}</li>`;
			} else {
				if (inList) { html += '</ul>'; inList = false; }
				html += `<p>${processed}</p>`;
			}
		}
		if (inList) html += '</ul>';
		return html;
	}

	// Track the last editing mode so the preview knows which content
	// type to render. Updated whenever a non-preview tab is clicked.
	let lastEditMode = $state<'text' | 'html' | 'markdown' | 'image'>('text');

	function switchMode(mode: 'text' | 'html' | 'markdown' | 'image' | 'preview') {
		if (mode !== 'preview') lastEditMode = mode;
		contentMode = mode;
	}

	// Derived: the rendered HTML body for the preview, based on the last
	// editing mode the user was in before clicking Preview.
	let previewBodyHtml = $derived.by(() => {
		if (lastEditMode === 'html') return contentHtml;
		if (lastEditMode === 'markdown') return renderMarkdown(contentMarkdown);
		if (lastEditMode === 'image') return imageUrl ? `<img src="${imageUrl}" alt="" style="max-width:100%;border-radius:0.5rem" />` : '';
		return contentBody ? `<p>${contentBody.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')}</p>` : '';
	});


	async function save() {
		if (!name || !slotTypeName || !contentHeadline.trim()) {
			errorMessage = t('ads.name_required', 'Name, slot type, and headline are required');
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
					cta: contentCta,
					...(contentHtml ? { html: contentHtml } : {}),
					...(contentMarkdown ? { markdown: contentMarkdown } : {}),
					...(imageMediaId ? { image_media_id: imageMediaId } : {}),
					...(imageUrl ? { image_url: imageUrl } : {})
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
{:else if formDataLoading}
	<div class="rounded-lg border p-6 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<p class="text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
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
				<input id="ad-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={name} placeholder={t('app.ads.new.ph_summer_travel_promo', 'Summer Travel Promo')} />
			</div>

			<div>
				<label for="ad-slot" class="mb-1 block text-sm font-medium">{t('ads.slot_type', 'Slot Type')} *</label>
				<select id="ad-slot" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={slotTypeName}>
					<option value="">{t('ads.select_slot', 'Select a slot type...')}</option>
					{#each slotTypes as slot (slot)}
						<option value={slot.slot_name}>{slot.slot_name}{#if slot.max_display_per_page} ({slot.max_display_per_page}/page){/if}</option>
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
				<label for="ad-headline" class="mb-1 block text-sm font-medium">{t('ads.headline', 'Headline')} *</label>
				<input id="ad-headline" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={contentHeadline} placeholder={t('app.ads.new.ph_discover_nigeria', 'Discover Nigeria')} />
			</div>

			<!-- Body with mode tabs: Text / HTML / Image -->
			<div>
				<div class="mb-1 flex items-center justify-between">
					<label for="ad-body" class="block text-sm font-medium">{t('ads.body', 'Body')}</label>
					<div class="flex gap-1">
						<button
							type="button"
							class="rounded px-2 py-0.5 text-xs font-medium transition-colors"
							style={contentMode === 'text' ? 'background-color: var(--color-primary); color: white' : 'color: var(--color-muted)'}
							onclick={() => switchMode('text')}
						>
							<Type class="mr-1 inline h-3 w-3" />{t('ads.mode_text', 'Text')}
						</button>
						<button
							type="button"
							class="rounded px-2 py-0.5 text-xs font-medium transition-colors"
							style={contentMode === 'html' ? 'background-color: var(--color-primary); color: white' : 'color: var(--color-muted)'}
							onclick={() => switchMode('html')}
						>
							<Code class="mr-1 inline h-3 w-3" />{t('ads.mode_html', 'HTML')}
						</button>
						<button
							type="button"
							class="rounded px-2 py-0.5 text-xs font-medium transition-colors"
							style={contentMode === 'markdown' ? 'background-color: var(--color-primary); color: white' : 'color: var(--color-muted)'}
							onclick={() => switchMode('markdown')}
						>
							<FileText class="mr-1 inline h-3 w-3" />{t('ads.mode_markdown', 'Markdown')}
						</button>
						<button
							type="button"
							class="rounded px-2 py-0.5 text-xs font-medium transition-colors"
							style={contentMode === 'image' ? 'background-color: var(--color-primary); color: white' : 'color: var(--color-muted)'}
							onclick={() => switchMode('image')}
						>
							<ImageIcon class="mr-1 inline h-3 w-3" />{t('ads.mode_image', 'Image')}
						</button>
						<button
							type="button"
							class="rounded px-2 py-0.5 text-xs font-medium transition-colors"
							style={contentMode === 'preview' ? 'background-color: var(--color-primary); color: white' : 'color: var(--color-muted)'}
							onclick={() => switchMode('preview')}
						>
							<Eye class="mr-1 inline h-3 w-3" />{t('ads.mode_preview', 'Preview')}
						</button>
					</div>
				</div>
				{#if contentMode === 'text'}
					<textarea id="ad-body" rows="3" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={contentBody} placeholder={t('app.ads.new.ph_explore_the_vibrant_culture_and', 'Explore the vibrant culture and landscapes...')}></textarea>
				{:else if contentMode === 'html'}
					<textarea id="ad-body-html" rows="5" class="w-full rounded-md border px-3 py-2 font-mono text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={contentHtml} placeholder="<p>Write <strong>HTML</strong> content here...</p>"></textarea>
					<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('ads.html_helper', 'Raw HTML — will be sanitized on render')}</p>
				{:else if contentMode === 'markdown'}
					<textarea id="ad-body-md" rows="5" class="w-full rounded-md border px-3 py-2 font-mono text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={contentMarkdown} placeholder={t('app.ads.new.ph_bold_and_italic_markdown_content', '**Bold** and *italic* markdown content...')}></textarea>
					<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('ads.markdown_helper', 'Markdown — rendered to HTML on display')}</p>
				{:else if contentMode === 'image'}
					<!-- Image mode: pick an image from the media library -->
					{#if imageUrl}
						<div class="relative inline-block">
							<img src={imageUrl} alt="" class="max-h-48 rounded-lg border object-cover" style="border-color: var(--color-border)" />
							<button
								type="button"
								class="absolute -right-2 -top-2 rounded-full p-1 shadow-lg"
								style="background-color: var(--color-danger); color: white"
								onclick={removeImage}
								aria-label={t('common.remove', 'Remove')}
							>
								<X class="h-4 w-4" />
							</button>
						</div>
					{:else}
						<button
							type="button"
							class="flex w-full items-center justify-center gap-2 rounded-lg border-2 border-dashed py-8 text-sm transition-colors hover:opacity-70"
							style="border-color: var(--color-border); color: var(--color-muted)"
							onclick={() => (showMediaPicker = true)}
						>
							<ImageIcon class="h-5 w-5" />
							{t('ads.content_image_placeholder', 'Click to select an image from the media library')}
						</button>
					{/if}
				{:else}
					<!-- Preview mode: render the ad as it will appear to users -->
					<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-background)">
						{#if !contentHeadline.trim() && !previewBodyHtml}
							<p class="py-8 text-center text-sm" style="color: var(--color-muted)">
								{t('ads.preview_empty', 'Nothing to preview yet — add a headline and body content.')}
							</p>
						{:else}
							<div class="space-y-3">
								{#if contentHeadline.trim()}
									<h3 class="text-lg font-bold" style="color: var(--color-foreground)">{contentHeadline}</h3>
								{/if}
								{#if previewBodyHtml}
									<!-- eslint-disable-next-line svelte/no-at-html-tags -- preview only, content is admin-authored -->
									<div class="text-sm leading-relaxed prose prose-sm max-w-none" style="color: var(--color-foreground)">
										{@html previewBodyHtml}
									</div>
								{/if}
								{#if contentCta.trim()}
									<span class="inline-block rounded-md px-4 py-2 text-sm font-medium" style="background-color: var(--color-primary); color: white">
										{contentCta}
									</span>
								{/if}
							</div>
						{/if}
					</div>
					<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('ads.preview_helper', 'Preview — switch back to a content tab to edit')}</p>
				{/if}
			</div>

			<div>
				<label for="ad-cta" class="mb-1 block text-sm font-medium">{t('ads.cta', 'Call to Action')}</label>
				<input id="ad-cta" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={contentCta} placeholder={t('app.ads.new.ph_learn_more', 'Learn More')} />
			</div>
		</div>
		<!-- Budget & Schedule -->
		<div class="rounded-lg border p-6 space-y-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('ads.budget_schedule', 'Budget & Schedule')}</h2>

			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="ad-budget" class="mb-1 block text-sm font-medium">{t('ads.budget', 'Budget (credits)')}</label>
					<input id="ad-budget" type="number" min="0" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={budget} placeholder="1000" />
					<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('ads.budget_helper', 'Minimum 10 credits if specified')}</p>
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
					<input id="ad-geo-input" type="text" class="flex-1 rounded-md border px-3 py-2 text-sm font-mono uppercase" style="border-color: var(--color-border); background-color: var(--color-background)" placeholder={t('app.ads.new.ph_ng', 'NG')} bind:value={geoInput} onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addGeoCode(); } }} />
					<Button variant="outline" size="sm" onclick={addGeoCode}>{t('common.add', 'Add')}</Button>
				</div>
				{#if targetGeo.length > 0}
					<div class="mt-2 flex flex-wrap gap-2">
						{#each targetGeo as code (code)}
							<span class="inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-medium" style="background-color: var(--color-primary); color: white">
								{code}
								<button onclick={() => removeGeoCode(code)} class="hover:opacity-70" aria-label={t('common.remove', 'Remove')}>×</button>
							</span>
						{/each}
					</div>
				{/if}
				{#if countries.length > 0}
					<div class="mt-2 flex flex-wrap gap-1">
						{#each countries.slice(0, 20) as c (c)}
								{#if c.geo_code}
							<button class="rounded px-2 py-0.5 text-xs hover:opacity-70" style="background-color: var(--color-muted); color: var(--color-background)" onclick={() => { const code = c.geo_code!.toUpperCase(); if (!targetGeo.includes(code)) targetGeo = [...targetGeo, code]; }}>
								{c.name} ({c.geo_code})
							</button>
								{/if}
						{/each}
					</div>
				{/if}
			</div>

			<!-- Target Space IDs — debounced autocomplete dropdown -->
			<div>
				<label for="space-autocomplete-input" class="mb-1 block text-sm font-medium">{t('ads.target_spaces', 'Target Specific Spaces')}</label>
				<SpaceAutocomplete bind:selectedIds={targetSpaceIds} placeholder={t('ads.search_spaces', 'Search spaces...')} />
			</div>
		</div>

		<!-- Actions -->
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => goto('/ads')}>{t('common.cancel', 'Cancel')}</Button>
			<Button onclick={save} disabled={saving || !name || !slotTypeName || !contentHeadline.trim()}>
				<Save class="h-3.5 w-3.5" />
				{saving ? t('common.saving', 'Saving...') : t('ads.create_campaign', 'Create Campaign')}
			</Button>
		</div>
	</div>
{/if}

<MediaPickerModal bind:open={showMediaPicker} onselect={handleMediaSelect} />
