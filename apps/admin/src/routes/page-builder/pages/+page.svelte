<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /page-builder/pages — migrated to the EntityPage scaffold.
	// List + page metadata CRUD via EntityPage; state actions (publish/archive)
	// keep page-local confirm Modals and refetch via entityController.reload().
	// The block-editor detail page lives at /page-builder/pages/[id] (untouched):
	// Edit navigates there rather than opening a SlideOver form.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { pagesApi, pageTemplatesAdminApiExtended, pageBuilderAdminApiExtended, api, apiRaw, contentAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { Eye, Pencil, Trash2, Send, Archive, Rocket, Copy } from 'lucide-svelte';
	import { landingPalettes, type Block } from '@japaconnect/landing-blocks';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type Page = {
		id: string;
		title: string;
		slug: string;
		status: string;
		page_type?: string;
		template_id?: string;
		theme_id?: string;
		created_at: string;
		updated_at: string;
	};

	const canManagePages = $derived(hasPermission('pages.manage'));
	const canPublishPages = $derived(hasPermission('pages.publish'));

	// slugify derives a URL-safe slug from a free-form title so the backend
	// (CreatePageInput.Slug — optional) always receives a valid value even
	// when the admin leaves the Slug field blank. Mirrors the legacy page.
	function slugify(value: string): string {
		return value
			.toLowerCase()
			.trim()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-+|-+$/g, '');
	}

	// ── Template / theme options loaded on mount (failures keep blank-only selects) ──
	let templateOptions = $state<Array<{ value: string; label: string }>>([
		{ value: '', label: t('page_builder.blank_template', 'Blank (no template)') }
	]);
	let themeOptions = $state<Array<{ value: string; label: string }>>([
		{ value: '', label: t('page_builder.default_theme', 'Default') }
	]);
	// Page statuses are shared with the content status catalog (Rule 16).
	// The page builder only uses draft / published / archived.
	let statusOptions = $state<Array<{ value: string; label: string }>>([]);
	let pageTypeOptions = $state<Array<{ value: string; label: string }>>([]);

	async function loadTemplatesAndThemes() {
		// Fetch with a large page_size so dropdown options aren't truncated
		// by the default 20-item pagination.
		const [tplRes, themeRes] = await Promise.all([
			pageTemplatesAdminApiExtended.list('page_size=100'),
			pageBuilderAdminApiExtended.listThemes('page_size=100')
		]);
		if (tplRes.ok) {
			const tpls = (tplRes.data as any[]) ?? [];
			templateOptions = [
				{ value: '', label: t('page_builder.blank_template', 'Blank (no template)') },
				...tpls.map((tpl) => ({ value: String(tpl.id), label: String(tpl.name) }))
			];
		}
		if (themeRes.ok) {
			const themes = (themeRes.data as any[]) ?? [];
			themeOptions = [
				{ value: '', label: t('page_builder.default_theme', 'Default') },
				...themes.map((th) => ({
					value: String(th.id),
					label: th.is_default ? `${String(th.name)} (default)` : String(th.name)
				}))
			];
		}
	}

	async function loadStatusOptions() {
		const res = await contentAdminApi.getStatusOptions();
		if (res.ok) {
			const all = (res.data as Array<{ value: string; label: string }>) ?? [];
			const allowed = new Set(['draft', 'published', 'archived']);
			statusOptions = all.filter((o) => allowed.has(o.value));
		}
	}

	async function loadPageTypeOptions() {
		const res = await pageBuilderAdminApiExtended.getPageTypes();
		if (res.ok) {
			pageTypeOptions = (res.data as Array<{ value: string; label: string }>) ?? [];
		}
	}

	onMount(() => {
		loadTemplatesAndThemes();
		loadStatusOptions();
		loadPageTypeOptions();
	});

	// ── Bulk delete ───── EntityPage auto-reloads + clears selection after a bulk action handler completes.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => pagesApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete pages');
		} finally {
			bulkDeleting = false;
		}
	}

	// seedHomepage creates the editable guest default homepage as a real
	// `pages` row (slug='homepage', page_type='landing', status='published')
	// seeded from landingPalettes['homepage'].defaultSeed. Once it exists it
	// shows up in this list and is editable at /page-builder/pages/[id].
	// Ref: Section 16 (Landing consolidation) + ADR-058 §5 (guest homepage).
	//
	// `blocks` is sent as a parsed array (NOT JSON.stringify'd) because api()
	// already stringifies the body — double-encoding produced a JSON string
	// that the backend's json.RawMessage + sanitizeBlocksJSON mishandled,
	// yielding an empty homepage.
	async function seedHomepage() {
		const seed = landingPalettes['homepage']?.defaultSeed ?? [];
		const seedBlocks = seed.map((entry, i) => ({
			id: `seed-${i}`,
			type: entry.type,
			visible: true,
			label: entry.label,
			props: entry.props,
			motion: entry.motion,
		})) as unknown as Block[];

		// If a homepage page already exists, re-seed its blocks (self-heal:
		// earlier versions created an empty/corrupted row) then open the
		// editor. Otherwise create it. Filter by slug so pagination doesn't
		// hide the homepage on a later page.
		const result = await pagesApi.list('slug=homepage&page_size=100');
		if (result.ok) {
			const existing = (result.data as Page[])?.find((p) => p.slug === 'homepage');
			if (existing) {
				try {
					const upd = await pagesApi.update(existing.id, {
						title: 'Homepage',
						page_type: 'landing',
						status: 'published',
						blocks: seedBlocks,
					});
					if (!upd.ok) {
						pageError = upd.error ?? t('common.error', 'Failed to seed homepage');
						return;
					}
				} catch {
					pageError = t('common.error', 'Failed to seed homepage');
					return;
				}
				goto(`/page-builder/pages/${existing.id}`);
				return;
			}
		}

		try {
			const res = await api<{ id: string }>('/admin/pages', {
				method: 'POST',
				body: {
					title: 'Homepage',
					slug: 'homepage',
					page_type: 'landing',
					status: 'published',
					blocks: seedBlocks,
				},
			});
			if (!res.ok) {
				pageError = res.error ?? t('common.error', 'Failed to seed homepage');
				return;
			}
			goto(`/page-builder/pages/${res.data.id}`);
		} catch {
			pageError = t('common.error', 'Failed to seed homepage');
		}
	}

	// duplicatePage clones a page (blocks/meta/theme/page_type) into a new draft
	// via POST /admin/pages/{id}/duplicate. Used to take the default guest
	// landing page (or any page) and redesign the copy for another app's
	// homepage. On success it navigates straight to the new page's editor.
	let duplicating = $state(false);
	async function duplicatePage(row: Page) {
		if (duplicating) return; // in-flight guard
		duplicating = true;
		pageError = '';
		try {
			const res = await pagesApi.duplicate(row.id);
			if (res.ok) {
				const newId = (res.data as any)?.id;
				if (newId) goto(`/page-builder/pages/${newId}`);
			} else {
				pageError = res.error ?? t('common.error', 'Failed to duplicate page');
			}
		} catch {
			pageError = t('common.error', 'Failed to duplicate page');
		} finally {
			duplicating = false;
		}
	}

	// ── State-action confirmation modals (publish / archive) ────────────────
	// EntityPage's confirm Modal only covers default delete; state actions keep
	// page-local Modals with richer copy, confirm calls API + controller.reload().
	let acting = $state(false);
	let actionTarget = $state<Page | null>(null);
	let showPublishConfirm = $state(false);
	let showArchiveConfirm = $state(false);

	function openPublish(row: Page) {
		actionTarget = row;
		showPublishConfirm = true;
	}
	function openArchive(row: Page) {
		actionTarget = row;
		showArchiveConfirm = true;
	}

	async function confirmPublish() {
		if (!actionTarget) return;
		acting = true;
		try {
			const result = await pagesApi.publish(actionTarget.id);
			if (result.ok) {
				showPublishConfirm = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to publish page');
			}
		} catch {
			pageError = t('common.error', 'Failed to publish page');
		} finally {
			acting = false;
		}
	}

	async function confirmArchive() {
		if (!actionTarget) return;
		acting = true;
		try {
			const result = await pagesApi.archive(actionTarget.id);
			if (result.ok) {
				showArchiveConfirm = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to archive page');
			}
		} catch {
			pageError = t('common.error', 'Failed to archive page');
		} finally {
			acting = false;
		}
	}

	function statusBadge(status: string) {
		switch (status) {
			case 'published':
				return 'bg-success/15 text-success dark:text-success';
			case 'draft':
				return 'bg-muted/15 text-muted dark:text-muted';
			case 'archived':
				return 'bg-warning/15 text-warning dark:text-warning';
			default:
				return 'bg-muted/15 text-muted dark:text-muted';
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	// Custom actions replace defaults. Edit navigates to the dnd-kit block
	// editor at /page-builder/pages/[id] rather than opening a SlideOver form.
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: Page, ctx) => ctx?.openView(row) },
			{ label: t('common.edit', 'Edit'), icon: Pencil, onclick: (row: Page) => goto(`/page-builder/pages/${row.id}`), show: () => canManagePages },
		{
			label: t('page_builder.duplicate', 'Duplicate'),
			icon: Copy,
			onclick: duplicatePage,
			show: () => canManagePages
		},
			{
				label: t('page_builder.publish', 'Publish'),
				icon: Send,
				onclick: openPublish,
				show: (row: Page) => row.status === 'draft' && canPublishPages
			},
			{
				label: t('page_builder.archive', 'Archive'),
				icon: Archive,
				onclick: openArchive,
				// Archiving is a state transition gated by pages.publish on the
				// backend — match the permission so manage-only admins don't
				// see an action that always 403s.
				show: (row: Page) => row.status === 'published' && canPublishPages
			},
			{
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row: Page, ctx) => ctx?.openDelete(row),
				show: () => canManagePages
			}
		];
		return acts;
	});

	// ── Template / theme autocomplete helpers (filter pre-loaded options client-side) ──
	function fetchTemplateOptions(q: string): Promise<{ id: string; label: string }[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? templateOptions
			: templateOptions.filter((o) => o.label.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((o) => ({ id: o.value, label: o.label })));
	}

	function fetchTemplateOptionsInitial(): Promise<{ id: string; label: string }[]> {
		return Promise.resolve(templateOptions.map((o) => ({ id: o.value, label: o.label })));
	}

	function getTemplateSelectedLabel(id: string): string {
		return templateOptions.find((o) => o.value === id)?.label ?? id;
	}

	function fetchThemeOptions(q: string): Promise<{ id: string; label: string }[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? themeOptions
			: themeOptions.filter((o) => o.label.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((o) => ({ id: o.value, label: o.label })));
	}

	function fetchThemeOptionsInitial(): Promise<{ id: string; label: string }[]> {
		return Promise.resolve(themeOptions.map((o) => ({ id: o.value, label: o.label })));
	}

	function getThemeSelectedLabel(id: string): string {
		return themeOptions.find((o) => o.value === id)?.label ?? id;
	}

	// ── Form fields ── `fields` drives EntityForm rendering + edit-form seeding.
	const fields = $derived.by<FieldDef[]>(() => [
		{ key: 'title', label: t('page_builder.title', 'Title'), type: 'text', help: t('page_builder.title_hint', 'The title shown in listings and detail views.'), required: true },
		{
			key: 'slug',
			label: t('page_builder.slug', 'Slug'),
			type: 'text',
			help: t('page_builder.slug_hint', 'URL-safe identifier — lowercase, hyphens, no spaces.'),
			placeholder: t('page_builder.slug_hint', 'Auto-generated from title')
		},
		{
			key: 'page_type',
			label: t('page_builder.page_type', 'Page type'),
			type: 'select',
			options: pageTypeOptions.length ? pageTypeOptions : [
				{ value: 'landing', label: t('page_builder.landing', 'Landing') },
				{ value: 'about', label: 'About' },
				{ value: 'service', label: 'Service' },
				{ value: 'event', label: 'Event' },
				{ value: 'custom', label: 'Custom' }
			],
			help: t('page_builder.page_type_hint', 'Landing pages use the full block library with geo-targeting and motion')
		},
		{
			key: 'status',
			label: t('page_builder.status', 'Status'),
			type: 'select',
			help: t('page_builder.status_hint', 'The record state — controls visibility and actions.'),
			options: statusOptions.length ? statusOptions : [
				{ value: 'draft', label: t('page_builder.status_draft', 'Draft') },
				{ value: 'published', label: t('page_builder.status_published', 'Published') },
				{ value: 'archived', label: t('page_builder.status_archived', 'Archived') }
			]
		},
		{
			key: 'template_id',
			label: t('page_builder.template', 'Template'),
			type: 'autocomplete',
			placeholder: t('page_builder.template_placeholder', 'Search template…'),
			emptyMessage: t('common.no_results', 'No results found'),
			fetcher: fetchTemplateOptions,
			initialFetcher: fetchTemplateOptionsInitial,
			getSelectedLabel: getTemplateSelectedLabel,
			help: t('page_builder.template_hint', 'Start from a pre-built template')
		},
		{
			key: 'theme_id',
			label: t('page_builder.theme', 'Theme'),
			type: 'autocomplete',
			help: t('page_builder.theme_id_hint', 'The visual theme applied.'),
			placeholder: t('page_builder.theme_placeholder', 'Search theme…'),
			emptyMessage: t('common.no_results', 'No results found'),
			fetcher: fetchThemeOptions,
			initialFetcher: fetchThemeOptionsInitial,
			getSelectedLabel: getThemeSelectedLabel
		}
	]);

	// Config built as a $derived so template/theme option updates (loaded on mount) propagate.
	const config = $derived<EntityPageConfig>({
		title: t('page_builder.pages_title', 'Pages'),
		description: t('page_builder.pages_description', 'Manage pages for the page builder'),
		createLabel: t('page_builder.create_page', 'Create Page'),
		createPermission: 'pages.manage',
		pageSize: 25,
		// Seeding creates the page directly in 'published' status, which the
		// backend gates on pages.publish — hide the action for manage-only
		// admins so they don't hit a 403.
		headerActions: canManagePages && canPublishPages
			? [{
					label: t('page_builder.seed_homepage', 'Create Default Homepage'),
					icon: Rocket,
					onclick: seedHomepage,
					variant: 'outline'
				}]
			: undefined,
		columns: [
			{ key: 'title', label: t('page_builder.title', 'Title'), sortable: true },
			{
				key: 'slug',
				label: t('page_builder.slug', 'Slug'),
				sortable: true,
				render: (v: string) => `<span style="color: var(--color-muted)">/${v}</span>`
			},
			{
				key: 'status',
				label: t('page_builder.status', 'Status'),
				sortable: true,
				render: (v: string) =>
					`<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${statusBadge(v)}">${v}</span>`
			},
			{
				key: 'template_id',
				label: t('page_builder.template', 'Template'),
				render: (v: string) =>
					v
						? `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
						: `<span style="color: var(--color-muted)">—</span>`
			},
			{
				key: 'updated_at',
				label: t('page_builder.updated', 'Updated'),
				sortable: true,
				render: (v: string) => new Date(v).toLocaleDateString()
			}
		],
		filters: [
			{
				key: 'status',
				label: t('page_builder.status', 'Status'),
				options: statusOptions.length ? statusOptions : [
					{ value: 'draft', label: t('page_builder.status_draft', 'Draft') },
					{ value: 'published', label: t('page_builder.status_published', 'Published') },
					{ value: 'archived', label: t('page_builder.status_archived', 'Archived') }
				]
			}
		],
		actions: rowActions,
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ q, filter, page, pageSize, sort }) => {
			const params = new URLSearchParams();
			params.set('page_size', String(pageSize));
			// EntityPage's page is 0-based; the API's `page` is 1-based.
			params.set('page', String(page + 1));
			if (sort) {
				const [key, dir] = sort.split(':');
				params.set('sort', key);
				if (dir) params.set('dir', dir);
			}
			if (filter.status) params.set('status', filter.status);
			if (q) params.set('q', q);
			const result = await apiRaw<{ data?: Page[]; total?: number; total_pages?: number }>(
				`/admin/pages?${params.toString()}`
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
			throw new Error(result.error ?? t('common.error', 'Failed to load pages'));
		},
		create: async (body) => {
			const payload: Record<string, unknown> = {
				title: String(body.title ?? '').trim(),
				slug: String(body.slug ?? '').trim() || slugify(String(body.title ?? '')),
				page_type: String(body.page_type ?? 'custom')
			};
			if (body.status) payload.status = String(body.status);
			if (body.template_id) payload.template_id = String(body.template_id);
			if (body.theme_id) payload.theme_id = String(body.theme_id);
			const res = await pagesApi.create(payload as any);
			return res.ok ? { ok: true, id: (res.data as any)?.id } : { ok: false, error: res.error };
		},
		onCreated: (id: string) => goto(`/page-builder/pages/${id}`),
		remove: async (id) => {
			const res = await pagesApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: PageView
	});
</script>

<svelte:head>
	<title>{t('page_builder.pages_title', 'Pages')} — {siteName()} Admin</title>
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

<!-- ── State-action confirmation modals ─────────────────────────────────── -->

<!-- Publish -->
<Modal bind:open={showPublishConfirm} title={t('page_builder.publish', 'Publish')}>
	{#if actionTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('page_builder.publish_confirm', 'Publish page')} <strong>{actionTarget.title}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			{t('page_builder.publish_hint', 'This will make the page publicly visible.')}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showPublishConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmPublish} disabled={acting}>
			{acting ? t('common.saving', 'Saving...') : t('page_builder.publish', 'Publish')}
		</Button>
	</div>
</Modal>

<!-- Archive -->
<Modal bind:open={showArchiveConfirm} title={t('page_builder.archive', 'Archive')}>
	{#if actionTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('page_builder.archive_confirm', 'Archive page')} <strong>{actionTarget.title}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-warning)">
			{t('page_builder.archive_hint', 'Archived pages are hidden from the public site.')}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showArchiveConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmArchive} disabled={acting}>
			{acting ? t('common.saving', 'Saving...') : t('page_builder.archive', 'Archive')}
		</Button>
	</div>
</Modal>

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

<!-- View slide-over body (mirrors the legacy view) -->
{#snippet PageView({ row }: { row: Page })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('page_builder.title', 'Title')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('page_builder.slug', 'Slug')}
			</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">/{row.slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('page_builder.page_type', 'Page type')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.page_type ?? 'custom'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('page_builder.status', 'Status')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('page_builder.template', 'Template')}
			</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.template_id ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('page_builder.updated', 'Updated')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{new Date(row.updated_at).toLocaleString()}
			</p>
		</div>
	</div>
{/snippet}
