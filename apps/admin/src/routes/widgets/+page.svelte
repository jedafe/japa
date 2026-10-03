<script lang="ts">
	// /widgets — Page Builder widget_configurations CRUD (Section 16).
	// Uses EntityPage with a formSnippet because the config field is
	// dynamic per widget_type (e.g. latest_content has content_type/sort/featured,
	// forum_threads has sort/space_id, reviews has entity_type, etc.).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import {
		widgetsAdminApiExtended,
		apiRaw,
		contentAdminApi,
		type CatalogOption,
		type CardTypeOption,
		type WidgetConfigField,
		type WidgetConfigOptions
	} from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	type Widget = {
		id: string;
		name: string;
		widget_type: string;
		config: Record<string, unknown>;
		cache_ttl_secs: number;
		is_active: boolean;
		created_at: string;
		updated_at: string;
	};

	// Fallback static catalog (Rule 16: replaced by the backend
	// GET /admin/pagebuilder/widget-config-options payload when it loads).
	function fallbackWidgetTypes(): CatalogOption[] {
		return [
			{ value: 'latest_content', label: 'Latest Content' },
			{ value: 'trending_tags', label: 'Trending Tags' },
			{ value: 'space_members', label: 'Space Members' },
			{ value: 'user_leaderboard', label: 'User Leaderboard' },
			{ value: 'upcoming_events', label: 'Upcoming Events' },
			{ value: 'featured_spaces', label: 'Featured Spaces' },
			{ value: 'custom_html', label: 'Custom HTML' },
			{ value: 'featured_content', label: 'Featured Content (admin-pinned)' },
			{ value: 'blog_posts', label: 'Blog Posts' },
			{ value: 'forum_threads', label: 'Forum Threads' },
			{ value: 'job_listings', label: 'Job Listings' },
			{ value: 'achievements', label: 'Achievements' },
			{ value: 'learning_tracks', label: 'Learning Tracks' },
			{ value: 'community_digest', label: 'Community Digest' },
			{ value: 'reviews', label: 'Reviews' },
			{ value: 'comparisons', label: 'Comparisons' },
				{ value: 'community_organisations', label: 'Community Organisations' },
			{ value: 'mentor_spotlight', label: 'Mentor Spotlight' },
			{ value: 'newsletter_signup', label: 'Newsletter Signup' }
		];
	}

	function fallbackConfigSchemas(): Record<string, WidgetConfigField[]> {
		return {
			latest_content: [
				{ key: 'content_type', label: 'Content Type', type: 'select', default: '', options: [{ value: '', label: 'All types' }], help: 'Filter by content type. Leave as "All types" for mixed content.' },
				{ key: 'sort', label: 'Sort', type: 'select', default: 'recent', options: [{ value: 'recent', label: 'Most Recent' }, { value: 'popular', label: 'Most Viewed' }, { value: 'discussed', label: 'Most Discussed' }] },
				{ key: 'featured', label: 'Featured Only', type: 'checkbox', default: false, help: 'Show only pinned/featured content.' },
				{ key: 'space_id', label: 'Space ID', type: 'text', default: '', help: 'Filter by space UUID (optional).' },
				{ key: 'limit', label: 'Limit', type: 'number', default: 5, help: 'Max items to return.' },
			],
			featured_content: [
				{ key: 'content_type', label: 'Content Type', type: 'select', default: '', options: [{ value: '', label: 'All types' }], help: 'Filter by content type.' },
				{ key: 'sort', label: 'Sort', type: 'select', default: 'featured', options: [{ value: 'featured', label: 'Featured Order (admin-curated)' }, { value: 'recent', label: 'Most Recent' }, { value: 'popular', label: 'Most Viewed' }, { value: 'discussed', label: 'Most Discussed' }] },
				{ key: 'space_id', label: 'Space ID', type: 'text', default: '', help: 'Filter by space UUID (optional).' },
				{ key: 'limit', label: 'Limit', type: 'number', default: 5 },
			],
			blog_posts: [
				{ key: 'sort', label: 'Sort', type: 'select', default: 'recent', options: [{ value: 'recent', label: 'Most Recent' }, { value: 'popular', label: 'Most Viewed' }, { value: 'discussed', label: 'Most Discussed' }] },
				{ key: 'space_id', label: 'Space ID', type: 'text', default: '', help: 'Filter by space UUID (optional).' },
				{ key: 'limit', label: 'Limit', type: 'number', default: 5 },
			],
			forum_threads: [
				{ key: 'sort', label: 'Sort', type: 'select', default: 'hot', options: [{ value: 'hot', label: 'Hot (vote score)' }, { value: 'recent', label: 'Most Recent' }, { value: 'unanswered', label: 'Unanswered' }] },
				{ key: 'space_id', label: 'Space ID', type: 'text', default: '', help: 'Filter by space UUID (optional).' },
				{ key: 'limit', label: 'Limit', type: 'number', default: 5 },
			],
			job_listings: [
				{ key: 'space_id', label: 'Space ID', type: 'text', default: '', help: 'Filter by space UUID (optional).' },
				{ key: 'limit', label: 'Limit', type: 'number', default: 5 },
			],
			achievements: [
				{ key: 'milestone_type', label: 'Milestone Type', type: 'select', default: '', options: [{ value: '', label: 'All milestones' }, { value: 'relocation', label: 'Relocation' }, { value: 'career', label: 'Career' }, { value: 'education', label: 'Education' }, { value: 'community', label: 'Community' }] },
				{ key: 'limit', label: 'Limit', type: 'number', default: 5 },
			],
			learning_tracks: [
				{ key: 'difficulty', label: 'Difficulty', type: 'select', default: '', options: [{ value: '', label: 'All levels' }, { value: 'beginner', label: 'Beginner' }, { value: 'intermediate', label: 'Intermediate' }, { value: 'advanced', label: 'Advanced' }] },
				{ key: 'limit', label: 'Limit', type: 'number', default: 5 },
			],
			community_digest: [
				{ key: 'space_id', label: 'Space ID', type: 'text', default: '', help: 'Filter by space UUID (optional).' },
				{ key: 'limit', label: 'Limit', type: 'number', default: 3 },
			],
			reviews: [
				{ key: 'entity_type', label: 'Entity Type', type: 'select', default: '', options: [{ value: '', label: 'All entities' }, { value: 'content', label: 'Content' }, { value: 'space', label: 'Space' }, { value: 'university', label: 'University' }] },
				{ key: 'limit', label: 'Limit', type: 'number', default: 5 },
			],
			comparisons: [
				{ key: 'limit', label: 'Limit', type: 'number', default: 5 },
			],
			community_organisations: [
				{ key: 'org_type', label: 'Organisation Type', type: 'select', default: '', options: [{ value: '', label: 'All types' }, { value: 'association', label: 'Association' }, { value: 'club', label: 'Club' }, { value: 'society', label: 'Society' }, { value: 'network', label: 'Network' }] },
				{ key: 'origin_community', label: 'Origin Community', type: 'text', default: '', help: 'Filter by origin community name.' },
				{ key: 'limit', label: 'Limit', type: 'number', default: 5 },
			],
			mentor_spotlight: [
				{ key: 'limit', label: 'Limit', type: 'number', default: 5 },
			],
			newsletter_signup: [
				{ key: 'list_id', label: 'List ID', type: 'text', default: '', help: 'Newsletter list ID for subscriptions.' },
				{ key: 'heading', label: 'Heading', type: 'text', default: 'Subscribe to our newsletter' },
				{ key: 'subtext', label: 'Subtext', type: 'text', default: '' },
				{ key: 'button_text', label: 'Button Text', type: 'text', default: 'Subscribe' },
				{ key: 'placeholder', label: 'Placeholder', type: 'text', default: 'Enter your email' },
			],
			custom_html: [
				{ key: 'html', label: 'HTML Content', type: 'text', default: '', help: 'Raw HTML (sanitized on render).' },
			],
			trending_tags: [{ key: 'limit', label: 'Limit', type: 'number', default: 10 }],
			space_members: [
				{ key: 'space_id', label: 'Space ID', type: 'text', default: '', help: 'Space UUID (required for space_members).' },
				{ key: 'limit', label: 'Limit', type: 'number', default: 5 },
			],
			user_leaderboard: [{ key: 'limit', label: 'Limit', type: 'number', default: 10 }],
			upcoming_events: [
				{ key: 'space_id', label: 'Space ID', type: 'text', default: '', help: 'Filter by space UUID (optional).' },
				{ key: 'limit', label: 'Limit', type: 'number', default: 5 },
			],
			featured_spaces: [{ key: 'limit', label: 'Limit', type: 'number', default: 5 }],
		};
	}

	function fallbackCardTypes(): CardTypeOption[] {
		return [
			{ value: 'grid', icon: '▦', label: 'Grid', desc: 'Card grid with images and excerpts' },
			{ value: 'list', icon: '☰', label: 'List', desc: 'Bordered rows with title and excerpt' },
			{ value: 'compact', icon: '●', label: 'Compact', desc: 'Pill-style tags, minimal text' },
			{ value: 'ghost', icon: '✦', label: 'Ghost', desc: 'Borderless, text-forward blog style' },
			{ value: 'magazine', icon: '📰', label: 'Magazine', desc: 'Featured story + sidebar items' },
			{ value: 'reddit', icon: '⚡', label: 'Reddit', desc: 'Compact rows with vote score and meta' },
		];
	}

	function fallbackContentWidgetTypes(): string[] {
		return [
			'latest_content', 'featured_content', 'blog_posts', 'forum_threads',
			'job_listings', 'achievements', 'learning_tracks', 'community_digest',
			'reviews', 'comparisons', 'community_organisations',
			'mentor_spotlight', 'space_members', 'user_leaderboard', 'upcoming_events',
			'featured_spaces',
		];
	}

	let widgetConfigOptions = $state<WidgetConfigOptions | undefined>(undefined);
	let contentTypeOptions = $state<CatalogOption[]>([{ value: '', label: 'All types' }]);

	$effect(() => {
		void (async () => {
			const [configRes, ctRes] = await Promise.all([
				widgetsAdminApiExtended.getConfigOptions(),
				contentAdminApi.listContentTypes()
			]);
			if (configRes.ok && configRes.data) {
				widgetConfigOptions = configRes.data;
			}
			if (ctRes.ok && ctRes.data) {
				contentTypeOptions = [{ value: '', label: 'All types' }, ...ctRes.data];
			}
		})();
	});

	const widgetTypeOptions = $derived<CatalogOption[]>(
		widgetConfigOptions?.widget_types?.length ? widgetConfigOptions.widget_types : fallbackWidgetTypes()
	);

	const cardTypeOptions = $derived<CardTypeOption[]>(
		widgetConfigOptions?.card_types?.length ? widgetConfigOptions.card_types : fallbackCardTypes()
	);

	function buildConfigSchemas(): Record<string, WidgetConfigField[]> {
		const base = widgetConfigOptions?.config_schemas ?? fallbackConfigSchemas();
		const schemas: Record<string, WidgetConfigField[]> = {};
		for (const [type, fields] of Object.entries(base)) {
			schemas[type] = fields.map((f) => ({ ...f }));
			for (const f of schemas[type]) {
				if (f.key === 'content_type') {
					f.options = contentTypeOptions;
				}
			}
		}
		return schemas;
	}

	const configSchemas = $derived<Record<string, WidgetConfigField[]>>(buildConfigSchemas());

	function isContentWidget(type: string): boolean {
		const set = new Set(widgetConfigOptions?.content_widget_types ?? fallbackContentWidgetTypes());
		return set.has(type);
	}

	// Build default config for a widget type from its schema.
	function defaultConfig(widgetType: string): Record<string, unknown> {
		const schema = configSchemas[widgetType];
		const cfg: Record<string, unknown> = {};
		// Content widgets get a default card_type; form/html/tag widgets don't.
		if (isContentWidget(widgetType)) {
			cfg.card_type = 'grid';
		}
		if (!schema) return cfg;
		for (const field of schema) {
			if (field.default !== undefined) cfg[field.key] = field.default;
		}
		return cfg;
	}

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');

	function buildConfig(): EntityPageConfig {
		const fields: FieldDef[] = [
			{ key: 'name', label: t('widgets.name', 'Name'), type: 'text', required: true },
			{
				key: 'widget_type',
				label: t('widgets.type', 'Type'),
				type: 'select',
				options: widgetTypeOptions,
				required: true,
				help: 'Data source for the widget (Section 16)'
			},
			{
				key: 'cache_ttl_secs',
				label: 'Cache TTL (seconds)',
				type: 'number',
				help: 'How long to cache rendered widget data in Valkey (default 300)'
			},
			{ key: 'is_active', label: t('widgets.active', 'Active'), type: 'checkbox' }
		];

		return {
			title: t('widgets.title', 'Widgets'),
			description: t('widgets.description', 'Manage widgets available across the platform.'),
			createLabel: t('widgets.create', 'Create'),
			pageSize: 25,
			columns: [
				{ key: 'name', label: t('widgets.name', 'Name') },
				{
					key: 'widget_type',
					label: t('widgets.type', 'Type'),
					render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>`
				},
				{ key: 'is_active', label: t('widgets.status', 'Status'), componentSnippet: StatusCell }
			],
			filters: [
				{
					key: 'is_active',
					label: t('widgets.status', 'Status'),
					options: [
						{ value: 'true', label: t('widgets.active', 'Active') },
						{ value: 'false', label: t('widgets.inactive', 'Inactive') }
					]
				}
			],
			fields,
			load: async ({ q, filter, page, pageSize }) => {
				const params = new URLSearchParams();
				params.set('page_size', String(pageSize));
				params.set('page_offset', String(page * pageSize));
				if (filter.is_active) params.set('is_active', filter.is_active);
				if (q) params.set('q', q);
				const result = await apiRaw<{ data?: Widget[]; total?: number; total_pages?: number }>(
					`/admin/widgets?${params.toString()}`
				);
				if (result.ok) {
					const d = result.data;
					const items = d.data ?? [];
					return {
						data: items,
						total: d.total ?? items.length,
						totalPages: d.total_pages ?? 1
					};
				}
				throw new Error(result.error ?? t('common.error', 'Failed to load widgets'));
			},
			create: async (body) => {
				const ttl = Number(body.cache_ttl_secs ?? 300);
				const widgetType = String(body.widget_type ?? '').trim();
				// Build config from the form values — the formSnippet stores
				// config field values into body via the `_config` key.
				const cfg = (body._config as Record<string, unknown>) ?? defaultConfig(widgetType);
				const res = await widgetsAdminApiExtended.create({
					name: String(body.name ?? '').trim(),
					widget_type: widgetType,
					config: cfg,
					cache_ttl_secs: ttl > 0 ? ttl : 300,
					is_active: body.is_active === undefined ? true : !!body.is_active
				});
				return res.ok ? { ok: true } : { ok: false, error: res.error };
			},
			update: async (id, body) => {
				const ttl = Number(body.cache_ttl_secs ?? 300);
				const widgetType = String(body.widget_type ?? '').trim();
				const cfg = (body._config as Record<string, unknown>) ?? defaultConfig(widgetType);
				const res = await widgetsAdminApiExtended.update(id, {
					name: String(body.name ?? '').trim(),
					widget_type: widgetType,
					config: cfg,
					cache_ttl_secs: ttl > 0 ? ttl : 300,
					is_active: !!body.is_active
				});
				return res.ok ? { ok: true } : { ok: false, error: res.error };
			},
			remove: async (id) => {
				const res = await widgetsAdminApiExtended.delete(id);
				return res.ok ? { ok: true } : { ok: false, error: res.error };
			},
			formSnippet: WidgetForm,
			viewSnippet: WidgetView
		};
	}

	const config = $derived<EntityPageConfig>(buildConfig());
</script>

<svelte:head>
	<title>{t('widgets.title', 'Widgets')} — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet StatusCell({ value }: { value: boolean; row: Widget })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value ? 'bg-success/15 text-success' : 'bg-muted/15 text-muted'}">
		{value ? t('widgets.active', 'Active') : t('widgets.inactive', 'Inactive')}
	</span>
{/snippet}

<!-- Create/Edit form — base fields + dynamic per-type config section -->
{#snippet WidgetForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
	{@const widgetType = String(value.widget_type ?? '')}
	{@const schema = configSchemas[widgetType] ?? []}
	{@const existingConfig = (editing?.config ?? value.config ?? defaultConfig(widgetType)) as Record<string, unknown>}

	<div class="space-y-4">
		<!-- Base field: Name -->
		<div>
			<label for="widget-name" class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t('widgets.name', 'Name')} *
			</label>
			<input
				id="widget-name"
				type="text"
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder="e.g. Latest Blog Posts"
				bind:value={value.name}
			/>
		</div>

		<!-- Base field: Widget Type -->
		<div>
			<label for="widget-type" class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t('widgets.type', 'Type')} *
			</label>
			<AutocompleteSelect
				id="widget-type"
				bind:value={value.widget_type}
				onValueChange={(v) => {
					// Reset config defaults when type changes.
					const newType = v;
					value._config = defaultConfig(newType);
				}}
				options={widgetTypeOptions}
				class="w-full"
			/>
			<p class="mt-1 text-[10px]" style="color: var(--color-muted)">Data source for the widget (Section 16)</p>
		</div>

		<!-- Base field: Cache TTL -->
		<div>
			<label for="widget-ttl" class="block text-sm font-medium" style="color: var(--color-foreground)">
				Cache TTL (seconds)
			</label>
			<input
				id="widget-ttl"
				type="number"
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder="300"
				bind:value={value.cache_ttl_secs}
			/>
			<p class="mt-1 text-[10px]" style="color: var(--color-muted)">How long to cache rendered widget data in Valkey (default 300)</p>
		</div>

		<!-- Base field: Active -->
		<div>
			<label class="flex items-center gap-2 text-sm" style="color: var(--color-foreground)">
				<input type="checkbox" bind:checked={value.is_active} />
				{t('widgets.active', 'Active')}
			</label>
		</div>

		<!-- Card type picker (shown only for content-rendering widget types) -->
		{#if isContentWidget(widgetType)}
			<div class="mt-6 border-t pt-4" style="border-color: var(--color-border)">
				<h4 class="mb-1 text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">
					Card Type
				</h4>
				<p class="mb-3 text-[10px]" style="color: var(--color-muted)">
					How content cards are rendered on the frontend. This is the source of truth — the page builder only controls the number of columns.
				</p>
				<div class="grid grid-cols-2 gap-2">
					{#each cardTypeOptions as opt}
						<button
							type="button"
							onclick={() => { value._config = { ...(value._config ?? existingConfig), card_type: opt.value }; }}
							class="flex flex-col items-start gap-1 rounded-lg border p-2.5 text-left transition-all"
							style={String((value._config ?? existingConfig).card_type ?? 'grid') === opt.value
								? 'border-color: var(--color-primary); box-shadow: 0 0 0 1px var(--color-primary); background: var(--color-surface-subtle)'
								: 'border-color: var(--color-border); background: var(--color-surface)'}
						>
							<span class="flex items-center gap-1.5 text-sm font-medium" style="color: var(--color-foreground)">
								<span class="text-base">{opt.icon}</span>
								{opt.label}
							</span>
							<span class="text-[10px] leading-tight" style="color: var(--color-muted)">{opt.desc}</span>
						</button>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Dynamic per-type config section -->
		{#if widgetType && schema.length > 0}
			<div class="mt-6 border-t pt-4" style="border-color: var(--color-border)">
				<h4 class="mb-3 text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">
					Widget Configuration
				</h4>
				<div class="space-y-4">
					{#each schema as field}
						<div>
							{#if field.type !== 'checkbox'}
								<span class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{field.label}</span>
							{/if}
							{#if field.type === 'text'}
								<input
									type="text"
									class="w-full rounded border px-2 py-1 text-sm"
									style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)"
									value={String(existingConfig[field.key] ?? field.default ?? '')}
									onchange={(e) => { value._config = { ...(value._config ?? existingConfig), [field.key]: e.currentTarget.value }; }}
								/>
							{:else if field.type === 'number'}
								<input
									type="number"
									class="w-full rounded border px-2 py-1 text-sm"
									style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)"
									value={Number(existingConfig[field.key] ?? field.default ?? 0)}
									onchange={(e) => { value._config = { ...(value._config ?? existingConfig), [field.key]: Number(e.currentTarget.value) }; }}
								/>
							{:else if field.type === 'checkbox'}
								<label class="flex items-center gap-2">
									<input
										type="checkbox"
										checked={!!(existingConfig[field.key] ?? field.default)}
										onchange={(e) => { value._config = { ...(value._config ?? existingConfig), [field.key]: e.currentTarget.checked }; }}
									/>
									<span class="text-sm" style="color: var(--color-foreground)">{field.label}</span>
								</label>
							{:else if field.type === 'select'}
								<AutocompleteSelect
									value={String(existingConfig[field.key] ?? field.default ?? '')}
									onValueChange={(v) => { value._config = { ...(value._config ?? existingConfig), [field.key]: v }; }}
									options={field.options ?? []}
									class="w-full"
								/>
							{:else if field.type === 'textarea'}
								<textarea
									rows="4"
									class="w-full rounded border px-2 py-1 text-sm"
									style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)"
									value={String(existingConfig[field.key] ?? field.default ?? '')}
									onchange={(e) => { value._config = { ...(value._config ?? existingConfig), [field.key]: e.currentTarget.value }; }}
								></textarea>
							{:else}
								<!-- Fallback for field types not listed above (e.g. url) — render
								     a text input rather than nothing. -->
								<input
									type="text"
									class="w-full rounded border px-2 py-1 text-sm"
									style="border-color: var(--color-border-input); background: var(--color-background); color: var(--color-foreground)"
									value={String(existingConfig[field.key] ?? field.default ?? '')}
									onchange={(e) => { value._config = { ...(value._config ?? existingConfig), [field.key]: e.currentTarget.value }; }}
								/>
							{/if}
							{#if field.help}
								<p class="mt-1 text-[10px]" style="color: var(--color-muted)">{field.help}</p>
							{/if}
						</div>
					{/each}
				</div>
			</div>
		{:else if widgetType && schema.length === 0}
			<div class="mt-6 border-t pt-4" style="border-color: var(--color-border)">
				<p class="text-xs" style="color: var(--color-muted)">No configuration options for this widget type.</p>
			</div>
		{/if}
	</div>
{/snippet}

<!-- View slide-over body -->
{#snippet WidgetView({ row }: { row: Widget })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('widgets.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('widgets.type', 'Type')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.widget_type}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Cache TTL</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.cache_ttl_secs ?? 300}s</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('widgets.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_active ? t('widgets.active', 'Active') : t('widgets.inactive', 'Inactive')}</p>
		</div>
		{#if row.config && Object.keys(row.config).length > 0}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Configuration</p>
				<pre class="mt-1 overflow-x-auto rounded p-2 text-xs" style="background: var(--color-surface-subtle); color: var(--color-foreground)">{JSON.stringify(row.config, null, 2)}</pre>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.created_at', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ?? '—'}</p>
		</div>
	</div>
{/snippet}
