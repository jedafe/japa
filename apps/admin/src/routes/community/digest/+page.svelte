<script lang="ts">
	// /community/digest — admin HITL review queue for community digests (§84).
	// Lists digest editions with server-side pagination, filter, and sort.
	// Admin can publish drafts/reviews and trigger manual generation.
	import { onMount } from 'svelte';
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { digestAdminApi, spacesAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, Sparkles } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

	// DigestItem matches the backend DigestEditionDTO (§84 API contract).
	type DigestItem = {
		id: string;
		space_id?: string;
		cadence: string;
		status: string;
		scheduled_for?: string;
		published_at?: string;
		email_sent: boolean;
		generated_by?: string;
		reviewed_by?: string;
		created_at: string;
		// ADR-088 AI generation columns
		ai_generated?: boolean;
		ai_model?: string;
		hitl_reviewed_by?: string;
		ai_generated_at?: string;
	};

	const canManage = $derived(hasPermission('digest.manage'));

	// ── State ────────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let showGenerateModal = $state(false);

	// Destination Spaces for the dest filter (§84 Geo Awareness). Loaded once
	// on mount from /admin/spaces?level=country so the filter options are
	// driven by the backend (Rule 16 — no hardcoded lists).
	type SpaceOption = { id: string; name: string };
	let destOptions = $state<SpaceOption[]>([]);
	onMount(async () => {
		try {
			const res = await spacesAdminApi.list('?level=country&limit=200');
			if (res.ok && Array.isArray(res.data)) {
				destOptions = (res.data as Array<{ id: string; name: string }>).map((s) => ({
					id: s.id,
					name: s.name
				}));
			}
		} catch {
			// Non-fatal — the dest filter simply stays empty.
		}
	});

	async function publishDigest(item: DigestItem) {
		acting = true;
		pageError = '';
		try {
			const result = await digestAdminApi.publish(item.id);
			if (result.ok) {
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	function openGenerate() {
		showGenerateModal = true;
	}

	async function confirmGenerate() {
		acting = true;
		pageError = '';
		try {
			const result = await digestAdminApi.generate();
			if (result.ok) {
				showGenerateModal = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: DigestItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('app.digest.publish', 'Publish'),
				icon: CheckCircle,
				onclick: publishDigest,
				show: (row: DigestItem) => row.status !== 'published' && row.status !== 'cancelled'
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'cadence', label: t('app.digest.col_cadence', 'Cadence'), type: 'text' },
		{ key: 'status', label: t('app.digest.col_status', 'Status'), type: 'text' }
	];

	const filters = $derived<FilterDef[]>([
		{
			key: 'status',
			label: t('app.digest.col_status', 'Status'),
			options: [
				{ value: 'draft', label: t('app.digest.status_draft', 'Draft') },
				{ value: 'review', label: t('app.digest.status_review', 'Review') },
				{ value: 'published', label: t('app.digest.status_published', 'Published') },
				{ value: 'cancelled', label: t('app.digest.status_cancelled', 'Cancelled') }
			]
		},
		{
			key: 'dest',
			label: t('app.digest.col_destination', 'Destination'),
			options: destOptions.map((s) => ({ value: s.id, label: s.name }))
		}
	]);

	const config = $derived<EntityPageConfig>({
		title: t('app.digest.review_queue_heading', 'Digest Review Queue'),
		description: t('app.digest.review_queue_desc', 'Review and publish community digests'),
		readonly: true,
		pageSize: 25,
		filters,
		columns: [
			{ key: 'cadence', label: t('app.digest.col_cadence', 'Cadence'), sortable: true },
			{ key: 'status', label: t('app.digest.col_status', 'Status'), sortable: true },
			{
				key: 'scheduled_for',
				label: t('app.digest.col_scheduled_for', 'Scheduled For'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			},
			{
				key: 'published_at',
				label: t('app.digest.col_published_at', 'Published'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			},
			{
				key: 'email_sent',
				label: t('app.digest.col_email_sent', 'Email Sent'),
				render: (value: boolean) => (value ? '✓' : '—')
			},
			{
				key: 'created_at',
				label: t('app.digest.col_created', 'Created'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			const queryParts: string[] = [`page=${params.page}`, `limit=${params.pageSize}`];
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			if (params.sort) {
				// EntityPage passes sort as "key:dir" (e.g. "created_at:desc").
				const [sortKey, sortDir] = params.sort.split(':');
				if (sortKey) queryParts.push(`sort=${encodeURIComponent(sortKey)}`);
				if (sortDir) queryParts.push(`order=${sortDir === 'asc' ? 'asc' : 'desc'}`);
			}
			if (params.filter?.status) queryParts.push(`status=${encodeURIComponent(params.filter.status)}`);
			if (params.filter?.dest) queryParts.push(`dest=${encodeURIComponent(params.filter.dest)}`);
			const queryString = queryParts.join('&');

			const result = await digestAdminApi.list(queryString);
			if (result.ok) {
				const data = (result.data.data as DigestItem[]) ?? [];
				const total = result.data.pagination?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / params.pageSize));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: DigestView
	});
</script>

<svelte:head>
	<title>{t('app.digest.review_queue_page_title', 'Digest Review Queue — JapaConnect Admin')}</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	{#if canManage}
		<Button onclick={openGenerate} disabled={acting}>
			<Sparkles class="mr-1.5 h-4 w-4" />
			{t('app.digest.generate', 'Generate')}
		</Button>
	{/if}
</div>

<EntityPage {config} bind:controller={entityController} />

<!-- ── Generate digest modal ──────────────────────────────────────────────── -->
<Modal bind:open={showGenerateModal} title={t('app.digest.modal_generate', 'Generate Digest')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.digest.generate_desc', 'Generate a new community digest draft from recent activity. The edition enters a 1-hour review window before auto-publish.')}
		</p>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showGenerateModal = false)}>{t('app.digest.cancel', 'Cancel')}</Button>
		<Button onclick={confirmGenerate} disabled={acting}>
			{acting ? t('app.digest.generating', 'Generating...') : t('app.digest.generate', 'Generate')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet DigestView({ row }: { row: DigestItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_cadence', 'Cadence')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.cadence || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_scheduled_for', 'Scheduled For')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.scheduled_for ? new Date(row.scheduled_for).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_published_at', 'Published')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.published_at ? new Date(row.published_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_email_sent', 'Email Sent')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.email_sent ? '✓' : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
