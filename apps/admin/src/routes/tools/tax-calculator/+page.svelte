<script lang="ts">
	// /tools/tax-calculator — read-only HITL review queue with Approve Guide + AI Generate Guide actions.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { taxCalculatorAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, Sparkles } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type GuideItem = {
		id: string;
		title: string;
		dest_country: string;
		employment_type: string;
		status: string;
		ai_generated: boolean;
		updated_at: string;
		content?: string;
		summary?: string;
	};

	const canManage = $derived(hasPermission('tax_calculator.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	// Generate modal state
	let showGenerateModal = $state(false);
	let generateTopic = $state('');
	let generateCountry = $state('');
	let generateEmploymentType = $state('');
	let generating = $state(false);

	async function approveGuide(item: GuideItem) {
		acting = true;
		pageError = '';
		try {
			const result = await taxCalculatorAdminApi.approveGuide(item.id);
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
		generateCountry = '';
		generateEmploymentType = '';
		showGenerateModal = true;
	}

	async function confirmGenerate() {
		if (!generateTopic.trim()) return;
		generating = true;
		pageError = '';
		try {
			const result = await taxCalculatorAdminApi.generateGuide({
				topic: generateTopic.trim(),
				dest_country: generateCountry.trim() || undefined,
				employment_type: generateEmploymentType.trim() || undefined
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
			generating = false;
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: GuideItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.approve_guide', 'Approve Guide'),
				icon: CheckCircle,
				onclick: approveGuide,
				show: () => true
			});
			acts.push({
				label: t('common.generate_guide', 'Generate Guide'),
				icon: Sparkles,
				onclick: openGenerate,
				show: () => true
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'title', label: t('common.title', 'Title'), type: 'text' },
		{ key: 'dest_country', label: t('common.country', 'Country'), type: 'text' },
		{ key: 'employment_type', label: t('common.employment_type', 'Employment Type'), type: 'text' },
		{ key: 'content', label: t('common.content', 'Content'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.tax_calculator_heading', 'Tax Calculator Guide Review Queue'),
		description: t('app.tools.tax_calculator_desc', 'Review and approve tax calculator guide entries'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('common.title', 'Title'), sortable: true },
			{ key: 'dest_country', label: t('common.country', 'Country'), sortable: true },
			{ key: 'employment_type', label: t('common.employment_type', 'Employment Type'), sortable: true },
			{
				key: 'status',
				label: t('common.status', 'Status'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'ai_generated',
				label: t('common.ai_generated', 'AI Generated'),
				render: (value: boolean) => (value ? t('common.yes', 'Yes') : t('common.no', 'No'))
			},
			{
				key: 'updated_at',
				label: t('common.updated', 'Updated'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async () => {
			const result = await taxCalculatorAdminApi.list();
			if (result.ok) {
				const data = (result.data as GuideItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: GuideView
	});
</script>

<svelte:head>
	<title>{t('app.tools.tax_calculator_page_title', 'Tax Calculator Guide Review Queue — JapaConnect Admin')}</title>
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

<!-- ── Generate modal ────────────────────────────────────────────────────── -->
<Modal bind:open={showGenerateModal} title={t('app.tools.generate_tax_guide', 'Generate Tax Calculator Guide')}>
	<div class="space-y-4">
		<div>
			<label for="gen-topic" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.topic_prompt', 'Topic / Prompt')}</label>
			<textarea
				id="gen-topic"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="4"
				bind:value={generateTopic}
				placeholder={t('app.tools.topic_placeholder', 'Describe the tax calculator guide to generate...')}
			></textarea>
		</div>
		<div>
			<label for="gen-country" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.country', 'Country')}</label>
			<input
				id="gen-country"
				type="text"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generateCountry}
				placeholder={t('common.country_placeholder', 'e.g. Japan')}
			/>
		</div>
		<div>
			<label for="gen-emp" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.employment_type', 'Employment Type')}</label>
			<input
				id="gen-emp"
				type="text"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generateEmploymentType}
				placeholder={t('common.employment_type_placeholder', 'e.g. Full-time Employee')}
			/>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showGenerateModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmGenerate} disabled={generating || !generateTopic.trim()}>
			{generating ? t('common.generating', 'Generating...') : t('common.generate_guide', 'Generate Guide')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet GuideView({ row }: { row: GuideItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.country', 'Country')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.dest_country || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.employment_type', 'Employment Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.employment_type || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.ai_generated', 'AI Generated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.ai_generated ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.updated', 'Updated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
		</div>
		{#if row.summary}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.summary', 'Summary')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.summary}</p>
			</div>
		{/if}
		{#if row.content}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.content', 'Content')}</p>
				<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.content}</p>
			</div>
		{/if}
	</div>
{/snippet}
