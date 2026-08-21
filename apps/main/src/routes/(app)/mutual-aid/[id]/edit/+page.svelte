<script lang="ts">
	// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
	import { t } from '$lib/i18n';
	import { goto } from '$app/navigation';
	import { ArrowLeft, Send } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { untrack } from 'svelte';
	import { mutualAidCategoryOptions } from '$lib/enumOptions';
	import type { MutualAidPost, MutualAidPostType, MutualAidCategory } from '$lib/types';
	import type { PageData } from './$types';
	import AiBodyField from '$lib/ai/AiBodyField.svelte';

	let { data }: { data: PageData } = $props();

	const post = $derived(data.post);

	const categoryOptions = $derived(mutualAidCategoryOptions(t, false, data.categories));

	let postType = $state<MutualAidPostType>(untrack(() => post.post_type as MutualAidPostType));
	let category = $state<MutualAidCategory>(untrack(() => (post.category as MutualAidCategory) ?? (data.categories[0] ?? 'housing')));
	let title = $state(untrack(() => post.title ?? ''));
	let description = $state(untrack(() => post.description ?? ''));
	// BUG-005: destination/city are read-only on edit — the backend
	// UpdatePostRequest only accepts category/title/description. Kept as
	// state only to seed the disabled select's selected option.
	let destinationSpaceId = $state(untrack(() => post.destination_space_id ?? ''));
	let submitting = $state(false);
	let error = $state<string | null>(null);

	const canSubmit = $derived(
		title.trim().length > 0 && description.trim().length > 0 && !submitting
	);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!canSubmit) return;
		submitting = true;
		error = null;
		const res = await api.patch<MutualAidPost>(`/mutual-aid/${post.id}`, {
			category,
			title: title.trim(),
			description: description.trim()
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
		<AiBodyField
			id="description"
			label={t('mutual_aid.field_description', 'Description')}
			contentType="mutual_aid"
			{title}
			body={description}
			onBody={(v) => (description = v)}
			onTitle={(v) => (title = v)}
			user={data.user}
			placeholder={t('mutual_aid.field_description_placeholder', 'Provide details about your request or offer…')}
			rows={6}
		/>

		<!-- Destination country (read-only — destination cannot be changed
		     after creation; the backend UpdatePostRequest only accepts
		     category/title/description. BUG-005.) -->
		<div class="flex flex-col gap-1.5">
			<label for="destination_country" class="text-sm font-medium text-[var(--color-text)]">
				{t('mutual_aid.field_destination', 'Destination country')}
			</label>
			<select
				id="destination_country"
				value={destinationSpaceId}
				disabled
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-text-muted)]"
			>
				<option value="">{t('mutual_aid.field_destination_placeholder', 'Select a country')}</option>
				{#each data.countries as country (country.id)}
					<option value={country.id}>{country.name}</option>
				{/each}
			</select>
		</div>

		<!-- City (read-only — same reason as destination. BUG-005.) -->
		<div class="flex flex-col gap-1.5">
			<label for="city" class="text-sm font-medium text-[var(--color-text)]">
				{t('mutual_aid.field_city', 'City')}
			</label>
			<input
				id="city"
				type="text"
				value={post.city_space_name ?? ''}
				disabled
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-text-muted)]"
			/>
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
