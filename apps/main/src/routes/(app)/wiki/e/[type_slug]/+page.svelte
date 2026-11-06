<script lang="ts">
	import SeoHead from '$lib/seo/SeoHead.svelte';
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Entity Directory amendment)
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { t, siteName } from '$lib/i18n';
	import { EntityHero } from '$lib/components/ui';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import PaginationControls from '$lib/components/ui/PaginationControls.svelte';
	import { api } from '$lib/api/client';
	import { invalidateAll } from '$app/navigation';
	import { ChevronRight, Plus, Search, Table2, X } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	let items = $derived(data.items ?? []);
	let total = $derived(data.total ?? 0);
	let label = $derived(data.label ?? data.kind);
	let pageCount = $derived(Math.max(1, Math.ceil(total / data.limit)));
	let search = $state(data.q ?? '');
	let showCreate = $state(false);
	let creating = $state(false);
	let createErr = $state('');
	let form = $state<Record<string, string>>({});
	let uniPicks = $state<Set<string>>(new Set());
	let universities = $state<{ slug: string; name: string }[]>([]);
	let countries = $state<{ slug: string; name: string }[]>([]);
	let programs = $state<{ slug: string; name: string }[]>([]);

	const isProduct = $derived(data.kind === 'product');
	const isCourse = $derived(data.kind === 'course');
	const isEmbassy = $derived(data.kind === 'embassy');
	const isBank = $derived(data.kind === 'bank');
	const isVisa = $derived(data.kind === 'visa_route');
	const isScholar = $derived(data.kind === 'scholarship');
	const needsCountry = $derived(isEmbassy || isVisa || isScholar || isBank);

	async function openCreate() {
		showCreate = true;
		if (isCourse) {
			const [us, ps] = await Promise.all([
				api.get<{ items: { slug: string; name: string }[] }>('/wiki/pickers/spaces?type=university'),
				api.get<{ items: { slug: string; name: string }[] }>('/wiki/entity-kinds/program/entities?limit=200')
			]);
			universities = us.data?.items ?? [];
			programs = (ps.data?.items ?? []).map((p: any) => ({ slug: p.slug, name: p.name }));
		}
		if (needsCountry) {
			const cs = await api.get<{ items: { slug: string; name: string }[] }>('/wiki/pickers/spaces?type=country');
			countries = cs.data?.items ?? [];
		}
	}

	function toggleUni(slug: string) {
		const next = new Set(uniPicks);
		next.has(slug) ? next.delete(slug) : next.add(slug);
		uniPicks = next;
	}

	async function submitCreate() {
		createErr = '';
		if (!form.slug?.trim()) { createErr = t('wiki.entities.err_slug', 'Slug is required'); return; }
		creating = true;
		const body: Record<string, unknown> = { slug: form.slug.trim() };
		if (isProduct) body.name = form.name ?? form.slug;
		else if (isCourse) {
			body.university_slugs = [...uniPicks];
			body.program_slug = form.program_slug;
		} else if (isEmbassy) {
			body.name = form.name;
			body.host_slug = form.country;
		} else if (isBank) {
			body.name = form.name;
			body.host_slug = form.country || undefined;
			body.country_code = form.country_code || undefined;
		} else if (isVisa) {
			body.name = form.name;
			body.destination_slug = form.country;
			body.visa_category = form.visa_category;
			body.gov_source_url = form.gov_source_url || undefined;
		} else if (isScholar) {
			body.name = form.name;
			body.country = form.country;
			body.source_url = form.source_url || undefined;
		} else {
			body.title = form.name ?? form.slug;
			body.language = form.language || 'en';
		}
		const res = await api.post<{ slug?: string; href?: string }>(`/wiki/e/${data.slug}`, body);
		creating = false;
		if (res.error) { createErr = res.error.message || t('wiki.entities.err_create', 'Create failed'); return; }
		showCreate = false;
		form = {};
		await invalidateAll();
	}

	function applySearch() {
		const url = new URL(page.url);
		if (search) url.searchParams.set('q', search);
		else url.searchParams.delete('q');
		url.searchParams.delete('page');
		goto(url.toString(), { keepFocus: true });
	}

	function setPage(p: number) {
		const url = new URL(page.url);
		url.searchParams.set('page', String(p));
		goto(url.toString());
	}
