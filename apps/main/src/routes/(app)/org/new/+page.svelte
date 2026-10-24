<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { enhance } from '$app/forms';
	import { Building2, Plus, Loader2 } from 'lucide-svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { ActionData, PageData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	let name = $state('');
	let slug = $state('');
	let tagline = $state('');
	let description = $state('');
	let websiteUrl = $state('');
	let countryCode = $state('');
	// Default to the first API-provided org type (Rule 16 — no hardcoded
	// option lists). When /organizations/meta is unavailable the select and
	// submit are disabled rather than guessing values.
	let orgType = $state(untrack(() => data.orgMeta?.org_types?.[0]?.value ?? ''));
	const metaUnavailable = $derived(!(data.orgMeta?.org_types?.length));
	let submitting = $state(false);
	let slugTouched = $state(false);

	// Country autocomplete — filters the SSR-loaded country spaces
	// (data.countries from /geo/countries) client-side. The selected
	// geo_code is submitted as the country_code form field (Rule 16).
	function countryFetcher(q: string): Promise<AutocompleteItem[]> {
		const needle = q.toLowerCase().trim();
		const list = data.countries ?? [];
		const filtered = needle
			? list.filter(
					(c) => c.name.toLowerCase().includes(needle) || c.geo_code.toLowerCase().includes(needle)
				)
			: list;
		return Promise.resolve(
			filtered.slice(0, 50).map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }))
		);
	}

	function countryInitialFetcher(): Promise<AutocompleteItem[]> {
		const list = data.countries ?? [];
		return Promise.resolve(
			list.slice(0, 8).map((c) => ({ id: c.geo_code, label: c.name, sublabel: c.geo_code }))
		);
	}

	function countryLabel(id: string): string {
		return (data.countries ?? []).find((c) => c.geo_code === id)?.name ?? id;
	}

	const pageTitle = $derived(t('organizations.new.title', 'New organization — {site}'));
	const pageDescription = $derived(
		t('organizations.new.description', 'Create a new organization on {site}.')
	);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]';

	// Auto-suggest slug from name unless the user has manually edited the slug.
	function suggestSlug(value: string): string {
		return value
			.toLowerCase()
			.trim()
			.replace(/[^a-z0-9\s-]/g, '')
			.replace(/\s+/g, '-')
			.replace(/-+/g, '-')
			.replace(/^-|-$/g, '');
	}

	function onNameInput() {
		if (!slugTouched) {
			slug = suggestSlug(name);
		}
	}

	function onSlugInput() {
		slugTouched = true;
	}
</script>

<svelte:head>
	<title>{pageTitle}</title>
	<meta name="description" content={pageDescription} />
	<meta property="og:title" content={pageTitle} />
	<meta property="og:description" content={pageDescription} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="border-b border-[var(--color-border)] px-4 py-3" style="margin-inline: max(1rem, calc((100% - 48rem)/2))">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Building2 size={20} /> {t('organizations.new.heading', 'New organization')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('organizations.new.subtitle', 'Create a new organization on {site}.')}
		</p>
	</div>

	<div class="mx-auto w-full max-w-3xl p-4">
		{#if form?.message}
			<div
				class="mb-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-text-subtle)]"
			>
				{form.message}
			</div>
		{/if}

		<form
			method="POST"
			use:enhance={() => {
				submitting = true;
				return async ({ update }) => {
					await update();
					submitting = false;
				};
			}}
			class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		>
			<div class="space-y-4">
				<div>
					<label for="name" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('organizations.new.field_name', 'Name')}
					</label>
					<input
						id="name"
						name="name"
						type="text"
						required
						bind:value={name}
						oninput={onNameInput}
						placeholder={t('organizations.new.name_placeholder', 'Organization name')}
						class={inputClass}
					/>
				</div>

				<div>
					<label for="slug" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('organizations.new.field_slug', 'Slug')}
					</label>
					<input
						id="slug"
						name="slug"
						type="text"
						required
						bind:value={slug}
						oninput={onSlugInput}
						placeholder={t('organizations.new.slug_placeholder', 'organization-slug')}
						class={inputClass}
					/>
					<p class="mt-1 text-xs text-[var(--color-muted)]">
						{t('organizations.new.slug_hint', 'Used in the URL: /org/your-slug')}
					</p>
				</div>

				<div>
					<label for="tagline" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('organizations.new.field_tagline', 'Tagline')}
					</label>
					<input
						id="tagline"
						name="tagline"
						type="text"
						bind:value={tagline}
						placeholder={t('organizations.new.tagline_placeholder', 'A short tagline')}
						class={inputClass}
					/>
				</div>

				<div>
					<label for="description" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('organizations.new.field_description', 'Description')}
					</label>
					<textarea
						id="description"
						name="description"
						rows="4"
						bind:value={description}
						placeholder={t('organizations.new.description_placeholder', 'Describe the organization…')}
						class={inputClass}
					></textarea>
				</div>

				<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
					<div>
						<label for="website_url" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
							{t('organizations.new.field_website', 'Website URL')}
						</label>
						<input
							id="website_url"
							name="website_url"
							type="url"
							bind:value={websiteUrl}
							placeholder="https://example.com"
							class={inputClass}
						/>
					</div>

					<div>
						<label for="country_code" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
							{t('organizations.new.field_country', 'Country')}
						</label>
						<!-- Hidden input submits the selected geo_code as country_code -->
						<input type="hidden" name="country_code" value={countryCode} />
						<Autocomplete
							placeholder={t('organizations.new.country_placeholder', 'Search country…')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={countryFetcher}
							initialFetcher={countryInitialFetcher}
							selectedId={countryCode || null}
							selectedLabel={countryCode ? countryLabel(countryCode) : null}
							onSelect={(item) => { countryCode = item.id; }}
							onClear={() => { countryCode = ''; }}
						/>
					</div>
				</div>

				<div>
					<label for="org_type" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('organizations.new.field_type', 'Organization type')}
					</label>
					<AutocompleteSelect
						id="org_type"
						name="org_type"
						bind:value={orgType}
						disabled={metaUnavailable}
						options={data.orgMeta?.org_types ?? []}
					/>
					{#if metaUnavailable}
						<p class="mt-1 text-xs text-[var(--color-danger)]" role="alert">
							{t('organizations.new.meta_unavailable', 'Organization types could not be loaded. Please try again later.')}
						</p>
					{/if}
				</div>

				<div class="flex justify-end">
					<button
						type="submit"
						disabled={submitting || metaUnavailable}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{#if submitting}
							<Loader2 size={16} class="animate-spin" />
						{:else}
							<Plus size={16} />
						{/if}
						{t('organizations.new.submit', 'Create organization')}
					</button>
				</div>
			</div>
		</form>
	</div>
</div>
