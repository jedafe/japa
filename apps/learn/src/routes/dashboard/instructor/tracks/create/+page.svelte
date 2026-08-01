<script lang="ts">
	// Track creation — Japa University instructor creates a new track (ADR-063).
	// CSR ONLY: POST /learn/university/my/tracks.
	// Payload matches backend CreateTrackInput (internal/university/store.go):
	//   title (required), slug (required), description, difficulty,
	//   price_kobo, currency, category, language.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { Plus, Save, ArrowLeft } from 'lucide-svelte';

	// Difficulty options are a fixed enum from the backend, not configurable data (Rule 16).
	const DIFFICULTIES = ['beginner', 'intermediate', 'advanced'] as const;
	type Difficulty = (typeof DIFFICULTIES)[number];

	let title = $state('');
	let slug = $state('');
	let slugTouched = $state(false);
	let description = $state('');
	let difficulty = $state<Difficulty | ''>('');
	let priceNaira = $state(0);
	let currency = $state('NGN');
	let category = $state('');
	let language = $state('en');
	let saving = $state(false);
	let error = $state<string | null>(null);

	// Auto-generate slug from title unless the user has manually edited the slug.
	$effect(() => {
		if (!slugTouched) {
			slug = title
				.toLowerCase()
				.trim()
				.replace(/[^a-z0-9\s-]/g, '')
				.replace(/\s+/g, '-')
				.replace(/-+/g, '-')
				.replace(/^-|-$/g, '');
		}
	});

	const formValid = $derived(title.trim().length > 0 && slug.trim().length > 0);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (saving || !formValid) return;
		saving = true;
		error = null;
		const r = await api.post('/learn/university/my/tracks', {
			title: title.trim(),
			slug: slug.trim(),
			description: description || undefined,
			difficulty: difficulty || undefined,
			price_kobo: Math.round(priceNaira * 100),
			currency: currency || undefined,
			category: category || undefined,
			language: language || undefined
		});
		saving = false;
		if (r.error) {
			error = r.error.message;
		} else {
			goto('/dashboard/instructor');
		}
	}
</script>

<svelte:head>
	<title>{t('university.create_track')} · {t('app.name')}</title>
</svelte:head>

<div class="flex justify-center px-4 py-6 md:px-6 lg:px-8">
	<form onsubmit={submit} class="w-full max-w-2xl space-y-5">
		<header class="mb-2">
			<a
				href="/dashboard/instructor"
				class="mb-3 inline-flex items-center gap-1 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
			>
				<ArrowLeft size={16} />
				{t('common.back')}
			</a>
			<h1 class="flex items-center gap-2 text-2xl font-bold text-[var(--color-text)]">
				<Plus size={24} class="text-[var(--color-primary)]" />
				{t('university.create_track')}
			</h1>
			<p class="mt-1 text-sm text-[var(--color-muted)]">
				{t('university.create_track_description')}
			</p>
		</header>

		<!-- Title (required) -->
		<div>
			<label for="title" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.track_title_label')}
			</label>
			<input
				id="title"
				type="text"
				bind:value={title}
				required
				placeholder="e.g. Mastering Go for Backend Engineers"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
		</div>

		<!-- Slug (required, auto-generated) -->
		<div>
			<label for="slug" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.track_slug_label')}
			</label>
			<input
				id="slug"
				type="text"
				bind:value={slug}
				oninput={() => (slugTouched = true)}
				required
				placeholder="mastering-go-for-backend-engineers"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
			<p class="mt-1 text-xs text-[var(--color-text-subtle)]">
				{t('university.track_slug_hint')}
			</p>
		</div>

		<!-- Description (optional) -->
		<div>
			<label for="description" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.track_description_label')}
			</label>
			<textarea
				id="description"
				bind:value={description}
				rows="4"
				placeholder="A short summary of what learners will gain from this track…"
				class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			></textarea>
		</div>

		<!-- Difficulty (optional) -->
		<div>
			<label for="difficulty" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.track_difficulty_label')}
			</label>
			<select
				id="difficulty"
				bind:value={difficulty}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
			>
				<option value="">—</option>
				{#each DIFFICULTIES as d (d)}
					<option value={d}>{t(`university.difficulty_${d}`)}</option>
				{/each}
			</select>
		</div>

		<!-- Price (Naira → kobo) -->
		<div>
			<label for="price" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.track_price_label')}
			</label>
			<input
				id="price"
				type="number"
				min="0"
				step="0.01"
				bind:value={priceNaira}
				placeholder="0"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
			<p class="mt-1 text-xs text-[var(--color-text-subtle)]">
				{priceNaira <= 0 ? t('university.track_price_free_hint') : t('university.track_price_hint', '₦{n} = {k} kobo').replace('{n}', String(priceNaira)).replace('{k}', String(Math.round(priceNaira * 100)))}
			</p>
		</div>

		<!-- Currency -->
		<div>
			<label for="currency" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.track_currency_label')}
			</label>
			<input
				id="currency"
				type="text"
				bind:value={currency}
				placeholder="NGN"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
		</div>

		<!-- Category (optional) -->
		<div>
			<label for="category" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.track_category_label')}
			</label>
			<input
				id="category"
				type="text"
				bind:value={category}
				placeholder="e.g. Backend Engineering"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
		</div>

		<!-- Language -->
		<div>
			<label for="language" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.track_language_label')}
			</label>
			<input
				id="language"
				type="text"
				bind:value={language}
				placeholder="en"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
		</div>

		{#if error}
			<p class="text-sm text-[var(--color-destructive)]">{error}</p>
		{/if}

		<button
			type="submit"
			disabled={saving || !formValid}
			class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-60"
		>
			<Save size={16} />
			{saving ? t('common.loading') : t('university.create_track')}
		</button>
	</form>
</div>
