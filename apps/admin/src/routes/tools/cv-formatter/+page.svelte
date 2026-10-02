<script lang="ts">
	// /tools/cv-formatter — admin management for CV formatting standards (§95).
	// Ref: japaconnect-engineering/sections/95-cv-formatter.md
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Save } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { cvFormatterAdminApi } from '$lib/api';
	import { useCountries } from '$lib/utils/countries.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	// CVStandardRow matches the backend StandardDTO (§95, ADR-103).
	type CVStandardRow = {
		id: string;
		destination_space_id: string;
		destination_space_name?: string;
		destination_space_code?: string;
		recommended_length_pages?: string;
		photo_policy?: string;
		date_format?: string;
		recommended_sections?: string[];
		tone_guidance?: string;
		common_pitfalls?: string;
		created_at: string;
		updated_at: string;
	};

	const canManage = $derived(hasPermission('cv_formatter.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	
	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	// ── Upsert modal state ──────────────────────────────────────────────────
	let showUpsertModal = $state(false);
	let upsertDestSpaceId = $state('');
	let upsertLengthPages = $state('');
	let upsertPhotoPolicy = $state('');
	let upsertDateFormat = $state('');
	let upsertSections = $state('');
	let upsertToneGuidance = $state('');
	let upsertCommonPitfalls = $state('');

	function openUpsert() {
		upsertDestSpaceId = '';
		upsertLengthPages = '';
		upsertPhotoPolicy = '';
		upsertDateFormat = '';
		upsertSections = '';
		upsertToneGuidance = '';
		upsertCommonPitfalls = '';
		showUpsertModal = true;
	}

	async function confirmUpsert() {
		acting = true;
		pageError = '';
		try {
			const body: Record<string, unknown> = {
				destination_space_id: upsertDestSpaceId,
				recommended_length_pages: upsertLengthPages || undefined,
				photo_policy: upsertPhotoPolicy || undefined,
				date_format: upsertDateFormat || undefined,
				recommended_sections: upsertSections
					? upsertSections.split(',').map((s) => s.trim()).filter(Boolean)
					: undefined,
				tone_guidance: upsertToneGuidance || undefined,
				common_pitfalls: upsertCommonPitfalls || undefined
			};
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
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: CVStandardRow, ctx) => ctx?.openView(_row) }
		];
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is never shown.
	const fields: FieldDef[] = [
		{ key: 'destination_space_id', label: t('app.tools.col_destination', 'Destination'), type: 'text' },
		{ key: 'recommended_length_pages', label: t('app.tools.col_length_pages', 'Recommended Length'), type: 'text' },
		{ key: 'photo_policy', label: t('app.tools.col_photo_policy', 'Photo Policy'), type: 'text' },
		{ key: 'date_format', label: t('app.tools.col_date_format', 'Date Format'), type: 'text' },
		{ key: 'tone_guidance', label: t('app.tools.col_tone_guidance', 'Tone Guidance'), type: 'textarea' },
		{ key: 'common_pitfalls', label: t('app.tools.col_common_pitfalls', 'Common Pitfalls'), type: 'textarea' }
	];

		const filters = $derived.by<FilterDef[]>(() => {
		countryUtils.ready;
		return [
			{
				key: 'dest',
				label: t('app.tools.col_destination', 'Destination'),
				type: 'autocomplete',
				placeholder: t('app.tools.country_placeholder', 'Search country…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: countryUtils.fetcher,
				initialFetcher: countryUtils.initialFetcher,
				getSelectedLabel: countryUtils.getSelectedLabel
			}
		];
	});

	const config = $derived<EntityPageConfig>({
		filters,
		title: t('app.tools.cv_formatter_heading', 'CV Formatter Standards'),
		description: t('app.tools.cv_formatter_desc', 'Manage country-specific CV formatting standards'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'destination_space_name',
				label: t('app.tools.col_destination', 'Destination'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'recommended_length_pages',
				label: t('app.tools.col_length_pages', 'Length'),
				render: (value: string) => value || '—'
			},
			{
				key: 'photo_policy',
				label: t('app.tools.col_photo_policy', 'Photo Policy'),
				render: (value: string) => value || '—'
			},
			{
				key: 'date_format',
				label: t('app.tools.col_date_format', 'Date Format'),
				render: (value: string) => value || '—'
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
			qs.set('page', String((params.page ?? 0) + 1));
			qs.set('limit', String(params.pageSize ?? 25));
			const queryString = qs.toString();
			const result = await cvFormatterAdminApi.listStandardsRaw(`?${queryString}`);
			if (result.ok) {
				const data = (result.data.data as CVStandardRow[]) ?? [];
				return {
					data,
					total: result.data.pagination?.total ?? data.length,
					totalPages: result.data.pagination?.total_pages ?? 1
				};
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
			<label for="upsert-dest" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_destination', 'Destination')}</label>
			<Autocomplete
				id="upsert-dest"
				placeholder={t('app.tools.country_placeholder', 'Search country…')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={countryUtils.fetcher}
				initialFetcher={countryUtils.initialFetcher}
				selectedId={upsertDestSpaceId || null}
				selectedLabel={upsertDestSpaceId ? countryUtils.getSelectedLabel(upsertDestSpaceId) : null}
				onSelect={(item: AutocompleteItem) => { upsertDestSpaceId = item.id; }}
				onClear={() => { upsertDestSpaceId = ''; }}
			/>
		</div>
		<div>
			<label for="upsert-pages" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_length_pages', 'Recommended Length (pages)')}</label>
			<input
				id="upsert-pages"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={upsertLengthPages}
				placeholder={t('app.tools.length_pages_placeholder', 'e.g. 1-2')}
			/>
		</div>
		<div>
			<label for="upsert-photo" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_photo_policy', 'Photo Policy')}</label>
			<AutocompleteSelect
				id="upsert-photo"
				bind:value={upsertPhotoPolicy}
				options={[{ value: '', label: t('common.none', 'None') }, { value: 'required', label: t('app.tools.photo_required', 'Required') }, { value: 'optional', label: t('app.tools.photo_optional', 'Optional') }, { value: 'not_recommended', label: t('app.tools.photo_not_recommended', 'Not Recommended') }, { value: 'prohibited', label: t('app.tools.photo_prohibited', 'Prohibited') }]}
				class="w-full"
			/>
		</div>
		<div>
			<label for="upsert-date" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_date_format', 'Date Format')}</label>
			<input
				id="upsert-date"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={upsertDateFormat}
				placeholder={t('app.tools.date_format_placeholder', 'e.g. YYYY-MM-DD')}
			/>
		</div>
		<div>
			<label for="upsert-sections" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_sections', 'Recommended Sections')}</label>
			<input
				id="upsert-sections"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={upsertSections}
				placeholder={t('app.tools.sections_placeholder', 'Comma-separated, e.g. Personal Info, Education, Work Experience')}
			/>
		</div>
		<div>
			<label for="upsert-tone" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_tone_guidance', 'Tone Guidance')}</label>
			<textarea
				id="upsert-tone"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="3"
				bind:value={upsertToneGuidance}
				placeholder={t('app.tools.tone_guidance_placeholder', 'e.g. Formal and concise')}
			></textarea>
		</div>
		<div>
			<label for="upsert-pitfalls" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_common_pitfalls', 'Common Pitfalls')}</label>
			<textarea
				id="upsert-pitfalls"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="3"
				bind:value={upsertCommonPitfalls}
				placeholder={t('app.tools.common_pitfalls_placeholder', 'e.g. Avoid including personal details like age or marital status')}
			></textarea>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showUpsertModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmUpsert} disabled={acting || !upsertDestSpaceId}>
			{acting ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet CVStandardView({ row }: { row: CVStandardRow })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_destination', 'Destination')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.destination_space_name ?? row.destination_space_id}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_length_pages', 'Recommended Length')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.recommended_length_pages ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_photo_policy', 'Photo Policy')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.photo_policy ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_date_format', 'Date Format')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.date_format ?? '—'}</p>
		</div>
		{#if row.recommended_sections && row.recommended_sections.length > 0}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_sections', 'Recommended Sections')}</p>
				<div class="mt-1 flex flex-wrap gap-2">
					{#each row.recommended_sections as section (section)}
						<span class="px-2.5 py-1 text-xs rounded-full" style="background: var(--color-surface-subtle); color: var(--color-foreground)">{section}</span>
					{/each}
				</div>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_tone_guidance', 'Tone Guidance')}</p>
			<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.tone_guidance ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_common_pitfalls', 'Common Pitfalls')}</p>
			<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.common_pitfalls ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_updated_at', 'Updated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
