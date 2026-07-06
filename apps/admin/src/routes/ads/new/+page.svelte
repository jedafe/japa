<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	let name = $state('');
	let creativeTitle = $state('');
	let creativeBody = $state('');
	let creativeImageUrl = $state('');
	let creativeDestinationUrl = $state('');
	let budget = $state(100);
	let bidStrategy = $state('cpc');
	let targetingGeo = $state('');
	let targetingInterests = $state('');
	let startDate = $state('');
	let endDate = $state('');
	let creating = $state(false);
	let errorMessage = $state('');

	const bidStrategies = [
		{ value: 'cpc', label: t('ads.bid.cpc') },
		{ value: 'cpm', label: t('ads.bid.cpm') },
		{ value: 'cpa', label: t('ads.bid.cpa') }
	];

	async function createCampaign() {
		if (!name.trim()) return;
		creating = true;
		errorMessage = '';
		try {
			const targeting: Record<string, unknown> = {};
			if (targetingGeo.trim()) targeting.geo = targetingGeo.split(',').map((s) => s.trim());
			if (targetingInterests.trim()) targeting.interests = targetingInterests.split(',').map((s) => s.trim());

			const result = await api('/admin/advertising/campaigns', {
				method: 'POST',
				body: {
					name: name.trim(),
					creative: {
						title: creativeTitle.trim() || null,
						body: creativeBody.trim() || null,
						image_url: creativeImageUrl.trim() || null,
						destination_url: creativeDestinationUrl.trim() || null
					},
					budget,
					bid_strategy: bidStrategy,
					targeting,
					start_date: startDate || null,
					end_date: endDate || null
				}
			});
			if (result.ok) {
				const campaign = result.data as { id: string };
				await goto(`/ads/${campaign.id}`);
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('ads.create_error');
		} finally {
			creating = false;
		}
	}
</script>

<svelte:head>
	<title>Create Campaign — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('ads.create_title')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('ads.create_description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mx-auto max-w-2xl space-y-6">
	<!-- Campaign Name -->
	<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<h2 class="mb-4 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('ads.sections.campaign')}</h2>
		<div>
			<label for="campaign-name" class="mb-1 block text-sm font-medium">{t('ads.fields.name')} <span style="color: var(--color-danger)">*</span></label>
			<input
				id="campaign-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				placeholder="e.g. Summer Relocation Guide Promo"
				bind:value={name}
			/>
		</div>
	</div>

	<!-- Creative -->
	<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<h2 class="mb-4 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('ads.sections.creative')}</h2>
		<div class="space-y-4">
			<div>
				<label for="creative-title" class="mb-1 block text-sm font-medium">{t('ads.fields.headline')}</label>
				<input
					id="creative-title"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder="Ad headline"
					bind:value={creativeTitle}
				/>
			</div>
			<div>
				<label for="creative-body" class="mb-1 block text-sm font-medium">{t('ads.fields.body')}</label>
				<textarea
					id="creative-body"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					rows="3"
					placeholder="Ad body text"
					bind:value={creativeBody}
				></textarea>
			</div>
			<div>
				<label for="creative-image" class="mb-1 block text-sm font-medium">{t('ads.fields.image_url')}</label>
				<input
					id="creative-image"
					type="url"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder="https://example.com/ad-image.jpg"
					bind:value={creativeImageUrl}
				/>
			</div>
			<div>
				<label for="creative-url" class="mb-1 block text-sm font-medium">{t('ads.fields.dest_url')}</label>
				<input
					id="creative-url"
					type="url"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder="https://example.com/landing"
					bind:value={creativeDestinationUrl}
				/>
			</div>
		</div>
	</div>

	<!-- Budget & Bidding -->
	<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<h2 class="mb-4 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('ads.sections.budget')}</h2>
		<div class="grid gap-4 sm:grid-cols-2">
			<div>
				<label for="budget" class="mb-1 block text-sm font-medium">{t('ads.fields.budget')}</label>
				<input
					id="budget"
					type="number"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					min="1"
					bind:value={budget}
				/>
			</div>
			<div>
				<label for="bid-strategy" class="mb-1 block text-sm font-medium">{t('ads.fields.bid_strategy')}</label>
				<select
					id="bid-strategy"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={bidStrategy}
				>
					{#each bidStrategies as strategy}
						<option value={strategy.value}>{strategy.label}</option>
					{/each}
				</select>
			</div>
		</div>
	</div>

	<!-- Targeting -->
	<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<h2 class="mb-4 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('ads.sections.targeting')}</h2>
		<div class="space-y-4">
			<div>
				<label for="targeting-geo" class="mb-1 block text-sm font-medium">{t('ads.fields.countries')}</label>
				<input
					id="targeting-geo"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder="NG, US, GB (comma-separated)"
					bind:value={targetingGeo}
				/>
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('ads.countries_helper')}</p>
			</div>
			<div>
				<label for="targeting-interests" class="mb-1 block text-sm font-medium">{t('ads.fields.interests')}</label>
				<input
					id="targeting-interests"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder="immigration, study-abroad (comma-separated)"
					bind:value={targetingInterests}
				/>
			</div>
		</div>
	</div>

	<!-- Schedule -->
	<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<h2 class="mb-4 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('ads.sections.schedule')}</h2>
		<div class="grid gap-4 sm:grid-cols-2">
			<div>
				<label for="start-date" class="mb-1 block text-sm font-medium">{t('ads.fields.start_date')}</label>
				<input
					id="start-date"
					type="date"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={startDate}
				/>
			</div>
			<div>
				<label for="end-date" class="mb-1 block text-sm font-medium">{t('ads.fields.end_date')}</label>
				<input
					id="end-date"
					type="date"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={endDate}
				/>
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('ads.end_helper')}</p>
			</div>
		</div>
	</div>

	<div class="flex items-center gap-3 pt-2">
		<button
			class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
			style="background-color: var(--color-primary)"
			disabled={creating || !name.trim()}
			onclick={createCampaign}
		>
			{creating ? t('ads.creating') : t('ads.create_campaign')}
		</button>
		<a href="/ads" class="rounded-md px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10" style="color: var(--color-muted)">
			{t('common.cancel')}
		</a>
	</div>
</div>
