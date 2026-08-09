<script lang="ts">
	// /tools/housing-guide — admin HITL review queue for AI-generated housing guides.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, Sparkles } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { housingGuideAdminApi } from '$lib/api';

	type HousingGuideItem = {
		id: string;
		title: string;
		dest_country: string;
		status: string;
		ai_generated: boolean;
		updated_at?: string;
		summary?: string;
		content?: string;
	};

	const canManage = $derived(hasPermission('housing_guide.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	// ── Generate modal state ────────────────────────────────────────────────
	let showGenerateModal = $state(false);
	let generateDestCountry = $state('');
	let generateTopic = $state('');

	function openGenerate() {
		generateDestCountry = '';
		generateTopic = '';
		showGenerateModal = true;
	}

	async function confirmGenerate() {
		acting = true;
		pageError = '';
		try {
			const result = await housingGuideAdminApi.generate({
				dest_country: generateDestCountry,
				topic: generateTopic
			});
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

	async function approveItem(item: HousingGuideItem) {
		acting = true;
		pageError = '';
		try {
			const result = await housingGuideAdminApi.approve(item.id);
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

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: HousingGuideItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.approve', 'Approve'),
				icon: CheckCircle,
				onclick: approveItem,
				show: () => true
			});
		}
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is never shown, but kept for shape completeness.
	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.tools.col_title', 'Title'), type: 'text' },
		{ key: 'dest_country', label: t('app.tools.col_dest_country', 'Destination Country'), type: 'text' },
		{ key: 'summary', label: t('app.tools.col_summary', 'Summary'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.housing_guide_heading', 'Housing Guide Review Queue'),
		description: t('app.tools.housing_guide_desc', 'Review and approve AI-generated housing guides'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.tools.col_title', 'Title'), sortable: true },
			{ key: 'dest_country', label: t('app.tools.col_dest_country', 'Destination'), sortable: true },
			{
				key: 'status',
				label: t('app.tools.col_status', 'Status'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'ai_generated',
				label: t('app.tools.col_ai_generated', 'AI Generated'),
				render: (value: boolean) => (value ? t('common.yes', 'Yes') : t('common.no', 'No'))
			},
			{
				key: 'updated_at',
				label: t('app.tools.col_updated_at', 'Updated'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async () => {
			const result = await housingGuideAdminApi.list();
			if (result.ok) {
				const data = (result.data as HousingGuideItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: HousingGuideView
	});
</script>

<svelte:head>
	<title>{t('app.tools.housing_guide_page_title', 'Housing Guide Review Queue — JapaConnect Admin')}</title>
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
		<Button onclick={openGenerate}>
			<Sparkles size={16} />
			{t('app.tools.generate', 'Generate')}
		</Button>
	{/if}
</div>

<EntityPage {config} bind:controller={entityController} />

<!-- ── Generate modal ────────────────────────────────────────────────────── -->
<Modal bind:open={showGenerateModal} title={t('app.tools.generate_housing_guide', 'Generate Housing Guide')}>
	<div class="space-y-4">
		<div>
			<label for="gen-dest-country" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_dest_country', 'Destination Country')}</label>
			<input
				id="gen-dest-country"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generateDestCountry}
				placeholder={t('app.tools.dest_country_placeholder', 'e.g. Japan')}
			/>
		</div>
		<div>
			<label for="gen-topic" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.topic_prompt', 'Topic / Prompt')}</label>
			<textarea
				id="gen-topic"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="4"
				bind:value={generateTopic}
				placeholder={t('app.tools.topic_placeholder', 'Describe what the housing guide should cover...')}
			></textarea>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showGenerateModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmGenerate} disabled={acting}>
			{acting ? t('app.tools.generating', 'Generating...') : t('app.tools.generate', 'Generate')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet HousingGuideView({ row }: { row: HousingGuideItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_dest_country', 'Destination Country')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.dest_country || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_ai_generated', 'AI Generated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.ai_generated ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_updated_at', 'Updated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_summary', 'Summary')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.summary || '—'}</p>
		</div>
	</div>
{/snippet}
