<script lang="ts">
	// /imports — Content Import oversight (Section 65, ADR-083).
	// Migrated to the EntityPage scaffold: DataTable with server-side
	// pagination + status filter, custom row actions (View, Retry),
	// bulk retry with in-flight guard, formSnippet for the create flow
	// (UserAutocomplete + API-fetched source types), and a viewSnippet
	// with per-item error log rendering.
	// Gated by `import.manage`.
	//
	// Ref: japaconnect-engineering/sections/65-content-import.md
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Select } from '$lib/components/ui/select';
	import UserAutocomplete from '$lib/components/ui/UserAutocomplete.svelte';
	import { api, apiRaw } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, RotateCw } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

	type ImportErrorEntry = {
		item_title?: string;
		url?: string;
		error?: string;
	};

	type ContentImport = {
		id: string;
		user_id: string;
		source_type: string;
		source_url?: string | null;
		status: 'pending' | 'processing' | 'completed' | 'failed' | 'partial';
		total_items: number;
		imported_count: number;
		failed_count: number;
		error_log: ImportErrorEntry[] | string;
		created_at: string;
		completed_at?: string | null;
		username?: string;
	};

	type ImportSource = { value: string; label: string; requires_url: boolean };

	const canManage = $derived(hasPermission('import.manage'));

	// ── Source types fetched from the backend API (Rule 16) ───────────────
	let importSources = $state<ImportSource[]>([]);
	const sourceTypeOptions = $derived(
		importSources.map((s) => ({ value: s.value, label: s.label }))
	);

	async function loadSources() {
		const result = await apiRaw<{ data: ImportSource[] }>('/me/imports/sources');
		if (result.ok && Array.isArray(result.data.data)) {
			importSources = result.data.data;
		}
	}

	$effect(() => {
		loadSources();
	});

	// ── Retry confirm modal (page-local, like /users state actions) ───────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let retryConfirmOpen = $state(false);
	let retryTarget = $state<ContentImport | null>(null);
	let acting = $state(false);

	function openRetryConfirm(row: ContentImport) {
		retryTarget = row;
		retryConfirmOpen = true;
	}

	async function confirmRetry() {
		if (!retryTarget) return;
		acting = true;
		try {
			const result = await api(`/admin/imports/${retryTarget.id}/retry`, { method: 'POST' });
			if (result.ok) {
				retryConfirmOpen = false;
				retryTarget = null;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('imports.msg_retry_failed', 'Failed to retry import');
			}
		} catch {
			pageError = t('imports.msg_retry_failed', 'Failed to retry import');
		} finally {
			acting = false;
		}
	}

	// ── Bulk retry with in-flight guard (admin AGENTS.md) ─────────────────
	let bulkRetrying = $state(false);

	async function bulkRetry(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkRetrying) return; // in-flight guard
		bulkRetrying = true;
		pageError = '';
		try {
			let ok = 0;
			let fail = 0;
			for (const id of idArr) {
				const r = await api(`/admin/imports/${id}/retry`, { method: 'POST' });
				if (r.ok) ok++;
				else fail++;
			}
			if (fail > 0) {
				pageError = t('imports.msg_bulk_retry', `${ok} import(s) requeued, ${fail} failed`);
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('imports.msg_retry_failed', 'Failed to retry import');
		} finally {
			bulkRetrying = false;
		}
	}

	// ── Helpers ───────────────────────────────────────────────────────────
	function statusBadgeClass(status: string): string {
		switch (status) {
			case 'completed': return 'bg-success/15 text-success border-success';
			case 'failed': return 'bg-danger/15 text-danger border-danger';
			case 'processing': return 'bg-warning/15 text-warning border-warning';
			case 'partial': return 'bg-warning/15 text-warning border-warning';
			default: return 'bg-surface text-foreground border';
		}
	}

	function parseErrorLog(raw: ImportErrorEntry[] | string): ImportErrorEntry[] {
		if (Array.isArray(raw)) return raw;
		if (typeof raw === 'string' && raw.length > 0) {
			try {
				const parsed = JSON.parse(raw);
				return Array.isArray(parsed) ? parsed : [];
			} catch {
				return [];
			}
		}
		return [];
	}

	// ── Row actions: custom actions replace defaults, so View is included explicitly ──
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: ContentImport, ctx) => ctx?.openView(row) }
		];
		if (canManage) {
			acts.push({
				label: t('imports.retry', 'Retry'),
				icon: RotateCw,
				show: (row: ContentImport) => row.status === 'failed' || row.status === 'partial',
				onclick: (row: ContentImport) => openRetryConfirm(row)
			});
		}
		return acts;
	});

	// ── Filters: status filter is backend-supported ───────────────────────
	const filters: FilterDef[] = [
		{
			key: 'status',
			label: t('imports.col_status', 'Status'),
			options: [
				{ value: 'pending', label: t('imports.tab_in_progress', 'Pending') },
				{ value: 'processing', label: t('imports.tab_in_progress', 'Processing') },
				{ value: 'completed', label: t('imports.tab_completed', 'Completed') },
				{ value: 'partial', label: t('imports.tab_partial', 'Partial') },
				{ value: 'failed', label: t('imports.tab_failed', 'Failed') }
			]
		}
	];

	// `fields` drives EntityForm rendering and edit-form seeding. The actual
	// create form is rendered via `formSnippet` (UserAutocomplete is not a
	// standard EntityForm input), but `fields` is still required for the
	// scaffold to seed the form value object.
	const fields: FieldDef[] = [
		{ key: 'user_id', label: t('imports.target_user', 'Target User'), type: 'text', help: t('imports.user_id_hint', 'The user account this is attached to.'), required: true },
		{ key: 'source_type', label: t('imports.source_type', 'Source Type'), type: 'text', help: t('imports.source_type_hint', 'The kind of source this is.'), required: true },
		{ key: 'source_url', label: t('imports.source_url', 'Source URL'), type: 'url', help: t('imports.source_url_hint', 'The URL this data was pulled from — used for verification.') }
	];

	const config = $derived<EntityPageConfig>({
		title: t('imports.title', 'Content Imports'),
		description: t('imports.description', 'Oversight of all user content imports (Medium, Substack, RSS). Retry failed imports.'),
		createLabel: t('imports.action_new', 'New Import'),
		createPermission: 'import.manage',
		pageSize: 25,
		columns: [
			{
				key: 'username',
				label: t('imports.col_user', 'User'),
				render: (v: string, row: ContentImport) =>
					`<div><p class="font-medium text-foreground">${v || 'unknown'}</p><p class="text-xs text-muted">${row.source_type}</p></div>`
			},
			{
				key: 'source_url',
				label: t('imports.col_source_url', 'Source URL'),
				render: (v: string | null | undefined) =>
					v
						? `<a href="${v}" target="_blank" rel="noopener" class="text-xs text-info truncate hover:underline">${v}</a>`
						: `<span class="text-xs text-muted">—</span>`
			},
			{
				key: 'status',
				label: t('imports.col_status', 'Status'),
				render: (value: string) => {
					const cls = statusBadgeClass(value);
					return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold capitalize ${cls}">${value}</span>`;
				}
			},
			{
				key: 'imported_count',
				label: t('imports.col_progress', 'Progress'),
				render: (v: number, row: ContentImport) => {
					const total = row.total_items || 0;
					const failed = row.failed_count || 0;
					const failedTag = failed > 0
						? ` <span class="text-xs text-danger">· ${failed} ${t('imports.failed', 'failed')}</span>`
						: '';
					return `<span class="text-sm text-foreground-alt">${v}/${total} ${t('imports.imported', 'imported')}${failedTag}</span>`;
				}
			},
			{
				key: 'created_at',
				label: t('imports.col_created', 'Created'),
				render: (value: string) =>
					`<span class="text-sm text-foreground-alt">${new Date(value).toLocaleString()}</span>`
			},
			{
				key: 'completed_at',
				label: t('imports.col_completed', 'Completed'),
				render: (value: string | null | undefined) =>
					value
						? `<span class="text-sm text-foreground-alt">${new Date(value).toLocaleString()}</span>`
						: `<span class="text-sm text-muted">—</span>`
			}
		],
		filters,
		actions: rowActions,
		bulkActions: canManage
			? [{
					label: t('imports.action_retry_selected', 'Retry Selected'),
					icon: RotateCw,
					onclick: bulkRetry,
					disabled: (_ids: Set<string>, rows?: any[]) =>
						!rows?.some((r) => r.status === 'failed' || r.status === 'partial')
				}]
			: undefined,
		fields,
		load: async ({ page, pageSize, filter }) => {
			const params = new URLSearchParams({
				page: String(page + 1), // API is 1-based
				limit: String(pageSize)
			});
			if (filter.status) params.set('status', filter.status);
			const result = await apiRaw<{ data: ContentImport[]; pagination: { total: number } }>(
				`/admin/imports?${params.toString()}`
			);
			if (result.ok) {
				const data = Array.isArray(result.data.data) ? result.data.data : [];
				const total = result.data.pagination?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / pageSize));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('imports.msg_load_failed', 'Failed to load imports'));
		},
		create: async (body) => {
			const userId = String(body.user_id ?? '').trim();
			const sourceType = String(body.source_type ?? '').trim();
			const sourceUrl = String(body.source_url ?? '').trim();
			if (!userId) {
				return { ok: false, error: t('imports.msg_select_user', 'Please select a user') };
			}
			// Check if the selected source type requires a URL.
			const source = importSources.find((s) => s.value === sourceType);
			const requiresUrl = source ? source.requires_url : sourceType !== 'zip';
			if (requiresUrl && !sourceUrl) {
				return { ok: false, error: t('imports.msg_source_url_required', 'Source URL is required for this source type') };
			}
			const result = await api('/admin/imports', {
				method: 'POST',
				body: {
					user_id: userId,
					source_type: sourceType,
					source_url: requiresUrl ? sourceUrl : undefined
				}
			});
			return result.ok ? { ok: true } : { ok: false, error: result.error };
		},
		viewSnippet: ImportView,
		formSnippet: ImportForm
	});
