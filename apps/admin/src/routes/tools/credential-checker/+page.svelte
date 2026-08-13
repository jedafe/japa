<script lang="ts">
	// /tools/credential-checker — admin HITL review queue for AI-generated credential checker guides.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, Sparkles } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { credentialCheckerAdminApi } from '$lib/api';

	type CredentialCheckerItem = {
		id: string;
		profession: string;
		dest_country: string;
		issuing_country: string;
		status: string;
		ai_generated: boolean;
		updated_at?: string;
		summary?: string;
		requirements?: string;
	};

	const canManage = $derived(hasPermission('credential_checker.manage'));

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
			const result = await credentialCheckerAdminApi.generate({
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

	async function approveItem(item: CredentialCheckerItem) {
		acting = true;
		pageError = '';
		try {
			const result = await credentialCheckerAdminApi.approve(item.id);
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
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: CredentialCheckerItem, ctx) => ctx?.openView(_row) }
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
		{ key: 'profession', label: t('app.tools.col_profession', 'Profession'), type: 'text' },
		{ key: 'dest_country', label: t('app.tools.col_dest_country', 'Destination Country'), type: 'text' },
		{ key: 'issuing_country', label: t('app.tools.col_issuing_country', 'Issuing Country'), type: 'text' },
		{ key: 'summary', label: t('app.tools.col_summary', 'Summary'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.credential_checker_heading', 'Credential Checker Review Queue'),
		description: t('app.tools.credential_checker_desc', 'Review and approve AI-generated credential checker guides'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'profession', label: t('app.tools.col_profession', 'Profession'), sortable: true },
			{ key: 'dest_country', label: t('app.tools.col_dest_country', 'Destination'), sortable: true },
			{ key: 'issuing_country', label: t('app.tools.col_issuing_country', 'Issuing Country'), sortable: true },
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
		load: async (params) => {
			const qs = new URLSearchParams();
			if (params.filter?.dest) qs.set('dest', params.filter.dest);
			const queryString = qs.toString();
			const result = await credentialCheckerAdminApi.list(queryString ? `?${queryString}` : '');
			if (result.ok) {
				const data = (result.data as CredentialCheckerItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: CredentialCheckerView
	});
</script>

<svelte:head>
	<title>{t('app.tools.credential_checker_page_title', 'Credential Checker Review Queue — JapaConnect Admin')}</title>
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
<Modal bind:open={showGenerateModal} title={t('app.tools.generate_credential_guide', 'Generate Credential Guide')}>
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
				placeholder={t('app.tools.topic_placeholder', 'Describe the profession and credentials to cover...')}
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
{#snippet CredentialCheckerView({ row }: { row: CredentialCheckerItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_profession', 'Profession')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.profession}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_dest_country', 'Destination Country')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.dest_country || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_issuing_country', 'Issuing Country')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.issuing_country || '—'}</p>
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
