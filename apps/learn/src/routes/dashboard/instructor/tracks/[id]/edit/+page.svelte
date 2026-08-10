<script lang="ts">
	// Track editor — Japa University instructor edits an existing track (ADR-063).
	// CSR ONLY: GET /learn/university/my/tracks (find by id), PATCH /learn/university/my/tracks/{id}.
	// PATCH accepts optional fields: title, description, difficulty, price_kobo,
	// currency, category, language (internal/university/handlers.go).
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import { Save, ArrowLeft } from 'lucide-svelte';
	import { DIFFICULTY_LEVELS } from '$lib/constants/learning';
	import type { InstructorTrack, TrackLevel } from '$lib/types';

	const trackId = page.params.id;

	type Category = { id: string; slug: string; name: string; sort_order: number };

	let track = $state<InstructorTrack | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let error = $state<string | null>(null);
	let success = $state<string | null>(null);
	let categories = $state<Category[]>([]);

	// Form fields
	let title = $state('');
	let description = $state('');
	let difficulty = $state<TrackLevel>('beginner');
	let priceNaira = $state(0);
	let currency = $state('NGN');
	let category = $state('');
	let language = $state('en');
	let subscriptionEligible = $state(false);

	// Fetch active categories from the public API (Rule 16 — no hardcoded option lists).
	onMount(async () => {
		const r = await api.get<Category[]>('/learn/university/categories');
		if (!r.error && r.data) {
			categories = r.data;
		}
	});

	$effect(() => {
		(async () => {
			const r = await api.get<InstructorTrack[]>('/learn/university/my/tracks');
			loading = false;
			if (r.data) {
				const found = r.data.find((tr) => tr.id === trackId) ?? null;
				track = found;
				if (found) {
					title = found.title ?? '';
					description = (found as InstructorTrack & { description?: string }).description ?? '';
					difficulty = found.level ?? 'beginner';
					priceNaira = (found.price_kobo ?? 0) / 100;
					currency = (found as InstructorTrack & { currency?: string }).currency ?? 'NGN';
					category = (found as InstructorTrack & { category?: string }).category ?? '';
					language = (found as InstructorTrack & { language?: string }).language ?? 'en';
					subscriptionEligible = (found as InstructorTrack & { subscription_eligible?: boolean }).subscription_eligible ?? false;
				}
			}
		})();
	});

	async function save(e: SubmitEvent) {
		e.preventDefault();
		if (saving) return;
		saving = true;
		error = null;
		success = null;

		const body: Record<string, unknown> = {
			title: title || undefined,
			description: description || undefined,
			difficulty: difficulty,
			price_kobo: Math.round(priceNaira * 100),
			currency: currency || undefined,
			category: category || undefined,
			language: language || undefined,
			subscription_eligible: subscriptionEligible
		};

		const r = await api.patch<InstructorTrack>(`/learn/university/my/tracks/${trackId}`, body);
		saving = false;

		if (r.error) {
			error = r.error.message;
		} else {
			success = t('university.track_saved', 'Track saved successfully.');
			// Redirect back to the instructor dashboard after a brief success flash.
			setTimeout(() => goto('/dashboard/instructor'), 800);
		}
	}
</script>

<svelte:head>
	<title>{t('university.edit_track', 'Edit Track')} · {t('app.name')}</title>
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<header class="mb-6">
		<a
			href="/dashboard/instructor"
			class="mb-3 inline-flex items-center gap-1 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={16} />
			{t('common.back', 'Back')}
		</a>
		<h1 class="text-2xl font-bold text-[var(--color-text)]">
			{t('university.edit_track', 'Edit Track')}
		</h1>
	</header>

	{#if loading}
		<div class="max-w-2xl space-y-4">
			{#each Array(6) as _}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<LoadingSkeleton class="mb-2 h-4 w-24" />
					<LoadingSkeleton class="h-10 w-full" />
				</div>
			{/each}
		</div>
	{:else if !track}
		<EmptyState
			title={t('university.track_not_found', 'Track not found')}
			description={t('university.track_not_found_description', 'This track could not be found or you do not have access.')}
		/>
	{:else}
		<form onsubmit={save} class="max-w-2xl space-y-5">
			<!-- Title -->
			<div>
				<label for="title" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.title_label', 'Title')}
				</label>
				<input
					id="title"
					type="text"
					bind:value={title}
					required
					placeholder={t('university.title_placeholder', 'Course title')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>

			<!-- Description -->
			<div>
				<label for="description" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.description_label', 'Description')}
				</label>
				<textarea
					id="description"
					bind:value={description}
					rows="4"
					placeholder={t('university.description_placeholder', 'Short summary of the course')}
					class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				></textarea>
			</div>

			<!-- Difficulty -->
			<div>
				<label for="difficulty" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.difficulty_label', 'Difficulty')}
				</label>
				<select
					id="difficulty"
					bind:value={difficulty}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
				>
					{#each DIFFICULTY_LEVELS as level}
						<option value={level}>{t(`level.${level}`)}</option>
					{/each}
				</select>
			</div>

			<!-- Price (Naira → kobo) -->
			<div>
				<label for="price" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.price_label', 'Price (₦)')}
				</label>
				<input
					id="price"
					type="number"
					min="0"
					step="0.01"
					bind:value={priceNaira}
					placeholder="0.00"
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
				<p class="mt-1 text-xs text-[var(--color-text-subtle)]">
					{t('university.price_hint', 'Enter the price in Naira. 0 = free.')}
				</p>
			</div>

			<!-- Currency -->
			<div>
				<label for="currency" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.currency_label', 'Currency')}
				</label>
				<input
					id="currency"
					type="text"
					bind:value={currency}
					placeholder="NGN"
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>

			<!-- Category (from API) -->
			<div>
				<label for="category" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.category_label', 'Category')}
				</label>
				<select
					id="category"
					bind:value={category}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
				>
					<option value="">—</option>
					{#each categories as c (c.id)}
						<option value={c.slug}>{c.name}</option>
					{/each}
				</select>
			</div>

			<!-- Language -->
			<div>
				<label for="language" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('university.language_label', 'Language')}
				</label>
				<input
					id="language"
					type="text"
					bind:value={language}
					placeholder="en"
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
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
			{#if success}
				<p class="text-sm text-[var(--color-progress)]">{success}</p>
			{/if}

			<button
				type="submit"
				disabled={saving}
				class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-60"
			>
				<Save size={16} />
				{saving ? t('common.loading') : t('common.save', 'Save')}
			</button>
		</form>
	{/if}
</div>