</script>

<svelte:head>
	<title>{t('imports.page_title', 'Content Imports — {site} Admin')}</title>
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

<!-- Retry confirmation modal (page-local, like /users state-action modals) -->
<Modal bind:open={retryConfirmOpen} title={t('imports.modal_retry_title', 'Confirm Retry')}>
	<p class="text-sm" style="color: var(--color-foreground)">
		{t('imports.modal_retry_body', 'Are you sure you want to retry this import? The source will be re-parsed and new draft posts created for any items that previously failed.')}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (retryConfirmOpen = false)}>
			{t('imports.cancel', 'Cancel')}
		</Button>
		<Button onclick={confirmRetry} disabled={acting}>
			{acting ? t('imports.btn_retrying', 'Retrying…') : t('imports.btn_retry_import', 'Retry Import')}
		</Button>
	</div>
</Modal>

<!-- ── View snippet: import details with error log ─────────────────────── -->
{#snippet ImportView({ row }: { row: ContentImport })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('imports.detail_user', 'User')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{row.username || row.user_id}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('imports.detail_source', 'Source')}
				</p>
				<p class="mt-1 text-sm font-medium capitalize" style="color: var(--color-foreground)">
					{row.source_type}
				</p>
			</div>
			{#if row.source_url}
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('imports.detail_source_url', 'Source URL')}
					</p>
					<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">
						{row.source_url}
					</p>
				</div>
			{/if}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('imports.detail_status', 'Status')}
				</p>
				<p class="mt-1">
					<span
						class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold capitalize {statusBadgeClass(row.status)}"
					>
						{row.status}
					</span>
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('imports.detail_progress', 'Progress')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">
					{row.imported_count}/{row.total_items} {t('imports.imported', 'imported')}
					· {row.failed_count} {t('imports.failed', 'failed')}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('imports.detail_created', 'Created')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">
					{new Date(row.created_at).toLocaleString()}
				</p>
			</div>
			{#if row.completed_at}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('imports.detail_completed', 'Completed')}
					</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">
						{new Date(row.completed_at).toLocaleString()}
					</p>
				</div>
			{/if}
		</div>

		{#if parseErrorLog(row.error_log).length > 0}
			{@const errors = parseErrorLog(row.error_log)}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('imports.detail_error_log', 'Error Log')} ({errors.length})
				</p>
				<ul class="mt-2 space-y-2">
					{#each errors as err, i (i)}
						<li class="rounded-md border p-2 text-xs" style="border-color: var(--color-border)">
							{#if err.item_title}
								<p class="font-medium" style="color: var(--color-foreground)">{err.item_title}</p>
							{/if}
							{#if err.url}
								<p class="mt-0.5 break-all font-mono" style="color: var(--color-muted)">{err.url}</p>
							{/if}
							<p class="mt-0.5" style="color: var(--color-danger)">{err.error}</p>
						</li>
					{/each}
				</ul>
			</div>
		{/if}
	</div>
{/snippet}

<!-- ── Form snippet: create import form (UserAutocomplete + source type + URL) ── -->
{#snippet ImportForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
	<div class="space-y-4">
		<div>
			<label for="import-user" class="mb-1 block text-sm font-medium text-foreground-alt">
				{t('imports.target_user', 'Target User')}
			</label>
			<span class="mt-1 block text-xs" style="color: var(--color-muted)">{t('imports.user_hint', 'The user account this is attached to.')}</span>
			<UserAutocomplete
				bind:value={value._username}
				placeholder={t('imports.user_placeholder', 'Search by username…')}
				onSelectUser={(u) => { value.user_id = u.id; value._username = u.username; }}
			/>
			{#if value.user_id}
				<p class="mt-1 text-xs text-muted">ID: {value.user_id}</p>
			{/if}
		</div>
		<div>
			<label for="import-source-type" class="mb-1 block text-sm font-medium text-foreground-alt">
				{t('imports.source_type', 'Source Type')}
			</label>
			<span class="mt-1 block text-xs" style="color: var(--color-muted)">{t('imports.source_type_hint', 'The type — controls how it’s handled.')}</span>
			<Select
				bind:value={value.source_type}
				options={sourceTypeOptions}
				placeholder={t('imports.source_type', 'Source Type')}
			/>
		</div>
		{#if importSources.find((s) => s.value === value.source_type)?.requires_url ?? true}
			<div>
				<label for="import-source-url" class="mb-1 block text-sm font-medium text-foreground-alt">
					{t('imports.source_url', 'Source URL')}
				</label>
			<span class="mt-1 block text-xs" style="color: var(--color-muted)">{t('imports.source_url_hint', 'The source url for this record.')}</span>
				<input
					id="import-source-url"
					type="url"
					bind:value={value.source_url}
					placeholder="https://medium.com/@username"
					class="w-full rounded-lg border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					style="border-color: var(--color-border)"
				/>
				<p class="mt-1 text-xs text-muted">
					{t('imports.source_url_hint', 'The RSS feed URL or profile URL of the blog to import.')}
				</p>
			</div>
		{/if}
		<div class="rounded-lg border p-3 text-xs" style="border-color: var(--color-border); color: var(--color-muted)">
			{t('imports.form_help', 'Imported posts become draft blog_post content attributed to the selected user, with canonical_url set to the original source. The user can review and publish them.')}
		</div>
	</div>
{/snippet}
