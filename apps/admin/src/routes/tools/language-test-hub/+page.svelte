<script lang="ts">
	// /tools/language-test-hub — read-only HITL review queue with Approve + AI Generate actions.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { languageTestHubAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, Sparkles } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type TestItem = {
		id: string;
		test_name: string;
		dest_country: string;
		visa_category: string;
		status: string;
		ai_generated: boolean;
		updated_at: string;
		content?: string;
		summary?: string;
	};

	const canManage = $derived(hasPermission('language_test_hub.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	// Generate modal state
	let showGenerateModal = $state(false);
	let generateTopic = $state('');
	let generateCountry = $state('');
	let generateVisaCategory = $state('');
	let generating = $state(false);

	async function approveTest(item: TestItem) {
		acting = true;
		pageError = '';
		try {
			const result = await languageTestHubAdminApi.approve(item.id);
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
		generateVisaCategory = '';
		showGenerateModal = true;
	}

	async function confirmGenerate() {
		if (!generateTopic.trim()) return;
		generating = true;
		pageError = '';
		try {
			const result = await languageTestHubAdminApi.generate({
				topic: generateTopic.trim(),
				dest_country: generateCountry.trim() || undefined,
				visa_category: generateVisaCategory.trim() || undefined
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
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: TestItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.approve', 'Approve'),
				icon: CheckCircle,
				onclick: approveTest,
				show: () => true
			});
			acts.push({
				label: t('common.generate', 'Generate'),
				icon: Sparkles,
				onclick: openGenerate,
				show: () => true
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'test_name', label: t('common.test_name', 'Test Name'), type: 'text' },
		{ key: 'dest_country', label: t('common.country', 'Country'), type: 'text' },
		{ key: 'visa_category', label: t('common.visa_category', 'Visa Category'), type: 'text' },
		{ key: 'content', label: t('common.content', 'Content'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.language_test_hub_heading', 'Language Test Hub Review Queue'),
		description: t('app.tools.language_test_hub_desc', 'Review and approve language test requirement entries'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'test_name', label: t('common.test_name', 'Test Name'), sortable: true },
			{ key: 'dest_country', label: t('common.country', 'Country'), sortable: true },
			{ key: 'visa_category', label: t('common.visa_category', 'Visa Category'), sortable: true },
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
			const result = await languageTestHubAdminApi.list();
			if (result.ok) {
				const data = (result.data as TestItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: TestView
	});
</script>

<svelte:head>
	<title>{t('app.tools.language_test_hub_page_title', 'Language Test Hub Review Queue — JapaConnect Admin')}</title>
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
<Modal bind:open={showGenerateModal} title={t('app.tools.generate_language_test', 'Generate Language Test Requirement')}>
	<div class="space-y-4">
		<div>
			<label for="gen-topic" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.topic_prompt', 'Topic / Prompt')}</label>
			<textarea
				id="gen-topic"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="4"
				bind:value={generateTopic}
				placeholder={t('app.tools.topic_placeholder', 'Describe the language test requirement to generate...')}
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
			<label for="gen-visa" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('common.visa_category', 'Visa Category')}</label>
			<input
				id="gen-visa"
				type="text"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={generateVisaCategory}
				placeholder={t('common.visa_category_placeholder', 'e.g. Skilled Worker')}
			/>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showGenerateModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmGenerate} disabled={generating || !generateTopic.trim()}>
			{generating ? t('common.generating', 'Generating...') : t('common.generate', 'Generate')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet TestView({ row }: { row: TestItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.test_name', 'Test Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.test_name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.country', 'Country')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.dest_country || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.visa_category', 'Visa Category')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.visa_category || '—'}</p>
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
