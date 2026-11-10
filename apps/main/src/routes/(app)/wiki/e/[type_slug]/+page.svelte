<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { ogCardUrl } from '#lib/seo/og.js';
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Entity Directory amendment)
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { t, siteName } from '#lib/i18n/index.js';
	import { EntityHero } from '#lib/components/ui/index.js';
	import EmptyState from '#lib/components/ui/EmptyState.svelte';
	import PaginationControls from '#lib/components/ui/PaginationControls.svelte';
	import { api } from '#lib/api/client.js';
	import { invalidateAll } from '$app/navigation';
	import Autocomplete, { type AutocompleteItem } from '#lib/components/create/Autocomplete.svelte';
	import { MultiAutocomplete } from '@japaconnect/ui';
	import { staticAutocompleteFetcher } from '#lib/autocomplete.js';
	import { loadCurrencies, type CurrencyInfo } from '#lib/utils/currency.js';
	import { ChevronRight, Plus, Search, Table2, X } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	let items = $derived(data.items ?? []);
	let total = $derived(data.total ?? 0);
	let label = $derived(data.label ?? data.kind);
	let pageCount = $derived(Math.max(1, Math.ceil(total / data.limit)));
	// Seeded from the URL once, then re-synced when in-route navigation changes
	// ?q= (back/forward, pager links) — the box must not show a stale query.
	let search = $state(untrack(() => data.q ?? ''));
	$effect(() => {
		search = data.q ?? '';
	});
	let showCreate = $state(false);
	let creating = $state(false);
	let createErr = $state('');
	let createNote = $state('');
	let form = $state<Record<string, string>>({});
	let uniPicks = $state<Set<string>>(new Set());
	let uniLabels = $state<Record<string, string>>({});
	let programLabel = $state('');
	let countries = $state<{ slug: string; name: string }[]>([]);
	let currencies = $state<CurrencyInfo[]>([]);
	// Per-university offering field sets — each picked uni gets its own
	// tuition/mode/etc. (mirrors admin course-offerings per-row fields).
	let uniFieldVals = $state<Record<string, Record<string, string>>>({});
	let uniMonthVals = $state<Record<string, string[]>>({});

	const isProgram = $derived(data.kind === 'program');
	const isCourse = $derived(data.kind === 'course');
	const isEmbassy = $derived(data.kind === 'embassy');
	const isBank = $derived(data.kind === 'bank');
	const isVisa = $derived(data.kind === 'visa_route');
	const isScholar = $derived(data.kind === 'scholarship');
	const needsCountry = $derived(isEmbassy || isVisa || isScholar || isBank);

	// Canonical suggestion lists — the same academic vocabulary the admin
	// /universities/course-catalog and course-offerings forms use (§39).
	// Suggestions only: fields stay freeText so non-canonical values work.
	const DEGREE_TYPES = [
		'Associate', 'BSc', 'BA', 'BEng', 'BEd', 'BBA', 'BCom', 'BPharm', 'LLB',
		'MBBS', 'ND', 'HND', 'Diploma', 'Certificate',
		'MSc', 'MA', 'MEng', 'MEd', 'MBA', 'MCom', 'LLM', 'MPhil', 'MPH',
		'PharmD', 'DVM', 'DDS', 'PhD', 'EdD', 'DBA'
	];
	const STUDY_FIELDS = [
		'Computer Science', 'Engineering', 'Medicine', 'Law',
		'Business Administration', 'Economics', 'Accounting', 'Nursing',
		'Pharmacy', 'Architecture', 'Agriculture', 'Education', 'Mathematics',
		'Physics', 'Chemistry', 'Biology', 'Political Science', 'Sociology',
		'Psychology', 'Arts'
	];
	const LANGUAGES = [
		'English', 'French', 'Arabic', 'German', 'Spanish', 'Italian',
		'Dutch', 'Portuguese', 'Yoruba', 'Chinese'
	];
	const ACCREDITORS = [
		'NUC accredited', 'NMCN accredited', 'COREN accredited',
		'AACSB', 'ABET', 'EQUIS', 'AMBA', 'QAA', 'NAAC',
		'WASC', 'SACSCOC', 'TEQSA', 'HLC', 'CAA'
	];
	const STUDY_MODES = ['Full-time', 'Part-time', 'Distance', 'Online', 'Hybrid'];
	const MONTHS = [
		'January', 'February', 'March', 'April', 'May', 'June',
		'July', 'August', 'September', 'October', 'November', 'December'
	];

	const suggFetcher = (options: string[]) =>
		staticAutocompleteFetcher(options.map((o) => ({ id: o, label: o })));
	const degreeFetcher = suggFetcher(DEGREE_TYPES);
	const studyFieldFetcher = suggFetcher(STUDY_FIELDS);
	const languageFetcher = suggFetcher(LANGUAGES);
	const accreditorFetcher = suggFetcher(ACCREDITORS);
	const monthFetcher = suggFetcher(MONTHS);
	const currencyFetcher = $derived(
		staticAutocompleteFetcher(
			currencies.map((c) => ({ id: c.code, label: c.code, sublabel: c.name }))
		)
	);

	async function openCreate() {
		showCreate = true;
		createNote = '';
		if (needsCountry && countries.length === 0) {
			const cs = await api.get<{ items: { slug: string; name: string }[] }>('/wiki/pickers/spaces?type=country');
			countries = cs.data?.items ?? [];
		}
		if (isCourse && currencies.length === 0) {
			currencies = await loadCurrencies();
		}
	}

	// Course pickers — Autocomplete fetchers hit the live search endpoints.
	async function fetchUniversities(q: string): Promise<AutocompleteItem[]> {
		const res = await api.get<{ items: { slug: string; name: string }[] }>(
			`/wiki/pickers/spaces?type=education&q=${encodeURIComponent(q)}`
		);
		return (res.data?.items ?? [])
			.filter((u) => !uniPicks.has(u.slug))
			.map((u) => ({ id: u.slug, label: u.name }));
	}

	async function fetchPrograms(q: string): Promise<AutocompleteItem[]> {
		const res = await api.get<{ items: { slug?: string; id: string; name: string }[] }>(
			`/wiki/entity-kinds/program/entities?q=${encodeURIComponent(q)}&limit=20`
		);
		return (res.data?.items ?? []).map((p) => ({ id: p.slug ?? p.id, label: p.name }));
	}

	function pickUniversity(item: AutocompleteItem) {
		const next = new Set(uniPicks);
		next.add(item.id);
		uniPicks = next;
		uniLabels = { ...uniLabels, [item.id]: item.label };
		if (!uniFieldVals[item.id]) uniFieldVals[item.id] = {};
		if (!uniMonthVals[item.id]) uniMonthVals[item.id] = [];
	}

	function removeUniversity(slug: string) {
		const next = new Set(uniPicks);
		next.delete(slug);
		uniPicks = next;
		delete uniFieldVals[slug];
		delete uniMonthVals[slug];
	}

	// Per-uni field accessors — offerings' editable fields live in
	// uniFieldVals[uslug]/uniMonthVals[uslug], not the shared form.
	const uf = (u: string, k: string) => uniFieldVals[u]?.[k] ?? '';
	const uset = (u: string, k: string, v: string) => {
		uniFieldVals[u] = { ...(uniFieldVals[u] ?? {}), [k]: v };
	};
	const um = (u: string) => uniMonthVals[u] ?? [];
	const umset = (u: string, v: string[]) => {
		uniMonthVals[u] = v;
	};

	async function submitCreate() {
		createErr = '';
		// Slug is optional for program (auto from name) and course (auto
		// {university}-{program}) — other kinds still require it.
		if (!isProgram && !isCourse && !form.slug?.trim()) { createErr = t('wiki.entities.err_slug', 'Slug is required'); return; }
		if (isProgram && !form.name?.trim()) { createErr = t('wiki.entities.err_name', 'Name is required'); return; }
		if (isCourse && (uniPicks.size === 0 || !form.program_slug)) {
			createErr = t('wiki.entities.err_course_parents', 'University and program are required');
			return;
		}
		creating = true;
		const body: Record<string, unknown> = {};
		if (form.slug?.trim()) body.slug = form.slug.trim();
		if (isProgram) {
			// Same field set as admin /universities/course-catalog.
			body.title = form.name ?? form.slug;
			if (form.description) body.description = form.description;
			if (form.degree_type) body.degree_type = form.degree_type;
			if (form.department) body.department = form.department;
			if (form.field_of_study) body.field_of_study = form.field_of_study;
			if (form.language) body.language = form.language;
			if (form.default_duration_years) body.default_duration_years = Number(form.default_duration_years);
		} else if (isCourse) {
			// Same field set as admin /universities/course-offerings —
			// per university, so values can differ across unis.
			body.program_slug = form.program_slug;
			body.offerings = [...uniPicks].map((u) => {
				const f = uniFieldVals[u] ?? {};
				const months = uniMonthVals[u] ?? [];
				const o: Record<string, unknown> = { university_slug: u };
				if (f.tuition) o.tuition = Number(f.tuition);
				if (f.currency) o.currency = f.currency;
				if (f.duration_years) o.duration_years = Number(f.duration_years);
				if (f.study_mode) o.study_mode = f.study_mode;
				if (months.length > 0) o.intake_months = months;
				if (f.entry_requirements) o.entry_requirements = f.entry_requirements;
				if (f.accreditation) o.accreditation = f.accreditation;
				return o;
			});
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
		const res = await api.post<{ slug?: string; href?: string; skipped?: string[] }>(`/wiki/e/${data.slug}`, body);
		creating = false;
		if (res.error) { createErr = res.error.message || t('wiki.entities.err_create', 'Create failed'); return; }
		// Some universities may be skipped (unknown slug or slug conflict) —
		// surface the count instead of silently dropping them.
		const skipped = res.data?.skipped ?? [];
		if (skipped.length > 0) {
			createNote = t('wiki.entities.skipped_notice', '{n} university offering(s) skipped — slug conflict or university not found').replace('{n}', String(skipped.length));
		}
		showCreate = false;
		form = {};
		uniPicks = new Set();
		uniLabels = {};
		uniFieldVals = {};
		uniMonthVals = {};
		programLabel = '';
		await invalidateAll();
	}

	function applySearch() {
		const url = new URL(page.url.href);
		if (search) url.searchParams.set('q', search);
		else url.searchParams.delete('q');
		url.searchParams.delete('page');
		goto(url.toString(), { reset: false });
	}

	function setPage(p: number) {
		const url = new URL(page.url.href);
		url.searchParams.set('page', String(p));
		goto(url.toString());
	}
</script>

<SeoHead title={`${label} — ${t('wiki.entities.title', 'Entity Wiki')} — ${siteName()}`} description={t('wiki.entities.type_description', 'Community-maintained wiki entries for {name} — maintained by members on {site}.').replace('{name}', label)} ogImage={ogCardUrl(page.url.origin, 'wiki-entities')} />

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
					class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-muted)] transition-colors hover:border-[var(--color-primary)]/60 hover:text-[var(--color-primary-text)]"
				>
					<Plus size={13} /> {t('wiki.entities.new_entry', 'New {label}').replace('{label}', label)}
				</button>
				{#if createNote}
					<p class="mt-2 text-xs text-[var(--color-muted)]">{createNote}</p>
				{/if}
			{:else}
				<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<div class="mb-3 flex items-center justify-between">
						<p class="text-sm font-semibold text-[var(--color-text)]">{t('wiki.entities.new_entry', 'New {label}').replace('{label}', label)}</p>
						<button type="button" onclick={() => (showCreate = false)} class="text-[var(--color-muted)] hover:text-[var(--color-text)]"><X size={15} /></button>
					</div>
					<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
						<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
							{t('wiki.entities.fld_slug', 'Slug')}
							<input
								bind:value={form.slug}
								class="inp"
								placeholder={isProgram
									? t('wiki.entities.slug_auto_program', 'Auto-generated from name if empty')
									: isCourse
										? t('wiki.entities.slug_auto_course', 'Auto: {university}-{program} if empty')
										: `my-${data.slug}`}
							/>
						</label>
						{#if !isCourse}
							<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								{t('wiki.entities.fld_name', 'Name')}
								<input bind:value={form.name} class="inp" />
							</label>
						{/if}
						{#if isCourse}
							<div class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)] sm:col-span-2">
								<span>{t('wiki.entities.fld_universities', 'Universities')}</span>
								{#if uniPicks.size > 0}
									<div class="flex flex-wrap gap-1.5">
										{#each [...uniPicks] as u (u)}
											<span class="inline-flex items-center gap-1 rounded-full border border-[var(--color-border)] px-2.5 py-1 text-[11px] font-medium text-[var(--color-text)]">
												{uniLabels[u] ?? u}
												<button type="button" onclick={() => removeUniversity(u)} class="text-[var(--color-muted)] hover:text-[var(--color-text)]" aria-label="remove"><X size={12} /></button>
											</span>
										{/each}
									</div>
								{/if}
								<Autocomplete
									placeholder={t('wiki.entities.uni_placeholder', 'Search universities…')}
									emptyMessage={t('wiki.entities.no_universities', 'No universities found')}
									fetcher={fetchUniversities}
									initialFetcher={() => fetchUniversities('')}
									selectedId={null}
									onSelect={pickUniversity}
								/>
							</div>
							<div class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								<span>{t('wiki.entities.fld_program', 'Program')}</span>
								<Autocomplete
									placeholder={t('wiki.entities.program_placeholder', 'Search programs…')}
									emptyMessage={t('wiki.entities.no_programs', 'No programs found')}
									fetcher={fetchPrograms}
									initialFetcher={() => fetchPrograms('')}
									selectedId={form.program_slug || null}
									selectedLabel={programLabel}
									onSelect={(item) => { form.program_slug = item.id; programLabel = item.label; }}
									onClear={() => { form.program_slug = ''; programLabel = ''; }}
								/>
							</div>
							{#if uniPicks.size === 0}
								<p class="text-xs text-[var(--color-muted)] sm:col-span-2">{t('wiki.entities.pick_uni_hint', 'Pick a university to set offering details — each university gets its own values.')}</p>
							{:else}
								{#each [...uniPicks] as u (u)}
									<div class="flex flex-col gap-3 rounded-[var(--radius)] border border-[var(--color-border)] p-3 sm:col-span-2">
										{#if uniPicks.size > 1}
											<p class="text-xs font-semibold text-[var(--color-text)]">{uniLabels[u] ?? u}</p>
										{/if}
										<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
											<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
												{t('wiki.entities.fld_tuition', 'Tuition')}
												<input type="number" min="0" step="any" value={uf(u, 'tuition')} oninput={(e) => uset(u, 'tuition', e.currentTarget.value)} class="inp" />
											</label>
											<div class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
												<span>{t('wiki.entities.fld_currency', 'Currency')}</span>
												{#if currencies.length > 0}
													<Autocomplete
														placeholder="USD"
														emptyMessage={t('common.no_results', 'No results found')}
														fetcher={currencyFetcher}
														initialFetcher={currencyFetcher}
														selectedId={uf(u, 'currency') || null}
														selectedLabel={uf(u, 'currency') || null}
														onSelect={(item) => uset(u, 'currency', item.id)}
														onClear={() => uset(u, 'currency', '')}
													/>
												{:else}
													<input value={uf(u, 'currency')} oninput={(e) => uset(u, 'currency', e.currentTarget.value)} class="inp" placeholder="USD" />
												{/if}
											</div>
											<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
												{t('wiki.entities.fld_duration', 'Duration (years)')}
												<input type="number" min="0" step="any" value={uf(u, 'duration_years')} oninput={(e) => uset(u, 'duration_years', e.currentTarget.value)} class="inp" />
											</label>
											<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
												{t('wiki.entities.fld_study_mode', 'Study Mode')}
												<select value={uf(u, 'study_mode')} onchange={(e) => uset(u, 'study_mode', e.currentTarget.value)} class="inp">
													<option value="">—</option>
													{#each STUDY_MODES as m}<option value={m}>{m}</option>{/each}
												</select>
											</label>
											<div class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)] sm:col-span-2">
												<span>{t('wiki.entities.fld_intake_months', 'Intake Months')}</span>
												<MultiAutocomplete
													placeholder={t('wiki.entities.intake_placeholder', 'Select intake months…')}
													emptyMessage={t('common.no_results', 'No results found')}
													fetcher={monthFetcher}
													initialFetcher={monthFetcher}
													values={um(u)}
													onchange={(v) => umset(u, v)}
													clearLabel={t('common.clear', 'Clear')}
													loadingLabel={t('common.loading', 'Loading...')}
													removeLabel={t('common.remove', 'Remove')}
												/>
											</div>
											<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)] sm:col-span-2">
												{t('wiki.entities.fld_entry_requirements', 'Entry Requirements')}
												<textarea value={uf(u, 'entry_requirements')} oninput={(e) => uset(u, 'entry_requirements', e.currentTarget.value)} class="inp" rows="2"></textarea>
											</label>
											<div class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
												<span>{t('wiki.entities.fld_accreditation', 'Accreditation')}</span>
												<Autocomplete
													freeText
													placeholder={t('wiki.entities.accreditation_placeholder', 'Search accreditors…')}
													emptyMessage={t('common.no_results', 'No results found')}
													fetcher={accreditorFetcher}
													initialFetcher={accreditorFetcher}
													selectedId={null}
													selectedLabel={uf(u, 'accreditation')}
													onSelect={(item) => uset(u, 'accreditation', item.id)}
													onFreeTextBlur={(q) => uset(u, 'accreditation', q)}
												/>
											</div>
										</div>
									</div>
								{/each}
							{/if}
						{/if}
						{#if isProgram}
							<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)] sm:col-span-2">
								{t('wiki.entities.fld_description', 'Description')}
								<textarea bind:value={form.description} class="inp" rows="2"></textarea>
							</label>
							<div class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								<span>{t('wiki.entities.fld_degree_type', 'Degree Type')}</span>
								<Autocomplete
									freeText
									placeholder="BSc / MSc / MBA / PhD"
									emptyMessage={t('common.no_results', 'No results found')}
									fetcher={degreeFetcher}
									initialFetcher={degreeFetcher}
									selectedId={null}
									selectedLabel={form.degree_type}
									onSelect={(item) => (form.degree_type = item.id)}
									onFreeTextBlur={(q) => (form.degree_type = q)}
								/>
							</div>
							<div class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								<span>{t('wiki.entities.fld_department', 'Department')}</span>
								<Autocomplete
									freeText
									placeholder={t('wiki.entities.department_placeholder', 'Search departments…')}
									emptyMessage={t('common.no_results', 'No results found')}
									fetcher={studyFieldFetcher}
									initialFetcher={studyFieldFetcher}
									selectedId={null}
									selectedLabel={form.department}
									onSelect={(item) => (form.department = item.id)}
									onFreeTextBlur={(q) => (form.department = q)}
								/>
							</div>
							<div class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								<span>{t('wiki.entities.fld_field_of_study', 'Field of Study')}</span>
								<Autocomplete
									freeText
									placeholder={t('wiki.entities.field_placeholder', 'Search fields of study…')}
									emptyMessage={t('common.no_results', 'No results found')}
									fetcher={studyFieldFetcher}
									initialFetcher={studyFieldFetcher}
									selectedId={null}
									selectedLabel={form.field_of_study}
									onSelect={(item) => (form.field_of_study = item.id)}
									onFreeTextBlur={(q) => (form.field_of_study = q)}
								/>
							</div>
							<div class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								<span>{t('wiki.entities.fld_language', 'Language')}</span>
								<Autocomplete
									freeText
									placeholder="English"
									emptyMessage={t('common.no_results', 'No results found')}
									fetcher={languageFetcher}
									initialFetcher={languageFetcher}
									selectedId={null}
									selectedLabel={form.language}
									onSelect={(item) => (form.language = item.id)}
									onFreeTextBlur={(q) => (form.language = q)}
								/>
							</div>
							<label class="flex flex-col gap-1 text-xs font-medium text-[var(--color-muted)]">
								{t('wiki.entities.fld_default_duration', 'Default Duration (years)')}
								<input type="number" min="0" step="any" bind:value={form.default_duration_years} class="inp" />
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
						{#if !isProgram && !isCourse && !isEmbassy && !isBank && !isVisa && !isScholar}
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
					<span class="min-w-0 flex-1 truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary-text)]">{it.name}</span>
					{#if it.discriminator}
						<span class="shrink-0 rounded-full border border-[var(--color-border)] px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-[var(--color-muted)]">
							{it.discriminator}
						</span>
					{/if}
					<ChevronRight size={14} class="shrink-0 text-[var(--color-muted)] transition-all group-hover:translate-x-0.5 group-hover:text-[var(--color-primary-text)]" />
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
