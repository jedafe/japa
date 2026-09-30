<script lang="ts">
	// Ref: japaconnect-engineering/sections/07-organizations.md (my organizations)
	// Lists the orgs the current user is a member of, with a role badge and a
	// link to the per-org admin dashboard at /dashboard/organizations/{id}.
	//
	// The "New organization" action opens an inline create form within the
	// center column (no navigation to /org/new). The form POSTs /organizations
	// via the browser API client; on success the list is invalidated and the
	// user is routed to the new org's admin dashboard.
	import { t } from '$lib/i18n';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Building2, Plus, ShieldCheck, Loader2, X } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { invalidateAll, goto } from '$app/navigation';
	import { suggestSlug } from '$lib/components/create/slug';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { MyOrganizationRow } from './+page.server';
	import type { PublicOrganization } from '$lib/types';

	let { data } = $props();
	const organizations = $derived(data.organizations);

	// Country autocomplete — filters the SSR-loaded country spaces
	// (data.countries from /geo/countries) client-side (Rule 16).
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

	// Inline create-form state.
	let showCreateForm = $state(false);
	let name = $state('');
	let slug = $state('');
	let slugTouched = $state(false);
	let tagline = $state('');
	let description = $state('');
	let websiteUrl = $state('');
	let countryCode = $state('');
	let orgType = $state('business');
	let submitting = $state(false);
	let errorKey = $state('');

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	function roleKey(role: string): string {
		return `org_admin.role.${role}`;
	}

	function onNameInput() {
		if (!slugTouched) slug = suggestSlug(name);
	}

	function onSlugInput() {
		slugTouched = true;
	}

	function resetForm() {
		name = '';
		slug = '';
		slugTouched = false;
		tagline = '';
		description = '';
		websiteUrl = '';
		countryCode = '';
		orgType = 'business';
		errorKey = '';
	}

	function toggleCreateForm() {
		showCreateForm = !showCreateForm;
		if (!showCreateForm) resetForm();
	}

	async function createOrg(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';

		const trimmedName = name.trim();
		const trimmedSlug = slug.trim();
		if (!trimmedName) {
			errorKey = 'org_admin.errors.name_required';
			return;
		}
		if (!trimmedSlug) {
			errorKey = 'org_admin.errors.slug_required';
			return;
		}

		const payload: Record<string, unknown> = {
			name: trimmedName,
			slug: trimmedSlug,
			org_type: orgType
		};
		if (tagline.trim()) payload.tagline = tagline.trim();
		if (description.trim()) payload.description = description.trim();
		if (websiteUrl.trim()) payload.website_url = websiteUrl.trim();
		if (countryCode.trim()) payload.country_code = countryCode.trim();

		submitting = true;
		try {
			const res = await api.post<PublicOrganization>('/organizations', payload);
			if (res.error || !res.data) {
				errorKey = 'org_admin.errors.create_failed';
			} else {
				showCreateForm = false;
				resetForm();
				await invalidateAll();
				await goto(`/dashboard/organizations/${res.data.id}`);
			}
		} catch {
			errorKey = 'org_admin.errors.create_failed';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>{t('dashboard.nav.organizations')} — JapaConnect</title>
</svelte:head>

<section class="space-y-4">
	<header class="flex items-center justify-between">
		<div>
			<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.organizations')}</h1>
			<p class="text-sm text-[var(--color-muted)]">{t('org_admin.list_subtitle')}</p>
		</div>
		<button
			type="button"
			onclick={toggleCreateForm}
			aria-expanded={showCreateForm}
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)]"
		>
			{#if showCreateForm}
				<X class="h-4 w-4" />
				{t('common.cancel', 'Cancel')}
			{:else}
				<Plus class="h-4 w-4" />
				{t('org_admin.new_org')}
			{/if}
		</button>
	</header>

	{#if showCreateForm}
		<!-- Inline create form (center column, no navigation) -->
		<form
			onsubmit={createOrg}
			class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5"
			novalidate
		>
			<div>
				<h2 class="text-sm font-medium text-[var(--color-text)]">{t('org_admin.create_title', 'New organization')}</h2>
				<p class="mt-0.5 text-xs text-[var(--color-muted)]">{t('org_admin.create_subtitle', 'Create a new organization on JapaConnect.')}</p>
			</div>

			{#if errorKey}
				<p class="rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]" role="alert">{t(errorKey)}</p>
			{/if}

			<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
				<label class="flex flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">{t('organizations.new.field_name', 'Name')}</span>
					<input
						class={inputClass}
						value={name}
						oninput={(e) => { name = e.currentTarget.value; onNameInput(); }}
						placeholder={t('organizations.new.name_placeholder', 'Organization name')}
						required
					/>
				</label>
				<label class="flex flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">{t('organizations.new.field_slug', 'Slug')}</span>
					<input
						class={inputClass}
						value={slug}
						oninput={(e) => { slug = e.currentTarget.value; onSlugInput(); }}
						placeholder={t('organizations.new.slug_placeholder', 'organization-slug')}
						required
					/>
					<span class="text-xs text-[var(--color-muted)]">{t('organizations.new.slug_hint', 'Used in the URL: /org/your-slug')}</span>
				</label>
			</div>

			<label class="flex flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('organizations.new.field_tagline', 'Tagline')}</span>
				<input class={inputClass} value={tagline} oninput={(e) => (tagline = e.currentTarget.value)} placeholder={t('organizations.new.tagline_placeholder', 'A short tagline')} />
			</label>

			<label class="flex flex-col gap-1.5">
				<span class="text-sm font-medium text-[var(--color-text)]">{t('organizations.new.field_description', 'Description')}</span>
				<textarea class={inputClass} rows={4} value={description} oninput={(e) => (description = e.currentTarget.value)} placeholder={t('organizations.new.description_placeholder', 'Describe the organization…')}></textarea>
			</label>

			<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
				<label class="flex flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">{t('organizations.new.field_website', 'Website URL')}</span>
					<input class={inputClass} value={websiteUrl} oninput={(e) => (websiteUrl = e.currentTarget.value)} placeholder="https://example.com" />
				</label>
				<label class="flex flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">{t('organizations.new.field_country', 'Country')}</span>
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
				</label>
				<label class="flex flex-col gap-1.5">
					<span class="text-sm font-medium text-[var(--color-text)]">{t('organizations.new.field_type', 'Organization type')}</span>
					<AutocompleteSelect
						bind:value={orgType}
						options={data.orgMeta?.org_types ?? []}
						disabled={!data.orgMeta?.org_types?.length}
					/>
				</label>
			</div>

			<div class="flex items-center justify-end gap-2">
				<button
					type="button"
					onclick={toggleCreateForm}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)]"
				>
					{t('common.cancel', 'Cancel')}
				</button>
				<button
					type="submit"
					disabled={submitting}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					{#if submitting}
						<Loader2 class="h-4 w-4 animate-spin" />
					{:else}
						<Plus class="h-4 w-4" />
					{/if}
					{t('organizations.new.submit', 'Create organization')}
				</button>
			</div>
		</form>
	{/if}

	{#if organizations.length === 0 && !showCreateForm}
		<EmptyState
			icon={Building2}
			title={t('org_admin.empty_title')}
			body={t('org_admin.empty_body')}
		/>
	{:else if organizations.length > 0}
		<ul class="space-y-2">
			{#each organizations as o (o.id)}
				<li>
					<a
						href="/dashboard/organizations/{o.id}"
						class="group flex items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition hover:border-[var(--color-primary)]/40"
					>
						<div class="flex min-w-0 items-center gap-3">
							<div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
								<Building2 class="h-5 w-5" />
							</div>
							<div class="min-w-0">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">
									{o.name}
									{#if o.verified}
										<ShieldCheck class="ml-1 inline h-3.5 w-3.5 text-[var(--color-primary)]" />
									{/if}
								</p>
								<p class="truncate text-xs text-[var(--color-muted)]">
									/{o.slug} · {t('meta.members', '{n} members').replace('{n}', String(o.member_count))}
								</p>
							</div>
						</div>
						<span class="shrink-0 rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-muted)]">
							{t(roleKey(o.member_role))}
						</span>
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</section>
