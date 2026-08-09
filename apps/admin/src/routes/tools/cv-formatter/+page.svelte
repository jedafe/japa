<script lang="ts">
	// /tools/cv-formatter — admin management for CV formatting standards (§95).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Save } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { cvFormatterAdminApi } from '$lib/api';

	type CVStandardItem = {
		id: string;
		dest_country: string;
		format_name: string;
		template?: string;
		guidelines?: string;
		language?: string;
		max_pages?: number;
		updated_at?: string;
	};

	const canManage = $derived(hasPermission('cv_formatter.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	// ── Upsert modal state ──────────────────────────────────────────────────
	let showUpsertModal = $state(false);
	let upsertDestCountry = $state('');
	let upsertFormatName = $state('');
	let upsertTemplate = $state('');
	let upsertGuidelines = $state('');
	let upsertLanguage = $state('');
	let upsertMaxPages = $state('');

	function openUpsert() {
		upsertDestCountry = '';
		upsertFormatName = '';
		upsertTemplate = '';
		upsertGuidelines = '';
		upsertLanguage = '';
		upsertMaxPages = '';
		showUpsertModal = true;
	}

	async function confirmUpsert() {
		acting = true;
		pageError = '';
		try {
			const body: Record<string, unknown> = {
				dest_country: upsertDestCountry,
				format_name: upsertFormatName,
				template: upsertTemplate,
				guidelines: upsertGuidelines,
				language: upsertLanguage
			};
			if (upsertMaxPages) body.max_pages = parseInt(upsertMaxPages, 10);
			const result = await cvFormatterAdminApi.upsertStandard(body);
			if (result.ok) {
				showUpsertModal = false;
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
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: CVStandardItem, ctx) => ctx?.openView(_row) }
		];
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is never shown.
	const fields: FieldDef[] = [
		{ key: 'dest_country', label: t('app.tools.col_dest_country', 'Destination Country'), type: 'text' },
		{ key: 'format_name', label: t('app.tools.col_format_name', 'Format Name'), type: 'text' },
		{ key: 'template', label: t('app.tools.col_template', 'Template'), type: 'textarea' },
		{ key: 'guidelines', label: t('app.tools.col_guidelines', 'Guidelines'), type: 'textarea' },
		{ key: 'language', label: t('app.tools.col_language', 'Language'), type: 'text' },
		{ key: 'max_pages', label: t('app.tools.col_max_pages', 'Max Pages'), type: 'number' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.cv_formatter_heading', 'CV Formatter Standards'),
		description: t('app.tools.cv_formatter_desc', 'Manage country-specific CV formatting standards'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'dest_country', label: t('app.tools.col_dest_country', 'Destination'), sortable: true },
			{ key: 'format_name', label: t('app.tools.col_format_name', 'Format'), sortable: true },
			{
				key: 'language',
				label: t('app.tools.col_language', 'Language'),
				render: (value: string) => value || '—'
			},
			{
				key: 'max_pages',
				label: t('app.tools.col_max_pages', 'Max Pages'),
				render: (value: number) => (value ? String(value) : '—')
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
			const result = await cvFormatterAdminApi.listStandards();
			if (result.ok) {
				const data = (result.data as CVStandardItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: CVStandardView
	});
</script>

<svelte:head>
	<title>{t('app.tools.cv_formatter_page_title', 'CV Formatter Standards — JapaConnect Admin')}</title>
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
		<Button onclick={openUpsert}>
			<Save size={16} />
			{t('app.tools.upsert_standard', 'Upsert Standard')}
		</Button>
	{/if}
</div>

<EntityPage {config} bind:controller={entityController} />

<!-- ── Upsert modal ──────────────────────────────────────────────────────── -->
<Modal bind:open={showUpsertModal} title={t('app.tools.upsert_cv_standard', 'Upsert CV Standard')}>
	<div class="space-y-4">
		<div>
			<label for="upsert-dest" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_dest_country', 'Destination Country')}</label>
			<input
				id="upsert-dest"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={upsertDestCountry}
				placeholder={t('app.tools.dest_country_placeholder', 'e.g. Japan')}
			/>
		</div>
		<div>
			<label for="upsert-format" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_format_name', 'Format Name')}</label>
			<input
				id="upsert-format"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={upsertFormatName}
				placeholder={t('app.tools.format_name_placeholder', 'e.g. Japanese Standard CV (rirekisho)')}
			/>
		</div>
		<div>
			<label for="upsert-lang" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_language', 'Language')}</label>
			<input
				id="upsert-lang"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={upsertLanguage}
				placeholder={t('app.tools.language_placeholder', 'e.g. Japanese')}
			/>
		</div>
		<div>
			<label for="upsert-pages" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_max_pages', 'Max Pages')}</label>
			<input
				id="upsert-pages"
				type="number"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={upsertMaxPages}
				placeholder={t('app.tools.max_pages_placeholder', 'e.g. 2')}
			/>
		</div>
		<div>
			<label for="upsert-template" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_template', 'Template')}</label>
			<textarea
				id="upsert-template"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="4"
				bind:value={upsertTemplate}
				placeholder={t('app.tools.template_placeholder', 'Template content or structure...')}
			></textarea>
		</div>
		<div>
			<label for="upsert-guidelines" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_guidelines', 'Guidelines')}</label>
			<textarea
				id="upsert-guidelines"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="3"
				bind:value={upsertGuidelines}
				placeholder={t('app.tools.guidelines_placeholder', 'Formatting guidelines...')}
			></textarea>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showUpsertModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmUpsert} disabled={acting}>
			{acting ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet CVStandardView({ row }: { row: CVStandardItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_dest_country', 'Destination Country')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.dest_country}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_format_name', 'Format Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.format_name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_language', 'Language')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.language || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_max_pages', 'Max Pages')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.max_pages ? String(row.max_pages) : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_template', 'Template')}</p>
			<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.template || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_guidelines', 'Guidelines')}</p>
			<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.guidelines || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_updated_at', 'Updated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
