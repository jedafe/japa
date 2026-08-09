<script lang="ts">
	// /community/digest — admin HITL review queue for community digests.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { digestAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, Sparkles } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type DigestItem = {
		id: string;
		title: string;
		issue_date: string;
		status: string;
		created_at: string;
		excerpt?: string;
	};

	const canManage = $derived(hasPermission('calendar.review'));

	// ── State ────────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let showGenerateModal = $state(false);
	let generateTopic = $state('');

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
		generateTopic = '';
		showGenerateModal = true;
	}

	async function confirmGenerate() {
		acting = true;
		pageError = '';
		try {
			const result = await digestAdminApi.generate({ topic: generateTopic });
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
				show: (row: DigestItem) => row.status !== 'published'
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.digest.col_title', 'Title'), type: 'text' },
		{ key: 'issue_date', label: t('app.digest.col_issue_date', 'Issue Date'), type: 'date' },
		{ key: 'status', label: t('app.digest.col_status', 'Status'), type: 'text' },
		{ key: 'excerpt', label: t('app.digest.detail_excerpt', 'Excerpt'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.digest.review_queue_heading', 'Digest Review Queue'),
		description: t('app.digest.review_queue_desc', 'Review and publish community digests'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.digest.col_title', 'Title'), sortable: true },
			{
				key: 'issue_date',
				label: t('app.digest.col_issue_date', 'Issue Date'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			},
			{ key: 'status', label: t('app.digest.col_status', 'Status'), sortable: true },
			{
				key: 'created_at',
				label: t('app.digest.col_created', 'Created'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async () => {
			const result = await digestAdminApi.list();
			if (result.ok) {
				const data = (result.data as DigestItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
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
			{t('app.digest.generate_desc', 'Generate a new community digest draft from recent activity.')}
		</p>
		<div>
			<label for="generate-topic" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.digest.topic_optional', 'Topic (optional)')}</label>
			<input
				id="generate-topic"
				type="text"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generateTopic}
				placeholder={t('app.digest.topic_placeholder', 'e.g. Upcoming events in Tokyo')}
			/>
		</div>
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
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_issue_date', 'Issue Date')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.issue_date ? new Date(row.issue_date).toLocaleDateString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.col_created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.digest.detail_excerpt', 'Excerpt')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.excerpt || '—'}</p>
		</div>
	</div>
{/snippet}