</script>

<SeoHead title={`${label} — ${t('wiki.entities.title', 'Entity Wiki')} — ${siteName()}`} description={t('wiki.entities.type_description', 'Community-maintained wiki entries for {name} — maintained by members on {site}.').replace('{name}', label)} />

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<EntityHero
		class="mt-4"
		icon={Table2}
		accent="var(--color-primary)"
		heading={label}
		description={t('wiki.entities.type_description', 'Community-maintained wiki entries for {name} — maintained by members on {site}.').replace('{name}', label)}
		crumbs={[
			{ label: t('common.home', 'Home'), href: '/' },
			{ label: t('wiki.entities.title', 'Entity Wiki'), href: '/wiki/e' },
			{ label }
		]}
		stats={[
			{ icon: Table2, value: String(total), label: t('wiki.entities.stat_entries', 'Entries') }
		]}
	/>

	{#if data.canCreate}
		<div class="mt-4" style="margin-inline: var(--page-gutter)">
			{#if !showCreate}
				<button
					type="button"
					onclick={openCreate}
					class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-muted)] transition-colors hover:border-[var(--color-primary)]/60 hover:text-[var(--color-primary)]"
				>
					<Plus size={13} /> {t('wiki.entities.new_entry', 'New {label}').replace('{label}', label)}
				</button>
			{:else}
				<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<div class="mb-3 flex items-center justify-between">
						<p class="text-sm font-semibold text-[var(--color-text)]">{t('wiki.entities.new_entry', 'New {label}').replace('{label}', label)}</p>
						<button type="button" onclick={() => (showCreate = false)} class="text-[var(--color-muted)] hover:text-[var(--color-text)]"><X size={15} /></button>
					</div>
					<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
						<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
							{t('wiki.entities.fld_slug', 'Slug')}
							<input bind:value={form.slug} class="inp" placeholder="my-{data.slug}" />
						</label>
						{#if !isCourse}
							<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								{t('wiki.entities.fld_name', 'Name')}
								<input bind:value={form.name} class="inp" />
							</label>
						{/if}
						{#if isCourse}
							<fieldset class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)] sm:col-span-2">
								<legend class="mb-1">{t('wiki.entities.fld_universities', 'Universities')}</legend>
								<div class="flex max-h-40 flex-col gap-1 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] p-2">
									{#each universities as u (u.slug)}
										<label class="flex items-center gap-2 px-1 py-0.5 text-sm text-[var(--color-text)]">
											<input type="checkbox" checked={uniPicks.has(u.slug)} onchange={() => toggleUni(u.slug)} />
											{u.name}
										</label>
									{/each}
								</div>
							</fieldset>
							<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								{t('wiki.entities.fld_program', 'Program')}
								<select bind:value={form.program_slug} class="inp">
									<option value="">—</option>
									{#each programs as pr (pr.slug)}<option value={pr.slug}>{pr.name}</option>{/each}
								</select>
							</label>
						{/if}
						{#if needsCountry && !isBank}
							<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								{isEmbassy ? t('wiki.entities.fld_host', 'Host country') : isVisa ? t('wiki.entities.fld_destination', 'Destination country') : t('wiki.entities.fld_country', 'Country')}
								<select bind:value={form.country} class="inp">
									<option value="">—</option>
									{#each countries as c (c.slug)}<option value={c.slug}>{c.name}</option>{/each}
								</select>
							</label>
						{/if}
						{#if isBank}
							<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								{t('wiki.entities.fld_country', 'Country')}
								<select bind:value={form.country} class="inp">
									<option value="">—</option>
									{#each countries as c (c.slug)}<option value={c.slug}>{c.name}</option>{/each}
								</select>
							</label>
							<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								{t('wiki.entities.fld_country_code', 'Country code')}
								<input bind:value={form.country_code} class="inp" placeholder="NG" maxlength="2" />
							</label>
						{/if}
						{#if isVisa}
							<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								{t('wiki.entities.fld_visa_category', 'Visa category')}
								<select bind:value={form.visa_category} class="inp">
									{#each ['student','work','tourist','business','family','pr'] as vc}<option value={vc}>{vc}</option>{/each}
								</select>
							</label>
							<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								{t('wiki.entities.fld_gov_url', 'Gov source URL')}
								<input bind:value={form.gov_source_url} class="inp" placeholder="https://…" />
							</label>
						{/if}
						{#if isScholar}
							<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								{t('wiki.entities.fld_source_url', 'Source URL')}
								<input bind:value={form.source_url} class="inp" placeholder="https://…" />
							</label>
						{/if}
						{#if !isProduct && !isCourse && !isEmbassy && !isBank && !isVisa && !isScholar}
							<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								{t('wiki.entities.fld_language', 'Language')}
								<input bind:value={form.language} class="inp" placeholder="en" />
							</label>
						{/if}
					</div>
					{#if createErr}
						<p class="mt-2 text-xs text-[var(--color-danger,#dc2626)]">{createErr}</p>
					{/if}
					<div class="mt-3 flex justify-end">
						<button
							type="button"
							onclick={submitCreate}
							disabled={creating}
							class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-opacity disabled:opacity-50"
						>
							{creating ? t('common.creating', 'Creating…') : t('common.create', 'Create')}
						</button>
					</div>
				</div>
			{/if}
		</div>
	{/if}

	{#if total > 0 || search}
		<form style="margin-inline: var(--page-gutter)" class="mt-6 flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 transition-colors focus-within:border-[var(--color-primary)]/60" onsubmit={(e) => { e.preventDefault(); applySearch(); }}>
			<Search size={16} class="shrink-0 text-[var(--color-muted)]" />
			<input
				bind:value={search}
				placeholder={t('wiki.entities.search', 'Search…')}
				class="w-full bg-transparent text-sm text-[var(--color-text)] outline-none placeholder:text-[var(--color-muted)]"
			/>
		</form>
	{/if}

	{#if items.length === 0}
		<div class="mt-8">
			<EmptyState icon={Table2} title={t('wiki.entities.empty_list', 'No entries found')} />
		</div>
	{:else}
		<div class="mt-6 grid grid-cols-1 gap-2 p-4 sm:grid-cols-2" style="margin-inline: var(--page-gutter)">
			{#each items as it (it.id)}
				<a
					href={it.href}
					class="group flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)]/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-primary)]"
				>
					<span class="min-w-0 flex-1 truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]">{it.name}</span>
					{#if it.discriminator}
						<span class="shrink-0 rounded-full border border-[var(--color-border)] px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-[var(--color-muted)]">
							{it.discriminator}
						</span>
					{/if}
					<ChevronRight size={14} class="shrink-0 text-[var(--color-muted)] transition-all group-hover:translate-x-0.5 group-hover:text-[var(--color-primary)]" />
				</a>
			{/each}
		</div>
		{#if pageCount > 1}
			<div class="mt-6 flex justify-center" style="margin-inline: var(--page-gutter)">
				<PaginationControls page={data.page} totalPages={pageCount} onPageChange={setPage} />
			</div>
		{/if}
	{/if}
</div>

<style>
	.inp {
		border-radius: var(--radius);
		border: 1px solid var(--color-border);
		background: var(--color-surface);
		padding: 0.5rem 0.75rem;
		font-size: 0.875rem;
		color: var(--color-text);
		outline: none;
	}
	.inp:focus {
		border-color: var(--color-primary);
		opacity: 0.9;
	}
</style>
