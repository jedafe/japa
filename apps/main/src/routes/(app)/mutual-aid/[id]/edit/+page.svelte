<script lang="ts">
	// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
	import { t } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import { ArrowLeft, Send } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import type { MutualAidPost, MutualAidPostType, MutualAidCategory, Space } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const post = $derived(data.post);

	const categoryOptions: { value: MutualAidCategory; label: string }[] = [
		{ value: 'housing', label: t('mutual_aid.cat_housing', 'Housing') },
		{ value: 'transport', label: t('mutual_aid.cat_transport', 'Transport') },
		{ value: 'documents', label: t('mutual_aid.cat_documents', 'Documents') },
		{ value: 'job_search', label: t('mutual_aid.cat_job_search', 'Job search') },
		{ value: 'language', label: t('mutual_aid.cat_language', 'Language') },
		{ value: 'social', label: t('mutual_aid.cat_social', 'Social') },
		{ value: 'other', label: t('mutual_aid.cat_other', 'Other') }
	];

	let postType = $state<MutualAidPostType>(post.post_type as MutualAidPostType);
	let category = $state<MutualAidCategory>((post.category as MutualAidCategory) ?? 'housing');
	let title = $state(post.title ?? '');
	let description = $state(post.description ?? '');
	let destinationSpaceId = $state(post.destination_space_id ?? '');
	let citySpaceId = $state(post.city_space_id ?? '');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	// City list — loaded when a destination country is selected (BUG-011).
	let cities = $state<Space[]>([]);
	let loadingCities = $state(false);
	let initialDestLoaded = $state(false);

	// When the destination changes, fetch the city list for that country.
	$effect(() => {
		const destId = destinationSpaceId;
		if (!destId) {
			cities = [];
			if (initialDestLoaded) citySpaceId = '';
			return;
		}
		if (initialDestLoaded) citySpaceId = '';
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
				initialDestLoaded = true;
			});
	});

	const canSubmit = $derived(
		title.trim().length > 0 && description.trim().length > 0 && destinationSpaceId.length > 0 && !submitting
	);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!canSubmit) return;
		submitting = true;
		error = null;
		const res = await api.patch<MutualAidPost>(`/mutual-aid/${post.id}`, {
			post_type: postType,
			category,
			title: title.trim(),
			description: description.trim(),
			destination_space_id: destinationSpaceId,
			city_space_id: citySpaceId || undefined
		});
		submitting = false;
		if (res.error) {
			error = res.error.message || t('mutual_aid.create_error', 'Failed to update post.');
			return;
		}
		goto(`/mutual-aid/${post.id}`);
	}
</script>

<svelte:head>
	<title>{t('mutual_aid.new_heading', 'Edit Mutual Aid Post')} — JapaConnect</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/mutual-aid/{post.id}"
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('mutual_aid.back', 'Back to mutual aid')}
		>
			<ArrowLeft size={18} />
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{t('mutual_aid.new_heading', 'Edit Mutual Aid Post')}
		</h1>
	</div>

	<form onsubmit={submit} class="mx-auto max-w-2xl space-y-5 p-4" novalidate>
		{#if error}
			<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-600" role="alert">
				{error}
			</p>
		{/if}

		<!-- Post type (read-only — changing type is not allowed) -->
		<div class="flex flex-col gap-1.5">
			<label for="post_type" class="text-sm font-medium text-[var(--color-text)]">
				{t('mutual_aid.field_post_type', 'Post type')}
			</label>
			<input
				id="post_type"
				type="text"
				value={postType}
				disabled
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-text-muted)]"
			/>
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
		<div class="flex flex-col gap-1.5">
			<label for="description" class="text-sm font-medium text-[var(--color-text)]">
				{t('mutual_aid.field_description', 'Description')}
				<span class="text-red-500" aria-hidden="true">*</span>
			</label>
			<textarea
				id="description"
				bind:value={description}
				rows="6"
				placeholder={t('mutual_aid.field_description_placeholder', 'Provide details about your request or offer…')}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			></textarea>
		</div>

		<!-- Destination country -->
		<div class="flex flex-col gap-1.5">
			<label for="destination_country" class="text-sm font-medium text-[var(--color-text)]">
				{t('mutual_aid.field_destination', 'Destination country')}
				<span class="text-red-500" aria-hidden="true">*</span>
			</label>
			<select
				id="destination_country"
				bind:value={destinationSpaceId}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
			>
				<option value="">{t('mutual_aid.field_destination_placeholder', 'Select a country')}</option>
				{#each data.countries as country (country.id)}
					<option value={country.id}>{country.name}</option>
				{/each}
			</select>
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

		<!-- Submit -->
		<div class="flex items-center justify-end gap-2 border-t border-[var(--color-border)] pt-4">
			<a
				href="/mutual-aid/{post.id}"
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{t('common.cancel', 'Cancel')}
			</a>
			<button
				type="submit"
				disabled={!canSubmit}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				<Send size={16} /> {submitting ? '…' : t('common.save', 'Save')}
			</button>
		</div>
	</form>
</div>
