<script lang="ts">
	// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { ArrowLeft, Send } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import type { MutualAidPost, MutualAidPostType, MutualAidCategory } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// Auth guard — the root layout exposes isAuthed (never overridden by page
	// loads). Redirect unauthenticated users to login with a return path.
	const authed = $derived(data.isAuthed);

	const postTypeOptions: { value: MutualAidPostType; label: string }[] = [
		{ value: 'need', label: t('mutual_aid.type_need', 'Need') },
		{ value: 'offer', label: t('mutual_aid.type_offer', 'Offer') },
		{ value: 'skill_swap', label: t('mutual_aid.type_skill_swap', 'Skill swap') }
	];

	const categoryOptions: { value: MutualAidCategory; label: string }[] = [
		{ value: 'housing', label: t('mutual_aid.cat_housing', 'Housing') },
		{ value: 'transport', label: t('mutual_aid.cat_transport', 'Transport') },
		{ value: 'documents', label: t('mutual_aid.cat_documents', 'Documents') },
		{ value: 'job_search', label: t('mutual_aid.cat_job_search', 'Job search') },
		{ value: 'language', label: t('mutual_aid.cat_language', 'Language') },
		{ value: 'social', label: t('mutual_aid.cat_social', 'Social') },
		{ value: 'other', label: t('mutual_aid.cat_other', 'Other') }
	];

	// Pre-select post_type from the ?type= query param (e.g. /mutual-aid/new?type=offer).
	const initialType = (() => {
		const q = page.url.searchParams.get('type');
		if (q === 'need' || q === 'offer' || q === 'skill_swap') return q;
		return 'need';
	})();

	let postType = $state<MutualAidPostType>(initialType);
	let category = $state<MutualAidCategory>('housing');
	let title = $state('');
	let description = $state('');
	let destinationSpaceId = $state('');
	let city = $state('');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	const canSubmit = $derived(
		title.trim().length > 0 && description.trim().length > 0 && destinationSpaceId.length > 0 && !submitting
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
			city_space_id: undefined
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

			<!-- City -->
			<div class="flex flex-col gap-1.5">
				<label for="city" class="text-sm font-medium text-[var(--color-text)]">
					{t('mutual_aid.field_city', 'City')}
				</label>
				<input
					id="city"
					type="text"
					bind:value={city}
					placeholder={t('mutual_aid.field_city_placeholder', 'e.g. Toronto')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>

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
