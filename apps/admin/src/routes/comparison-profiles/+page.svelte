<script lang="ts">
	// /comparison-profiles — admin management for comparison profile templates and user profiles.
	// Uses the EntityPage scaffold with a custom formSnippet because the JSON fields
	// (goals, preferences, demographics) cannot be edited by the basic EntityForm.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { jceAdminApi, type ComparisonProfileAdmin } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { parseJson, JsonParseError } from '$lib/utils/json';
	import { Trash2 } from 'lucide-svelte';
	import type { BulkAction } from '$lib/components/ui/data-table';
import ProfileFormFields from '$lib/components/jce/ProfileFormFields.svelte';

	type Profile = ComparisonProfileAdmin;

	// ── Helpers for JSON + list textareas ───────────────────────────────────
	function listToString(raw: unknown): string {
		if (raw == null || raw === '') return '';
		if (Array.isArray(raw)) return raw.map(String).join(', ');
		if (typeof raw === 'string') return raw;
		return JSON.stringify(raw);
	}

	function parseStringList(raw: unknown): string[] {
		if (Array.isArray(raw)) return raw.map(String).map((s) => s.trim()).filter(Boolean);
		if (typeof raw === 'string' && raw.trim()) {
			return raw.split(',').map((s) => s.trim()).filter(Boolean);
		}
		return [];
	}

	function sourceSpace(raw: unknown): string | null {
		if (typeof raw !== 'string') return null;
		const s = raw.trim();
		return s || null;
	}

	function applySort<T extends Record<string, any>>(data: T[], sort: string): T[] {
		if (!sort) return data;
		const [key, dir] = sort.split(':') as [string, 'asc' | 'desc'];
		if (!key) return data;
		const sign = dir === 'desc' ? -1 : 1;
		return [...data].sort((a, b) => {
			const av = a[key];
			const bv = b[key];
			if (av == null && bv == null) return 0;
			if (av == null) return 1 * sign;
			if (bv == null) return -1 * sign;
			if (typeof av === 'string' && typeof bv === 'string') return av.localeCompare(bv) * sign;
			if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * sign;
			if (typeof av === 'boolean' && typeof bv === 'boolean') return (Number(av) - Number(bv)) * sign;
			return String(av).localeCompare(String(bv)) * sign;
		});
	}

	function tryNormalizeBody(
		value: Record<string, any>
	): { ok: true; body: Record<string, any> } | { ok: false; error: string } {
		try {
			return {
				ok: true,
				body: {
					title: String(value.title ?? '').trim(),
					source_space_id: sourceSpace(value.source_space_id),
					target_space_ids: parseStringList(value.target_space_ids),
					goals: parseJson(value.goals, t('comparison_profiles.goals', 'Goals'), [] as { key: string; weight: number }[]),
					preferences: parseJson(value.preferences, t('comparison_profiles.preferences', 'Preferences'), {} as Record<string, number>),
					demographics: parseJson(value.demographics, t('comparison_profiles.demographics', 'Demographics'), {} as Record<string, any>),
					is_default: !!value.is_default,
					is_template: !!value.is_template
				}
			};
		} catch (e) {
			if (e instanceof JsonParseError) return { ok: false, error: e.message };
			return { ok: false, error: e instanceof Error ? e.message : t('common.error', 'An error occurred') };
		}
	}

	// ── Permissions ─────────────────────────────────────────────────────────
	const canCreate = $derived(hasPermission('jce.profile.create'));
	const canEdit = $derived(hasPermission('jce.profile.update'));
	const canDelete = $derived(hasPermission('jce.profile.delete'));
	const canCreateTemplate = $derived(hasPermission('comparison.edit_any'));
	// Rule 11 amendment: permanent delete requires the dedicated node.
	const canHardDelete = $derived(hasPermission('admin.hard_delete'));

	let profileStats = $state<{ source_space_id: string; profile_count: number }[]>([]);
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	// Row map so bulk actions can resolve ids → rows after EntityPage clears
	// the selection.
	const profileRowMap = new Map<string, Profile>();
	const profileRowsFor = (ids: Set<string>): Profile[] =>
		Array.from(ids)
			.map((id) => profileRowMap.get(id))
			.filter((r): r is Profile => !!r);

	// ── Force delete (admin.hard_delete) — bulk shares one confirm modal ──
	let forceTargets = $state<Profile[]>([]);
	let showForceModal = $state(false);

	function openBulkForceDelete(ids: Set<string>) {
		const rows = profileRowsFor(ids).filter((r) => !!r.deleted_at);
		if (rows.length === 0) return;
		forceTargets = rows;
		showForceModal = true;
	}

	async function confirmForceDelete() {
		if (bulkDeleting || forceTargets.length === 0) return;
		bulkDeleting = true;
		pageError = '';
		try {
			const targets = forceTargets;
			let failed = 0;
			let lastErr = '';
			for (const row of targets) {
				const res = await jceAdminApi.hardDeleteProfile(row.id);
				if (!res.ok) {
					failed++;
					lastErr = res.error ?? '';
				}
			}
			showForceModal = false;
			forceTargets = [];
			if (failed > 0) {
				pageError =
					t('common.bulk_partial', '{failed} of {n} items could not be completed')
						.replace('{failed}', String(failed))
						.replace('{n}', String(targets.length)) + (lastErr ? ` — ${lastErr}` : '');
			}
			entityController?.reload();
		} finally {
			bulkDeleting = false;
		}
	}

	// `fields` is required by EntityPage to seed the edit form from a row.
	const fields: FieldDef[] = [
		{ key: 'title', label: t('comparison_profiles.name', 'Title'), type: 'text', required: true },
		{ key: 'source_space_id', label: t('comparison_profiles.source_space', 'Source country'), type: 'text' },
		{ key: 'target_space_ids', label: t('comparison_profiles.target_spaces', 'Target spaces'), type: 'text' },
		{ key: 'goals', label: t('comparison_profiles.goals', 'Goals'), type: 'textarea' },
		{ key: 'preferences', label: t('comparison_profiles.preferences', 'Preferences'), type: 'textarea' },
		{ key: 'demographics', label: t('comparison_profiles.demographics', 'Demographics'), type: 'textarea' },
		{ key: 'is_default', label: t('comparison_profiles.is_default', 'Default'), type: 'checkbox' },
		{ key: 'is_template', label: t('comparison_profiles.is_template', 'Template'), type: 'checkbox' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('comparison_profiles.title', 'Comparison Profiles'),
		description: t('comparison_profiles.description', 'Manage comparison profile templates and user profiles'),
		createLabel: t('comparison_profiles.add', 'Add Comparison Profile'),
		createPermission: 'jce.profile.create',
		editPermission: 'jce.profile.update',
		deletePermission: 'jce.profile.delete',
		hardDeletePermission: 'admin.hard_delete',
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('comparison_profiles.name', 'Title'), sortable: true },
			{
				key: 'source_space_id',
				label: t('comparison_profiles.source_space', 'Source country'),
				render: (v: string | null) => `<span class="text-xs" style="color: var(--color-muted)">${v || '—'}</span>`
			},
			{
				key: 'target_space_ids',
				label: t('comparison_profiles.target_spaces', 'Target spaces'),
				render: (v: unknown) => `<span class="text-xs" style="color: var(--color-muted)">${Array.isArray(v) ? v.length : 0} ${t('common.items', 'items')}</span>`
			},
			{
				key: 'is_template',
				label: t('comparison_profiles.is_template', 'Template'),
				render: (v: boolean) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v ? 'bg-success/15 text-success' : 'bg-muted/15 text-muted'}">${v ? t('common.yes', 'Yes') : t('common.no', 'No')}</span>`
			},
			{
				key: 'is_default',
				label: t('comparison_profiles.is_default', 'Default'),
				render: (v: boolean) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v ? 'bg-primary/15 text-primary' : 'bg-muted/15 text-muted'}">${v ? t('common.yes', 'Yes') : t('common.no', 'No')}</span>`
			},
			{
				key: 'deleted_at',
				label: t('common.filter_deleted', 'Deleted'),
				render: (v: string | null) =>
					v
						? `<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-danger/15 text-danger">${t('common.filter_deleted', 'Deleted')}</span>`
						: `<span style="color: var(--color-muted)">—</span>`
			}
		],
		filters: [
			{
				key: 'is_template',
				label: t('comparison_profiles.is_template', 'Template'),
				options: [
					{ value: 'true', label: t('common.yes', 'Yes') },
					{ value: 'false', label: t('common.no', 'No') }
				]
			},
			// Soft-deleted profiles are hidden by default — this filter surfaces
			// them so the Hard Delete action has targets.
			{
				key: 'deleted',
				label: t('common.filter_deleted', 'Deleted'),
				options: [
					{ value: 'only', label: t('common.filter_deleted_only', 'Deleted only') },
					{ value: 'all', label: t('common.filter_deleted_all', 'Include deleted') }
				]
			}
		],
		bulkActions: canHardDelete
			? [
					{
						label: t('common.force_delete', 'Force Delete Selected'),
						icon: Trash2,
						variant: 'destructive',
						onclick: openBulkForceDelete,
						disabled: (ids) => bulkDeleting || !profileRowsFor(ids).some((r) => !!r.deleted_at)
					}
				]
			: undefined,
		fields,
		isDeleted: (row: Profile) => !!row.deleted_at,
		load: async ({ page, pageSize, q, sort, filter }) => {
			// The backend returns every profile without pagination, so fetch a large
			// window and apply client-side search/filter/pagination/sort.
			const [result, statsResult] = await Promise.all([
				jceAdminApi.listProfiles(filter?.deleted || undefined),
				jceAdminApi.getProfileStats()
			]);
			if (statsResult.ok) {
				profileStats = (statsResult.data as typeof profileStats) ?? [];
			}
			if (result.ok) {
				let data = (result.data as Profile[]) ?? [];
				// The platform list always appends live templates — re-apply the
				// deleted view client-side so it can't leak into 'only'.
				if (filter?.deleted === 'only') {
					data = data.filter((p) => !!p.deleted_at);
				} else if (!filter?.deleted) {
					data = data.filter((p) => !p.deleted_at);
				}
				const query = (q ?? '').trim().toLowerCase();
				if (query) {
					data = data.filter((p) =>
						[p.title, p.source_space_id].some((v) =>
							String(v ?? '').toLowerCase().includes(query)
						)
					);
				}
				// Default to template-only in the live view; in a deleted view show
				// everything unless is_template was explicitly chosen.
				if (filter?.deleted) {
					if (filter?.is_template === 'true') data = data.filter((p) => p.is_template);
					else if (filter?.is_template === 'false') data = data.filter((p) => !p.is_template);
				} else {
					const showTemplates = filter?.is_template !== 'false';
					data = data.filter((p) => p.is_template === showTemplates);
				}
				data = applySort(data, sort);
				const size = pageSize || 25;
				const offset = (page || 0) * size;
				const total = data.length;
				const totalPages = Math.max(1, Math.ceil(total / size));
				const pageRows = data.slice(offset, offset + size);
				for (const r of pageRows) profileRowMap.set(r.id, r);
				return { data: pageRows, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load comparison profiles'));
		},
		create: async (body) => {
			if (!canCreate) return { ok: false, error: t('common.permission_denied', 'Permission denied') };
			const normalized = tryNormalizeBody(body);
			if (!normalized.ok) return { ok: false, error: normalized.error };
			if (!normalized.body.title) {
				return { ok: false, error: t('comparison_profiles.title_required', 'Title is required') };
			}
			if (normalized.body.is_template && !canCreateTemplate) {
				return { ok: false, error: t('common.permission_denied', 'Permission denied') };
			}
			const res = await jceAdminApi.createProfile(normalized.body);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			if (!canEdit) return { ok: false, error: t('common.permission_denied', 'Permission denied') };
			const normalized = tryNormalizeBody(body);
			if (!normalized.ok) return { ok: false, error: normalized.error };
			if (!normalized.body.title) {
				return { ok: false, error: t('comparison_profiles.title_required', 'Title is required') };
			}
			const res = await jceAdminApi.updateProfile(id, normalized.body);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			if (!canDelete) return { ok: false, error: t('common.permission_denied', 'Permission denied') };
			const res = await jceAdminApi.deleteProfile(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		hardRemove: async (id) => {
			if (!canHardDelete) return { ok: false, error: t('common.permission_denied', 'Permission denied') };
			const res = await jceAdminApi.hardDeleteProfile(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: ProfileView,
		formSnippet: ProfileForm
	});
</script>

<svelte:head>
	<title>{t('comparison_profiles.title', 'Comparison Profiles')} — {t('common.admin_title_suffix', 'JapaConnect Admin')}</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

{#if profileStats.length > 0}
	<div class="mb-4 rounded-[var(--radius-card)] border p-4 text-sm" style="background-color: var(--color-surface); border-color: var(--color-border)">
		<p class="font-medium" style="color: var(--color-text)">
			{t('comparison_profiles.anon_stats', 'Anonymized user profile usage')}
		</p>
		<div class="mt-2 flex flex-wrap gap-3">
			{#each profileStats as stat (stat.source_space_id)}
				<span class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs" style="border-color: var(--color-border); background-color: var(--color-bg); color: var(--color-text)">
					<span style="color: var(--color-muted)">{stat.source_space_id}:</span>
					<strong>{stat.profile_count}</strong>
					{t('comparison_profiles.profiles', 'profiles')}
				</span>
			{/each}
		</div>
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── Force Delete confirmation (bulk) — permanent erasure of soft-deleted rows ── -->
<Modal bind:open={showForceModal} title={t('common.force_delete', 'Force Delete')} size="sm">
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{#if forceTargets.length === 1}
				{t('common.force_delete_confirm', 'Permanently delete "{name}"? This cannot be undone.').replace('{name}', forceTargets[0].title)}
			{:else}
				{t('common.force_delete_confirm_bulk', 'Permanently delete {count} selected items? This cannot be undone.').replace('{count}', String(forceTargets.length))}
			{/if}
		</p>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => { showForceModal = false; forceTargets = []; }}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmForceDelete} disabled={bulkDeleting}>
			{bulkDeleting ? t('common.deleting', 'Deleting...') : t('common.force_delete', 'Force Delete')}
		</Button>
	</div>
</Modal>

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet ProfileView({ row }: { row: Profile })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('comparison_profiles.name', 'Title')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparison_profiles.source_space', 'Source country')}
				</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.source_space_id || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparison_profiles.target_spaces', 'Target spaces')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{Array.isArray(row.target_space_ids) ? row.target_space_ids.length : 0} {t('common.items', 'items')}
				</p>
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparison_profiles.is_template', 'Template')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{row.is_template ? t('common.yes', 'Yes') : t('common.no', 'No')}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparison_profiles.is_default', 'Default')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{row.is_default ? t('common.yes', 'Yes') : t('common.no', 'No')}
				</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('comparison_profiles.goals', 'Goals')}
			</p>
			<pre class="mt-1 max-h-48 overflow-auto rounded-md border p-2 text-xs font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">{JSON.stringify(row.goals, null, 2)}</pre>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('comparison_profiles.preferences', 'Preferences')}
			</p>
			<pre class="mt-1 max-h-48 overflow-auto rounded-md border p-2 text-xs font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">{JSON.stringify(row.preferences, null, 2)}</pre>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('comparison_profiles.demographics', 'Demographics')}
			</p>
			<pre class="mt-1 max-h-48 overflow-auto rounded-md border p-2 text-xs font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">{JSON.stringify(row.demographics, null, 2)}</pre>
		</div>
	</div>
{/snippet}

{#snippet ProfileForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
	<div class="space-y-4">
		<div>
			<label for="profile-title" class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t('comparison_profiles.name', 'Title')}
				<span style="color: var(--color-danger)">*</span>
			</label>
			<input
				id="profile-title"
				type="text"
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={value.title}
			/>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="profile-source" class="block text-sm font-medium" style="color: var(--color-foreground)">
					{t('comparison_profiles.source_space', 'Source country')}
				</label>
				<input
					id="profile-source"
					type="text"
					class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					bind:value={value.source_space_id}
				/>
			</div>
			<div>
				<label for="profile-targets" class="block text-sm font-medium" style="color: var(--color-foreground)">
					{t('comparison_profiles.target_spaces', 'Target spaces')}
				</label>
				<textarea
					id="profile-targets"
					rows="2"
					class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					value={listToString(value.target_space_ids)}
					onchange={(e) => { value.target_space_ids = e.currentTarget.value; }}
				></textarea>
				<p class="mt-1 text-xs" style="color: var(--color-muted)">
					{t('comparison_profiles.target_spaces_hint', 'Comma-separated space IDs')}
				</p>
			</div>
		</div>
		<ProfileFormFields {value} {editing} />
	</div>
{/snippet}
