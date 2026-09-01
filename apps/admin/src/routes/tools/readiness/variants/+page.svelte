<script lang="ts">
	// /tools/readiness/variants — admin page for generating and editing
	// origin/destination-specific roadmap guide variants (Section 71 / ADR-071).
	// Ref: japaconnect-engineering/sections/71-readiness-assessment.md
	import {
		EntityPage,
		type EntityPageConfig,
		type EntityPageController,
		type FieldDef
	} from '$lib/components/entity';
	import { api, contentAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import type { Column, FilterDef } from '$lib/components/ui/data-table';

	type Country = { code: string; name: string };
	type JourneyOptions = {
		purposes?: { value: string; label: string }[];
		stages?: { value: string; label: string }[];
		visa_routes?: { value: string; label: string }[];
	};
	type FieldValues = Record<string, any>;
	type ContentItem = {
		id: string;
		slug: string;
		title: string;
		body?: string;
		status: string;
		content_type: string;
		guide_category?: string;
		field_values?: Record<string, any>;
		target_country?: string;
		source_country?: string;
		journey_stage?: string;
		visa_route?: string;
		purpose?: string;
		created_at?: string;
	};

	// ── State ───────────────────────────────────────────────────────────────
	let allContent = $state<ContentItem[]>([]);
	let loadedAll = $state(false);
	let countries = $state<Country[]>([]);
	let statusOptions = $state<{ value: string; label: string }[]>([]);
	let journeyOptions = $state<JourneyOptions>({});
	let entityController = $state<EntityPageController | undefined>(undefined);

	// Rule 16: option lists come from the API, not hardcoded arrays.
	$effect(() => {
		loadOptions();
	});

	async function loadOptions() {
		const [countriesRes, statusRes, journeyRes] = await Promise.all([
			loadCountries(),
			contentAdminApi.getStatusOptions(),
			api<JourneyOptions>('/feed/journey-options')
		]);
		if (countriesRes) countries = countriesRes;
		if (statusRes?.ok) statusOptions = (statusRes.data as { value: string; label: string }[]) ?? [];
		if (journeyRes?.ok) journeyOptions = (journeyRes.data as JourneyOptions) ?? {};
	}

	async function loadCountries(): Promise<Country[] | null> {
		const all: Country[] = [];
		const seen = new Set<string>();
		let offset = 0;
		while (true) {
			const res = await api<{ geo_code: string; name: string }[]>(
				`/spaces?level=country&sort=name&page_size=100&page_offset=${offset}`
			);
			if (!res.ok) return null;
			const page = ((res.data as { geo_code: string; name: string }[] | null) ?? [])
				.filter((c) => c.geo_code && !seen.has(c.geo_code))
				.map((c) => {
					seen.add(c.geo_code);
					return { code: c.geo_code, name: c.name };
				});
			if (page.length === 0) break;
			all.push(...page);
			offset += page.length;
			if (page.length < 100) break;
		}
		return all;
	}

	function isVariant(c: ContentItem) {
		return !!c.source_country || !!c.target_country;
	}

	function parseFieldValues(raw: any): Record<string, any> {
		if (!raw) return {};
		if (typeof raw === 'object') return raw as Record<string, any>;
		if (typeof raw === 'string') {
			try {
				const decoded = (globalThis as any).atob(raw);
				return JSON.parse(decoded) as Record<string, any>;
			} catch {
				return {};
			}
		}
		return {};
	}

	function transformRow(raw: any): ContentItem {
		const fieldValues = parseFieldValues(raw.field_values);
		return {
			...raw,
			field_values: fieldValues,
			purpose: (raw.purpose ?? fieldValues?.purpose) || ''
		};
	}

	async function fetchAllContent() {
		if (loadedAll) return;
		const all: ContentItem[] = [];
		let cursor = '';
		while (true) {
			const q = cursor
				? `?type=guide&page_size=100&cursor=${encodeURIComponent(cursor)}`
				: '?type=guide&page_size=100';
			const res = await api<ContentItem[]>(`/admin/content${q}`);
			if (!res.ok) throw new Error(res.error);
			const page = (res.data as ContentItem[] | null) ?? [];
			if (page.length === 0) break;
			const roadmap = page.map(transformRow);
			all.push(...roadmap);
			const last = page[page.length - 1];
			const nextCursor = last && last.created_at ? `${last.created_at}|${last.id}` : '';
			if (!nextCursor || nextCursor === cursor) break;
			cursor = nextCursor;
			if (page.length < 100) break;
		}
		allContent = all;
		loadedAll = true;
	}

	async function load(params: {
		page: number;
		pageSize: number;
		q: string;
		sort: string;
		filter: Record<string, string>;
	}) {
		if (!loadedAll) {
			await fetchAllContent();
		}

		let rows = allContent.filter((c) => isVariant(c) && c.guide_category === 'roadmap');

		if (params.q) {
			const lower = params.q.toLowerCase();
			rows = rows.filter(
				(r) => r.title.toLowerCase().includes(lower) || r.slug.toLowerCase().includes(lower)
			);
		}

		for (const [key, val] of Object.entries(params.filter)) {
			if (!val) continue;
			rows = rows.filter((r) => (r as any)[key] === val);
		}

		if (params.sort) {
			const [sortKey, sortDir] = params.sort.split(':');
			rows = [...rows].sort((a, b) => {
				const av = String((a as any)[sortKey] ?? '');
				const bv = String((b as any)[sortKey] ?? '');
				if (av < bv) return sortDir === 'asc' ? -1 : 1;
				if (av > bv) return sortDir === 'asc' ? 1 : -1;
				return 0;
			});
		}

		const total = rows.length;
		const totalPages = Math.max(1, Math.ceil(total / params.pageSize));
		const start = params.page * params.pageSize;
		return { data: rows.slice(start, start + params.pageSize), total, totalPages };
	}

	const baseGuides = $derived(
		allContent.filter((c) => !isVariant(c) && (c.field_values?.dimension || c.guide_category === 'roadmap'))
	);

	function getBaseGuide(id: string) {
		return baseGuides.find((g) => g.id === id);
	}

	function getSelectedDimension(value: Record<string, any>, editing: any): string {
		if (editing) {
			return (editing.field_values?.dimension as string) ?? '';
		}
		const base = getBaseGuide(value.base_guide_id);
		return (base?.field_values?.dimension as string) ?? (value.dimension as string) ?? '';
	}

	function generateVariant(value: Record<string, any>, editing: any) {
		if (editing) return;
		const base = getBaseGuide(value.base_guide_id);
		if (!base) return;
		if (!value.source_country && !value.target_country) return;
		const origin = value.source_country || 'any';
		const dest = value.target_country || 'any';
		const dim = (base.field_values?.dimension as string) ?? '';
		const parts = [value.target_country, value.source_country].filter(Boolean);
		const suffix = parts.length ? `-${parts.map((c) => c.toLowerCase()).join('-')}` : '';
		value.title = `${base.title} — ${origin} to ${dest}`;
		value.slug = `${base.slug}${suffix}`;
		value.body = `Placeholder guide for ${origin} → ${dest}. ${base.body ?? ''} Replace with accurate, up-to-date advice before publishing.`;
		value.dimension = dim;
	}

	function buildPayload(value: Record<string, any>) {
		const dim = getSelectedDimension(value, null);
		const fieldValues: Record<string, any> = { placeholder_variant: 'true' };
		if (dim) fieldValues.dimension = dim;
		if (value.purpose) fieldValues.purpose = value.purpose;

		return {
			content_type: 'guide',
			title: (value.title ?? '').trim(),
			slug: (value.slug ?? '').trim(),
			body: (value.body ?? '').trim(),
			body_html: '',
			status: value.status || 'draft',
			visibility: 'public',
			guide_category: 'roadmap',
			field_values: fieldValues,
			journey_stage: value.journey_stage || null,
			visa_route: value.visa_route || null,
			source_country: value.source_country || null,
			target_country: value.target_country || null
		};
	}

	async function create(body: Record<string, any>) {
		if (!body.title?.trim() || !body.slug?.trim()) {
			return { ok: false, error: t('readiness.variants.missing_required', 'Title and slug are required') };
		}
		const payload = buildPayload(body);
		const res = await api<unknown>('/content', { method: 'POST', body: payload });
		if (!res.ok) return { ok: false, error: res.error };
		allContent = [];
		loadedAll = false;
		entityController?.reload();
		return { ok: true };
	}

	async function update(id: string, body: Record<string, any>) {
		if (!body.title?.trim() || !body.slug?.trim()) {
			return { ok: false, error: t('readiness.variants.missing_required', 'Title and slug are required') };
		}
		const payload = buildPayload(body);
		const res = await api<unknown>(`/admin/content/${id}`, { method: 'PATCH', body: payload });
		if (!res.ok) return { ok: false, error: res.error };
		allContent = [];
		loadedAll = false;
		entityController?.reload();
		return { ok: true };
	}

	async function remove(id: string) {
		const res = await api<unknown>(`/admin/content/${id}/remove`, { method: 'POST', body: {} });
		if (!res.ok) return { ok: false, error: res.error };
		allContent = [];
		loadedAll = false;
		entityController?.reload();
		return { ok: true };
	}

	const columns: Column[] = $derived([
		{ key: 'title', label: t('readiness.variants.col_title', 'Title'), sortable: true },
		{ key: 'slug', label: t('readiness.variants.col_slug', 'Slug'), sortable: true },
		{ key: 'source_country', label: t('readiness.variants.col_source', 'Origin') },
		{ key: 'target_country', label: t('readiness.variants.col_target', 'Destination') },
		{ key: 'journey_stage', label: t('readiness.variants.col_stage', 'Stage') },
		{ key: 'visa_route', label: t('readiness.variants.col_visa', 'Visa') },
		{ key: 'purpose', label: t('readiness.variants.purpose', 'Purpose') },
		{ key: 'status', label: t('common.status', 'Status'), sortable: true }
	]);

	const filters: FilterDef[] = $derived([
		{
			key: 'status',
			label: t('common.status', 'Status'),
			type: 'select',
			options: statusOptions
		},
		{
			key: 'source_country',
			label: t('readiness.variants.col_source', 'Origin'),
			type: 'select',
			options: countries.map((c) => ({ value: c.code, label: `${c.name} (${c.code})` }))
		},
		{
			key: 'target_country',
			label: t('readiness.variants.col_target', 'Destination'),
			type: 'select',
			options: countries.map((c) => ({ value: c.code, label: `${c.name} (${c.code})` }))
		},
		{
			key: 'journey_stage',
			label: t('readiness.variants.col_stage', 'Stage'),
			type: 'select',
			options: journeyOptions.stages ?? []
		},
		{
			key: 'visa_route',
			label: t('readiness.variants.col_visa', 'Visa'),
			type: 'select',
			options: journeyOptions.visa_routes ?? []
		},
		{
			key: 'purpose',
			label: t('readiness.variants.purpose', 'Purpose'),
			type: 'select',
			options: journeyOptions.purposes ?? []
		}
	]);

	const fields: FieldDef[] = $derived([
		{ key: 'base_guide_id', label: t('readiness.variants.base_guide', 'Base Guide'), type: 'select', options: baseGuides.map((g) => ({ value: g.id, label: g.title })) },
		{ key: 'source_country', label: t('readiness.variants.origin_country', 'Origin Country'), type: 'select', options: countries.map((c) => ({ value: c.code, label: `${c.name} (${c.code})` })) },
		{ key: 'target_country', label: t('readiness.variants.destination_country', 'Destination Country'), type: 'select', options: countries.map((c) => ({ value: c.code, label: `${c.name} (${c.code})` })) },
		{ key: 'journey_stage', label: t('readiness.variants.journey_stage', 'Journey Stage'), type: 'select', options: journeyOptions.stages ?? [] },
		{ key: 'visa_route', label: t('readiness.variants.visa_route', 'Visa Route'), type: 'select', options: journeyOptions.visa_routes ?? [] },
		{ key: 'purpose', label: t('readiness.variants.purpose', 'Purpose'), type: 'select', options: journeyOptions.purposes ?? [] },
		{ key: 'dimension', label: t('readiness.variants.dimension', 'Roadmap Dimension'), type: 'text' },
		{ key: 'title', label: t('readiness.variants.title_label', 'Title'), type: 'text' },
		{ key: 'slug', label: t('readiness.variants.slug_label', 'Slug'), type: 'text' },
		{ key: 'body', label: t('readiness.variants.body', 'Body'), type: 'textarea' },
		{ key: 'status', label: t('common.status', 'Status'), type: 'select', options: statusOptions }
	]);

	const config: EntityPageConfig = $derived({
		title: t('readiness.variants.title', 'Roadmap Variants'),
		description: t(
			'readiness.variants.description',
			'Create and manage origin/destination-specific roadmap guide variants.'
		),
		columns,
		filters,
		fields,
		readonly: !hasPermission('content.create'),
		createPermission: 'content.create',
		editPermission: 'content.edit_any',
		deletePermission: 'content.delete_any',
		pageSize: 25,
		load,
		create,
		update,
		remove,
		formSnippet
	});
</script>

{#snippet formSnippet({ value, editing }: { value: Record<string, any>; editing: any | null })}
	{#key editing?.id}
		<div class="space-y-4 p-4">
			<div>
				<label for="rv-base" class="block text-sm font-medium" style="color: var(--color-foreground)">
					{t('readiness.variants.base_guide', 'Base Guide')}
				</label>
				<select
					id="rv-base"
					bind:value={value.base_guide_id}
					disabled={!!editing}
					onchange={(e: Event) => {
						value.base_guide_id = (e.currentTarget as HTMLSelectElement).value;
						generateVariant(value, editing);
					}}
					class="mt-1 w-full rounded-lg border p-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				>
					<option value="">{t('readiness.variants.select_base', 'Select a base guide')}</option>
					{#each baseGuides as guide}
						<option value={guide.id}>{guide.title}</option>
					{/each}
				</select>
			</div>

			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="rv-source" class="block text-sm font-medium" style="color: var(--color-foreground)">
						{t('readiness.variants.origin_country', 'Origin Country')}
					</label>
					<select
						id="rv-source"
						bind:value={value.source_country}
						onchange={(e: Event) => {
							value.source_country = (e.currentTarget as HTMLSelectElement).value;
							generateVariant(value, editing);
						}}
						class="mt-1 w-full rounded-lg border p-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					>
						<option value="">{t('readiness.variants.any', 'Any / Global')}</option>
						{#each countries as c}
							<option value={c.code}>{c.name} ({c.code})</option>
						{/each}
					</select>
				</div>
				<div>
					<label for="rv-dest" class="block text-sm font-medium" style="color: var(--color-foreground)">
						{t('readiness.variants.destination_country', 'Destination Country')}
					</label>
					<select
						id="rv-dest"
						bind:value={value.target_country}
						onchange={(e: Event) => {
							value.target_country = (e.currentTarget as HTMLSelectElement).value;
							generateVariant(value, editing);
						}}
						class="mt-1 w-full rounded-lg border p-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					>
						<option value="">{t('readiness.variants.any', 'Any / Global')}</option>
						{#each countries as c}
							<option value={c.code}>{c.name} ({c.code})</option>
						{/each}
					</select>
				</div>
			</div>

			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="rv-stage" class="block text-sm font-medium" style="color: var(--color-foreground)">
						{t('readiness.variants.journey_stage', 'Journey Stage')}
					</label>
					<select
						id="rv-stage"
						bind:value={value.journey_stage}
						class="mt-1 w-full rounded-lg border p-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					>
						<option value="">{t('readiness.variants.any', 'Any')}</option>
						{#each journeyOptions.stages ?? [] as opt}
							<option value={opt.value}>{opt.label}</option>
						{/each}
					</select>
				</div>
				<div>
					<label for="rv-visa" class="block text-sm font-medium" style="color: var(--color-foreground)">
						{t('readiness.variants.visa_route', 'Visa Route')}
					</label>
					<select
						id="rv-visa"
						bind:value={value.visa_route}
						class="mt-1 w-full rounded-lg border p-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					>
						<option value="">{t('readiness.variants.any', 'Any')}</option>
						{#each journeyOptions.visa_routes ?? [] as opt}
							<option value={opt.value}>{opt.label}</option>
						{/each}
					</select>
				</div>
			</div>

			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="rv-purpose" class="block text-sm font-medium" style="color: var(--color-foreground)">
						{t('readiness.variants.purpose', 'Purpose')}
					</label>
					<select
						id="rv-purpose"
						bind:value={value.purpose}
						class="mt-1 w-full rounded-lg border p-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					>
						<option value="">{t('readiness.variants.any', 'Any')}</option>
						{#each journeyOptions.purposes ?? [] as opt}
							<option value={opt.value}>{opt.label}</option>
						{/each}
					</select>
				</div>
			</div>

			<div>
				<span class="block text-sm font-medium" style="color: var(--color-foreground)">
					{t('readiness.variants.dimension', 'Roadmap Dimension')}
				</span>
				<span class="text-sm" style="color: var(--color-muted)">
					{getSelectedDimension(value, editing) || t('readiness.variants.no_dimension', '—')}
				</span>
			</div>

			<div>
				<label for="rv-title" class="block text-sm font-medium" style="color: var(--color-foreground)">
					{t('readiness.variants.title_label', 'Title')}
				</label>
				<input
					id="rv-title"
					type="text"
					bind:value={value.title}
					class="mt-1 w-full rounded-lg border p-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				/>
			</div>

			<div>
				<label for="rv-slug" class="block text-sm font-medium" style="color: var(--color-foreground)">
					{t('readiness.variants.slug_label', 'Slug')}
				</label>
				<input
					id="rv-slug"
					type="text"
					bind:value={value.slug}
					class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				/>
			</div>

			<div>
				<label for="rv-body" class="block text-sm font-medium" style="color: var(--color-foreground)">
					{t('readiness.variants.body', 'Body')}
				</label>
				<textarea
					id="rv-body"
					bind:value={value.body}
					rows="6"
					class="mt-1 w-full rounded-lg border p-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				></textarea>
			</div>

			<div>
				<label for="rv-status" class="block text-sm font-medium" style="color: var(--color-foreground)">
					{t('common.status', 'Status')}
				</label>
				<select
					id="rv-status"
					bind:value={value.status}
					class="mt-1 w-full rounded-lg border p-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				>
					{#each statusOptions as opt}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>

		</div>
	{/key}
{/snippet}

<svelte:head>
	<title>{t('readiness.variants.title', 'Roadmap Variants')} — JapaConnect Admin</title>
</svelte:head>

<EntityPage {config} bind:controller={entityController} />
