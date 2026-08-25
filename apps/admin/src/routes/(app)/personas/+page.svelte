<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { Bot, Plus, Trash2, Activity, Pencil, Pause, Play, Eye } from 'lucide-svelte';
	import { api, apiRaw } from '$lib/api';
	import type { BulkAction } from '$lib/components/ui/data-table';

	// Persona type matches the backend PersonaDetail / PersonaListItem DTOs
	// (internal/personas/service.go). Ref: §68, ADR-068.
	type Persona = {
		id: string;
		user_id: string;
		profile_id: string;
		disclosure_mode: string; // 'disclosed' | 'undisclosed' (§68 schema CHECK)
		autonomy_mode: string;   // 'queued' | 'autonomous' (§68 schema CHECK)
		voice_id: string;
		voice_name?: string | null;
		voice_prompt: string | null;
		voice_prompt_override?: string | null;
		activity_policy: Record<string, unknown>;
		status: string;          // 'active' | 'paused' | 'retired' (§68 schema CHECK)
		created_at: string;
		updated_at: string;
		backing_username?: string;
		profile_name?: string;
		total_actions?: number;
	};

	// PersonaInput matches the backend PersonaInput struct. The backend
	// requires name and slug (service.go validates both). type_id is optional
	// — when omitted the backend falls back to 'contributor'/'columnist', but
	// tenants that lack those types must pass an explicit type_id (BUG-007).
	// Ref: §68.
	type PersonaInput = {
		name: string;
		slug: string;
		tagline?: string;
		voice_id?: string;
		voice_prompt?: string;
		disclosure_mode: string;
		autonomy_mode: string;
		activity_policy?: ActivityPolicy;
		type_id?: string;
	};

	// ProfileTypeOption — minimal shape from /profiles/types (Rule 16: no
	// hardcoded option lists; the selector is populated from the API).
	type ProfileTypeOption = {
		id: string;
		name: string;
		slug: string;
	};

	type PersonaVoice = {
		id: string;
		name: string;
		slug: string;
		description?: string;
		voice_prompt: string;
		is_system: boolean;
	};

	// ActivityEntry matches the backend ActivityEntry struct
	// (internal/personas/service.go). Ref: §68.
	type ActivityEntry = {
		id: number;
		event_type: string;
		entity_type?: string;
		entity_id?: string;
		meta: Record<string, unknown>;
		is_synthetic: boolean;
		created_at: string;
	};

	// ActivityPolicy matches the backend ActivityPolicy struct (§68).
	type ActivityPolicy = {
		actions_per_day: number;
		action_budget: Record<string, number>;
		eligible_content_types: string[];
		eligible_spaces: string[];
		min_content_age_hours: number;
		avoid_own_content: boolean;
		avoid_recent_authors: string[];
		active_hours_utc: [number, number];
	};

	// UpdateInput matches the backend UpdatePersonaInput. Status is included
	// so admins can pause/retire personas from the UI.
	type UpdateInput = {
		disclosure_mode?: string;
		autonomy_mode?: string;
		voice_id?: string;
		voice_prompt?: string;
		status?: string;
		activity_policy?: ActivityPolicy;
	};

	const canManage = $derived(hasPermission('persona.manage'));
	const canView = $derived(hasPermission('persona.view'));
	// The activity log endpoint is gated by participation.view (ADR-077),
	// so only show the action to users who hold that node.
	const canViewActivity = $derived(hasPermission('participation.view'));

	let personas = $state<Persona[]>([]);
	let loading = $state(true);
	let error = $state('');
	let createSlideOverOpen = $state(false);
	let activitySlideOverOpen = $state(false);
	let activityLog = $state<ActivityEntry[]>([]);
	let activityLoading = $state(false);
	let activityTotal = $state(0);
	let activityPage = $state(0);
	let activityPageSize = $state(10);
	let activityPersonaId = $state('');
	let saving = $state(false);
	let message = $state('');
	let messageType = $state('');

	// Profile types for the create-form selector (BUG-007: without a
	// type_id selector, provisioning fails on tenants that lack the
	// 'contributor'/'columnist' default types). Loaded from /profiles/types
	// per Rule 16.
	let profileTypes = $state<ProfileTypeOption[]>([]);
	let profileTypesLoading = $state(false);
	let profileTypesError = $state('');

	// Shared persona voices for the create/edit selectors (ADR-114).
	// Loaded from /admin/persona-voices per Rule 16.
	let voiceOptions = $state<PersonaVoice[]>([]);
	let voicesLoading = $state(false);
	let voicesError = $state('');

	// Server-side pagination state (BUG-003: list was not paginated — only 20
	// personas loaded with no way to see more). BUG-007: added search and sort
	// support per DataTable spec.
	let listPage = $state(0);
	let listPageSize = $state(10);
	let listTotal = $state(0);
	let listTotalPages = $state(1);
	let statusFilter = $state('');
	let searchQuery = $state('');
	let sortKey = $state('created_at');
	let sortDir = $state<'asc' | 'desc'>('desc');
	// Selection state for DataTable bulk actions.
	let selectedPersonaIds = $state<Set<string>>(new Set());

	// defaultPolicy returns the canonical default activity_policy defined in
	// §68 (ADR-068). These are spec-defined schema defaults, not runtime data —
	// the §68 section file is the source of truth for the default shape. Per
	// Rule 1, the section file wins. Ref: japaconnect-engineering/sections/68-ai-personas.md
	function defaultPolicy(): ActivityPolicy {
		return {
			actions_per_day: 10,
			action_budget: { comment: 4, forum_answer: 2, forum_question: 1, follow: 5, react: 20, bookmark: 10 },
			eligible_content_types: ['blog_post', 'guide', 'discussion', 'forum_thread'],
			eligible_spaces: [],
			min_content_age_hours: 1,
			avoid_own_content: true,
			avoid_recent_authors: [],
			active_hours_utc: [8, 22]
		};
	}

	function newPersonaInput(): PersonaInput {
		return {
			name: '',
			slug: '',
			tagline: '',
			voice_id: '',
			voice_prompt: '',
			disclosure_mode: 'disclosed',
			autonomy_mode: 'queued',
			activity_policy: defaultPolicy()
		};
	}

	let newPersonas = $state<PersonaInput[]>([newPersonaInput()]);

	// Edit SlideOver
	let editSlideOverOpen = $state(false);
	let editingPersona = $state<Persona | null>(null);
	let editForm = $state<UpdateInput>({});
	let editViewOnly = $state(false);
	let savingEdit = $state(false);

	// loadProfileTypes fetches the available professional profile types so
	// the create-form selector can offer them (BUG-007). Per Rule 16, the
	// list comes from the API — never hardcoded.
	async function loadProfileTypes() {
		profileTypesLoading = true;
		profileTypesError = '';
		try {
			const result = await api<ProfileTypeOption[]>('/profiles/types');
			if (result.ok) {
				profileTypes = result.data ?? [];
			} else {
				profileTypesError = result.error;
			}
		} catch (e) {
			profileTypesError = e instanceof Error ? e.message : 'Failed to load profile types';
		} finally {
			profileTypesLoading = false;
		}
	}

	async function loadPersonaVoices() {
		voicesLoading = true;
		voicesError = '';
		try {
			const result = await api<PersonaVoice[]>('/admin/persona-voices?limit=100');
			if (result.ok) {
				voiceOptions = result.data ?? [];
			} else {
				voicesError = result.error ?? 'Failed to load voices';
			}
		} catch (e) {
			voicesError = e instanceof Error ? e.message : 'Failed to load voices';
		} finally {
			voicesLoading = false;
		}
	}

	// Seed list state from the URL so reload/back/forward keep the same
	// sort, search, page and filter (DataTable only mirrors the UI; the
	// parent is responsible for the actual fetch state). Ref: admin DataTable
	// URL-sync contract and §68 "Search, sort, filter, pagination ...".
	function seedFromUrl() {
		// Read the live URL instead of $app/state, which can lag behind replaceState.
		const p = new URLSearchParams(window.location.search);
		sortKey = p.get('sort') ?? 'created_at';
		sortDir = (p.get('dir') as 'asc' | 'desc') ?? 'desc';
		searchQuery = p.get('q') ?? '';
		statusFilter = p.get('status') ?? '';
		const pageNum = Number(p.get('page') ?? '1');
		listPage = Math.max(0, pageNum - 1);
		const ps = Number(p.get('pageSize') ?? '10');
		if ([10, 25, 50, 100].includes(ps)) listPageSize = ps;
	}

	function syncStatusToUrl() {
		const params = new URLSearchParams(window.location.search);
		if (statusFilter) params.set('status', statusFilter);
		else params.delete('status');
		const qs = params.toString();
		replaceState(`${window.location.pathname}${qs ? `?${qs}` : ''}`, {});
	}

	async function load() {
		loading = true;
		error = '';
		try {
			// BUG-002: use page_size/page_offset (canonical convention).
			// BUG-007: pass q/sort/dir for DataTable search+sort support.
			const params = new URLSearchParams();
			params.set('page_size', String(listPageSize));
			params.set('page_offset', String(listPage * listPageSize));
			if (statusFilter) params.set('status', statusFilter);
			if (searchQuery) params.set('q', searchQuery);
			if (sortKey) params.set('sort', sortKey);
			if (sortDir) params.set('dir', sortDir);
			const result = await apiRaw<{ data: Persona[]; meta: { total: number; limit: number; offset: number } }>(
				`/admin/personas?${params.toString()}`
			);
			if (result.ok) {
				personas = result.data?.data ?? [];
				listTotal = result.data?.meta?.total ?? personas.length;
				listTotalPages = Math.max(1, Math.ceil(listTotal / listPageSize));
			} else {
				error = result.error;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('personas.load_error', 'Failed to load personas');
		} finally {
			loading = false;
		}
	}

	async function viewActivity(id: string) {
		activityPersonaId = id;
		activityPage = 0;
		activitySlideOverOpen = true;
		await loadActivity();
	}

	// BUG-004: activity log was not paginated — only 20 entries loaded with no
	// way to see more. Now fetches with page_size/page_offset (BUG-002) and
	// supports pagination.
	async function loadActivity() {
		activityLoading = true;
		try {
			const params = new URLSearchParams();
			params.set('page_size', String(activityPageSize));
			params.set('page_offset', String(activityPage * activityPageSize));
			const result = await apiRaw<{ data: ActivityEntry[]; meta: { total: number } }>(
				`/admin/personas/${activityPersonaId}/activity?${params.toString()}`
			);
			if (result.ok) {
				activityLog = result.data?.data ?? [];
				activityTotal = result.data?.meta?.total ?? activityLog.length;
			} else {
				activityLog = [];
				activityTotal = 0;
			}
		} catch {
			activityLog = [];
			activityTotal = 0;
		} finally {
			activityLoading = false;
		}
	}

	function activityTotalPages() {
		return Math.max(1, Math.ceil(activityTotal / activityPageSize));
	}

	async function createPersonas() {
		saving = true;
		message = '';
		try {
			const inputs = newPersonas.filter(p => p.name && p.slug);
			if (inputs.length === 0) {
				messageType = 'error';
				message = t('personas.empty_batch', 'At least one persona with name and slug is required');
				saving = false;
				return;
			}
			const result = await api('/admin/personas', { method: 'POST', body: inputs });
			if (result.ok) {
				messageType = 'success';
				message = t('personas.create_success', '{count} persona(s) provisioned.').replace('{count}', String(inputs.length));
				createSlideOverOpen = false;
				newPersonas = [newPersonaInput()];
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('personas.create_error', 'Failed to create personas');
		} finally {
			saving = false;
		}
	}

	function addPersonaRow() {
		newPersonas = [...newPersonas, newPersonaInput()];
	}

	function removePersonaRow(idx: number) {
		newPersonas = newPersonas.filter((_, i) => i !== idx);
	}

	function openView(persona: Persona) {
		editingPersona = persona;
		editViewOnly = true;
		// Merge a partial backend policy over the canonical default so the edit
		// form always has the required fields (e.g. active_hours_utc[0]).
		const policy = persona.activity_policy as Partial<ActivityPolicy> | undefined;
		editForm = {
			disclosure_mode: persona.disclosure_mode,
			autonomy_mode: persona.autonomy_mode,
			voice_id: persona.voice_id ?? '',
			voice_prompt: persona.voice_prompt_override ?? persona.voice_prompt ?? '',
			status: persona.status,
			activity_policy: policy ? { ...defaultPolicy(), ...policy } as ActivityPolicy : defaultPolicy()
		};
		editSlideOverOpen = true;
	}

	function openEdit(persona: Persona) {
		editingPersona = persona;
		editViewOnly = false;
		// BUG-005: include activity_policy in the edit form so it is editable
		// after creation (spec §68 PATCH updates activity_policy).
		// Merge a partial backend policy over the canonical default so the form
		// always has the required fields (e.g. active_hours_utc[0]).
		const policy = persona.activity_policy as Partial<ActivityPolicy> | undefined;
		editForm = {
			disclosure_mode: persona.disclosure_mode,
			autonomy_mode: persona.autonomy_mode,
			voice_id: persona.voice_id ?? '',
			voice_prompt: persona.voice_prompt_override ?? persona.voice_prompt ?? '',
			status: persona.status,
			activity_policy: policy ? { ...defaultPolicy(), ...policy } as ActivityPolicy : defaultPolicy()
		};
		editSlideOverOpen = true;
	}

	async function updatePersonaStatus(persona: Persona, status: string) {
		message = '';
		try {
			const result = await api(`/admin/personas/${persona.id}`, { method: 'PATCH', body: { status } });
			if (result.ok) {
				messageType = 'success';
				message = t('personas.status_success', 'Persona status updated to {status}.').replace('{status}', status);
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('personas.update_error', 'Failed to update persona');
		}
	}

	// BUG-007: bulk actions per DataTable spec. Pause/retire let admins
	// manage multiple personas at once. In-flight guard prevents duplicate
	// calls on double-click (admin AGENTS.md bulk-delete pattern).
	let bulkProcessing = $state(false);

	async function bulkUpdateStatus(ids: Set<string>, status: string) {
		const idArr = Array.from(ids);
		if (idArr.length === 0 || bulkProcessing) return;
		bulkProcessing = true;
		message = '';
		try {
			const results = await Promise.all(idArr.map((id) =>
				api(`/admin/personas/${id}`, { method: 'PATCH', body: { status } })
			));
			const failed = results.filter((r) => !r.ok);
			if (failed.length > 0) {
				messageType = 'error';
				message = t('personas.bulk_status_partial', '{success} succeeded, {failed} failed: {error}')
					.replace('{success}', String(results.length - failed.length))
					.replace('{failed}', String(failed.length))
					.replace('{error}', failed[0].error ?? 'Unknown error');
			} else {
				messageType = 'success';
				message = t('personas.bulk_status_success', '{count} persona(s) updated to {status}.')
					.replace('{count}', String(idArr.length))
					.replace('{status}', status);
				selectedPersonaIds = new Set();
			}
			load();
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('personas.bulk_error', 'Bulk update failed');
		} finally {
			bulkProcessing = false;
		}
	}

	const bulkActionsList: BulkAction[] = [
		{
			label: t('personas.bulk_pause', 'Pause'),
			onclick: (ids: Set<string>) => bulkUpdateStatus(ids, 'paused'),
			disabled: (_ids: Set<string>, rows?: any[]) => {
				// Only allow pausing when at least one selected persona is active.
				return !rows?.some((r: Persona) => r.status === 'active');
			}
		},
		{
			label: t('personas.bulk_retire', 'Retire'),
			onclick: (ids: Set<string>) => bulkUpdateStatus(ids, 'retired'),
			variant: 'destructive',
			disabled: (_ids: Set<string>, rows?: any[]) => {
				// Only allow retiring when at least one selected persona is not retired.
				return !rows?.some((r: Persona) => r.status !== 'retired');
			}
		}
	];

	async function submitEdit() {
		if (!editingPersona) return;
		savingEdit = true;
		message = '';
		try {
			const body: UpdateInput = { ...editForm };
			// voice_prompt is always sent as a string (bound to a textarea).
			// An empty string clears it on the backend because the PATCH
			// handler decodes VoicePrompt as *string — empty is non-nil, so
			// COALESCE sets the column to '' rather than keeping the old value.
			const result = await api(`/admin/personas/${editingPersona.id}`, { method: 'PATCH', body });
			if (result.ok) {
				messageType = 'success';
				message = t('personas.update_success', 'Persona updated.');
				editSlideOverOpen = false;
				editingPersona = null;
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('personas.update_error', 'Failed to update persona');
		} finally {
			savingEdit = false;
		}
	}

	const columns = $derived([
		{
			key: 'profile_name',
			sortKey: 'name',
			label: t('personas.name', 'Name'),
			sortable: true,
			render: (value: string) => `<span class="text-foreground font-medium">${value ?? '—'}</span>`
		},
		{
			key: 'voice_name',
			label: t('personas.voice', 'Voice / Tone'),
			render: (value: string) => `<span class="text-muted-foreground text-xs">${value ?? '—'}</span>`
		},
		{
			key: 'backing_username',
			label: t('personas.username', 'Username'),
			render: (value: string) => `<span class="font-mono text-xs text-muted-foreground">${value ?? '—'}</span>`
		},
		{
			key: 'disclosure_mode',
			label: t('personas.disclosure', 'Disclosure'),
			render: (value: string) => `<span class="text-foreground">${value}</span>`
		},
		{
			key: 'autonomy_mode',
			label: t('personas.autonomy', 'Autonomy'),
			render: (value: string) => `<span class="text-foreground">${value}</span>`
		},
		{
			key: 'status',
			label: t('personas.status', 'Status'),
			render: (value: string) => {
				const cls = value === 'active' ? 'bg-success/15 text-success border-success' : 'bg-muted text-muted-foreground border-border';
				return `<span class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${cls}">${value}</span>`;
			}
		},
		{
			key: 'total_actions',
			label: t('personas.total_actions', 'Total Actions'),
			sortable: true,
			render: (value: number) => `<span class="text-muted-foreground text-xs">${value ?? 0}</span>`
		},
		{
			key: 'created_at',
			label: t('personas.created', 'Created'),
			sortable: true,
			render: (value: string) => `<span class="text-muted-foreground text-xs">${value ? new Date(value).toLocaleDateString() : '—'}</span>`
		}
	]);

	const rowActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row: Persona) => openView(row) },
		{ label: t('personas.activity_log', 'Activity'), icon: Activity, onclick: (row: Persona) => viewActivity(row.id), show: () => canViewActivity },
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row: Persona) => openEdit(row),
			show: () => canManage
		},
		{
			label: t('common.pause', 'Pause'),
			icon: Pause,
			onclick: (row: Persona) => updatePersonaStatus(row, 'paused'),
			show: (row: any) => canManage && row.status === 'active'
		},
		{
			label: t('common.resume', 'Resume'),
			icon: Play,
			onclick: (row: Persona) => updatePersonaStatus(row, 'active'),
			show: (row: any) => canManage && row.status === 'paused'
		},
		{
			label: t('personas.bulk_retire', 'Retire'),
			icon: Trash2,
			onclick: (row: Persona) => updatePersonaStatus(row, 'retired'),
			variant: 'destructive',
			show: (row: any) => canManage && row.status !== 'retired'
		}
	];

	seedFromUrl();
	load();
	loadProfileTypes();
	loadPersonaVoices();
</script>

<PageHeader
	title={t('personas.title', 'AI Personas')}
	description={t('personas.description', 'Manage AI persona accounts for content generation and participation.')}
	actions={canManage ? [
		{ label: t('personas.create', 'Create Personas'), icon: Plus, onclick: () => (createSlideOverOpen = true) }
	] : []}
/>

{#if message}
	<div
		class="mb-4 rounded-lg border p-3 text-sm"
		style={messageType === 'success'
			? 'border-color: var(--color-success); background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)'
			: 'border-color: var(--color-danger); background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)'}
	>
		{message}
	</div>
{/if}

{#if loading}
	<div class="flex items-center gap-2 p-8" style="color: var(--color-muted)">
		<Bot class="h-4 w-4 animate-pulse" />
		<span>{t('personas.loading', 'Loading personas...')}</span>
	</div>
{:else if error}
	<div class="rounded-lg border p-4" style="border-color: var(--color-danger); color: var(--color-danger)">
		{error}
	</div>
{:else}
	<div class="mb-4 flex items-center gap-3">
		<label for="status-filter" class="text-xs font-medium" style="color: var(--color-muted)">{t('personas.status', 'Status')}</label>
		<select
			id="status-filter"
			class="rounded-md border px-3 py-1.5 text-sm"
			style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			value={statusFilter}
			onchange={(e) => { statusFilter = (e.target as HTMLSelectElement).value; listPage = 0; syncStatusToUrl(); load(); }}
		>
			<option value="">{t('personas.all_statuses', 'All statuses')}</option>
			<option value="active">{t('personas.status_active', 'Active')}</option>
			<option value="paused">{t('personas.status_paused', 'Paused')}</option>
			<option value="retired">{t('personas.status_retired', 'Retired')}</option>
		</select>
	</div>
	<DataTable
		data={personas}
		{columns}
		actions={rowActions}
		serverSide
		searchable
		selectable={canManage}
		searchPlaceholder={t('personas.search_placeholder', 'Search by name or username...')}
		onsearch={(q) => { searchQuery = q; listPage = 0; load(); }}
		onsortchange={(s) => { sortKey = s.key ?? 'created_at'; sortDir = s.dir; listPage = 0; load(); }}
		bulkActionsList={canManage ? bulkActionsList : undefined}
		bind:selectedIds={selectedPersonaIds}
		bind:page={listPage}
		totalPages={listTotalPages}
		totalRows={listTotal}
		bind:pageSize={listPageSize}
		onpagechange={(p) => { listPage = p; load(); }}
		onpagesizechange={(s) => { listPageSize = s; listPage = 0; load(); }}
		emptyMessage={t('personas.empty', 'No personas found. Create some to get started.')}
	/>
{/if}

{#if createSlideOverOpen}
	<SlideOver
		bind:open={createSlideOverOpen}
		title={t('personas.create_title', 'Create AI Personas')}
	>
		<div class="space-y-4">
			<p class="text-sm" style="color: var(--color-muted)">
				{t('personas.create_hint', 'Provision one or more AI personas. All personas in a batch are created in a single transaction.')}
			</p>
			{#each newPersonas as persona, idx (idx)}
				<div class="rounded-lg border p-3" style="border-color: var(--color-border)">
					<div class="mb-2 flex items-center justify-between">
						<span class="text-sm font-medium" style="color: var(--color-foreground)">{t('personas.persona_n', 'Persona #{n}').replace('{n}', String(idx + 1))}</span>
						{#if newPersonas.length > 1}
							<button type="button" onclick={() => removePersonaRow(idx)} style="color: var(--color-danger)">
								<Trash2 class="h-4 w-4" />
							</button>
						{/if}
					</div>
					<div class="grid gap-3 sm:grid-cols-2">
						<div>
							<label for={`persona-name-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.name_label', 'Name')} *</label>
							<input
								id={`persona-name-${idx}`}
								type="text"
								class="w-full rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={persona.name}
								placeholder={t('personas.name_placeholder', 'e.g. Adaora Okafor')}
							/>
						</div>
						<div>
							<label for={`persona-slug-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.slug_label', 'Slug')} *</label>
							<input
								id={`persona-slug-${idx}`}
								type="text"
								class="w-full rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={persona.slug}
								placeholder={t('personas.slug_placeholder', 'e.g. adaora-okafor')}
							/>
						</div>
					</div>
					<div class="mt-2">
						<label for={`persona-type-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.profile_type', 'Profile Type')}</label>
						{#if profileTypesLoading}
							<p class="text-xs" style="color: var(--color-muted)">{t('personas.loading_types', 'Loading profile types...')}</p>
						{:else if profileTypesError}
							<p class="text-xs" style="color: var(--color-danger)">{profileTypesError}</p>
						{:else}
							<select
								id={`persona-type-${idx}`}
								class="w-full rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={persona.type_id}
							>
								<option value="">{t('personas.type_default', 'Default (contributor/columnist)')}</option>
								{#each profileTypes as pt (pt.id)}
									<option value={pt.id}>{pt.name} ({pt.slug})</option>
								{/each}
							</select>
						{/if}
					</div>
					<div class="mt-2">
						<label for={`persona-tagline-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.tagline', 'Tagline')}</label>
						<input
							id={`persona-tagline-${idx}`}
							type="text"
							class="w-full rounded-md border px-3 py-1.5 text-sm"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
							bind:value={persona.tagline}
							placeholder={t('personas.tagline_placeholder', 'Short bio / expertise summary')}
						/>
					</div>
					<div class="mt-2 grid gap-3 sm:grid-cols-2">
						<div>
							<label for={`persona-disclosure-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.disclosure', 'Disclosure Mode')}</label>
							<select
								id={`persona-disclosure-${idx}`}
								class="w-full rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={persona.disclosure_mode}
							>
								<option value="disclosed">{t('personas.disclosure_disclosed', 'Disclosed')}</option>
								<option value="undisclosed">{t('personas.disclosure_undisclosed', 'Undisclosed')}</option>
							</select>
						</div>
						<div>
							<label for={`persona-autonomy-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.autonomy', 'Autonomy Mode')}</label>
							<select
								id={`persona-autonomy-${idx}`}
								class="w-full rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={persona.autonomy_mode}
							>
								<option value="queued">{t('personas.autonomy_queued', 'Queued (admin review)')}</option>
								<option value="autonomous">{t('personas.autonomy_autonomous', 'Autonomous')}</option>
							</select>
						</div>
					</div>
					<div class="mt-2">
						<label for={`persona-voice-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.voice', 'Voice / Tone')}</label>
						{#if voicesLoading}
							<p class="text-xs" style="color: var(--color-muted)">{t('personas.loading_voices', 'Loading voices...')}</p>
						{:else if voicesError}
							<p class="text-xs" style="color: var(--color-danger)">{voicesError}</p>
						{:else}
							<select
								id={`persona-voice-${idx}`}
								class="w-full rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={persona.voice_id}
							>
								<option value="">{t('personas.voice_none', 'Create new voice from prompt below')}</option>
								{#each voiceOptions as voice (voice.id)}
									<option value={voice.id}>{voice.name}</option>
								{/each}
							</select>
						{/if}
					</div>
					<div class="mt-2">
						<label for={`persona-voice-override-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.voice_prompt', 'Voice Prompt (or new voice prompt if none selected)')}</label>
						<textarea
							id={`persona-voice-override-${idx}`}
							class="w-full rounded-md border px-3 py-1.5 text-sm"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
							rows="2"
							bind:value={persona.voice_prompt}
							placeholder={t('personas.voice_prompt_placeholder', 'Describe the persona\'s voice and personality...')}
						></textarea>
					</div>
					<details class="mt-2">
						<summary class="cursor-pointer text-xs font-medium" style="color: var(--color-muted)">{t('personas.policy_details', 'Activity Policy')}</summary>
						<div class="mt-2 grid gap-3 sm:grid-cols-2">
							<div>
								<label for={`persona-actions-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.actions_per_day', 'Actions per day')}</label>
								<input
									id={`persona-actions-${idx}`}
									type="number"
									min="0"
									max="100"
									class="w-full rounded-md border px-3 py-1.5 text-sm"
									style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
									bind:value={persona.activity_policy!.actions_per_day}
								/>
							</div>
							<div>
								<label for={`persona-min-age-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.min_content_age', 'Min content age (hours)')}</label>
								<input
									id={`persona-min-age-${idx}`}
									type="number"
									min="0"
									class="w-full rounded-md border px-3 py-1.5 text-sm"
									style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
									bind:value={persona.activity_policy!.min_content_age_hours}
								/>
							</div>
							<div class="sm:col-span-2">
								<label for={`persona-content-types-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.eligible_content_types', 'Eligible Content Types (comma-separated)')}</label>
								<input
									id={`persona-content-types-${idx}`}
									type="text"
									class="w-full rounded-md border px-3 py-1.5 text-sm"
									style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
									value={persona.activity_policy!.eligible_content_types.join(', ')}
									oninput={(e) => { persona.activity_policy!.eligible_content_types = (e.target as HTMLInputElement).value.split(',').map(s => s.trim()).filter(Boolean); }}
									placeholder="blog_post, guide, discussion"
								/>
							</div>
							<div class="sm:col-span-2">
								<label for={`persona-spaces-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.eligible_spaces', 'Eligible Spaces (comma-separated)')}</label>
								<input
									id={`persona-spaces-${idx}`}
									type="text"
									class="w-full rounded-md border px-3 py-1.5 text-sm"
									style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
									value={persona.activity_policy!.eligible_spaces.join(', ')}
									oninput={(e) => { persona.activity_policy!.eligible_spaces = (e.target as HTMLInputElement).value.split(',').map(s => s.trim()).filter(Boolean); }}
									placeholder="lagos, abuja"
								/>
							</div>
							<div class="sm:col-span-2">
								<label for={`persona-action-budget-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.action_budget', 'Action Budget (JSON)')}</label>
							<textarea
								id={`persona-action-budget-${idx}`}
								class="w-full rounded-md border px-3 py-1.5 text-sm font-mono"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								rows="2"
								value={JSON.stringify(persona.activity_policy!.action_budget)}
								oninput={(e) => {
									try {
										persona.activity_policy!.action_budget = JSON.parse((e.target as HTMLTextAreaElement).value || '{}');
									} catch { /* leave unchanged on parse error */ }
								}}
								placeholder={'{"comment": 4, "react": 20}'}
							></textarea>
							<p class="mt-1 text-[10px]" style="color: var(--color-muted)">{t('personas.action_budget_hint', 'Map of action type to max count per day.')}</p>
						</div>
						<div class="sm:col-span-2">
							<label for={`persona-avoid-recent-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.avoid_recent_authors', 'Avoid Recent Authors (comma-separated user IDs)')}</label>
							<input
								id={`persona-avoid-recent-${idx}`}
								type="text"
								class="w-full rounded-md border px-3 py-1.5 text-sm font-mono"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								value={persona.activity_policy!.avoid_recent_authors.join(', ')}
								oninput={(e) => { persona.activity_policy!.avoid_recent_authors = (e.target as HTMLInputElement).value.split(',').map(s => s.trim()).filter(Boolean); }}
								placeholder="uuid, uuid"
							/>
						</div>
						<div class="sm:col-span-2">
							<label for={`persona-active-hours-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.active_hours_utc', 'Active Hours UTC (start-end)')}</label>
								<div class="flex items-center gap-2">
									<input
										id={`persona-active-hours-start-${idx}`}
										type="number"
										min="0"
										max="23"
										class="w-full rounded-md border px-3 py-1.5 text-sm"
										style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
										bind:value={persona.activity_policy!.active_hours_utc[0]}
										placeholder="8"
									/>
									<span class="text-xs" style="color: var(--color-muted)">—</span>
									<input
										id={`persona-active-hours-end-${idx}`}
										type="number"
										min="0"
										max="23"
										class="w-full rounded-md border px-3 py-1.5 text-sm"
										style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
										bind:value={persona.activity_policy!.active_hours_utc[1]}
										placeholder="22"
									/>
								</div>
							</div>
							<div class="flex items-center gap-2 sm:col-span-2">
								<input
									id={`persona-avoid-own-${idx}`}
									type="checkbox"
									class="rounded"
									bind:checked={persona.activity_policy!.avoid_own_content}
								/>
								<label for={`persona-avoid-own-${idx}`} class="text-xs" style="color: var(--color-muted)">{t('personas.avoid_own_content', 'Avoid own content')}</label>
							</div>
						</div>
					</details>
				</div>
			{/each}
			<Button variant="outline" onclick={addPersonaRow}>
				<Plus class="h-4 w-4" />
				{t('personas.add_row', 'Add another persona')}
			</Button>
		</div>
		{#snippet footer()}
			<div class="flex justify-end gap-2">
				<Button variant="outline" onclick={() => (createSlideOverOpen = false)}>{t('common.cancel', 'Cancel')}</Button>
				<Button onclick={createPersonas} disabled={saving}>
					{saving ? t('common.saving', 'Saving...') : t('personas.provision', 'Provision Personas')}
				</Button>
			</div>
		{/snippet}
	</SlideOver>
{/if}

{#if activitySlideOverOpen}
	<SlideOver
		bind:open={activitySlideOverOpen}
		title={t('personas.activity_log', 'Persona Activity Log')}
	>
		{#if activityLoading}
			<div class="p-4 text-center" style="color: var(--color-muted)">
				<Activity class="mx-auto mb-2 h-6 w-6 animate-pulse" />
				{t('personas.loading_activity', 'Loading activity...')}
			</div>
		{:else if activityLog.length === 0}
			<p class="p-4 text-center text-sm" style="color: var(--color-muted)">{t('personas.no_activity', 'No activity recorded.')}</p>
		{:else}
			<div class="max-h-96 space-y-2 overflow-y-auto">
				{#each activityLog as entry (entry.id)}
					<div class="rounded-md border p-2 text-xs" style="border-color: var(--color-border); color: var(--color-foreground)">
						<div class="flex items-center justify-between gap-2">
							<span class="font-medium">{entry.event_type}</span>
							<span style="color: var(--color-muted)">{entry.created_at ? new Date(entry.created_at).toLocaleString() : '—'}</span>
						</div>
						{#if entry.entity_type}
							<div class="mt-1" style="color: var(--color-muted)">
								{t('personas.entity', 'Entity')}: {entry.entity_type}
								{#if entry.entity_id}
									<span class="font-mono">{entry.entity_id.slice(0, 8)}</span>
								{/if}
							</div>
						{/if}
						{#if entry.is_synthetic}
							<span class="mt-1 inline-block rounded border px-1 py-0.5 text-[10px]" style="border-color: var(--color-border); color: var(--color-muted)">synthetic</span>
						{/if}
						{#if entry.meta && Object.keys(entry.meta).length > 0}
							<details class="mt-1">
								<summary class="cursor-pointer" style="color: var(--color-muted)">{t('personas.meta', 'Meta')}</summary>
								<pre class="mt-1 overflow-x-auto rounded p-1 text-[10px]" style="background: var(--color-muted); color: var(--color-foreground)">{JSON.stringify(entry.meta, null, 2)}</pre>
							</details>
						{/if}
					</div>
				{/each}
			</div>
			{#if activityTotalPages() > 1}
				<div class="mt-3 flex items-center justify-between">
					<span class="text-xs" style="color: var(--color-muted)">
						{t('common.page', 'Page')} {activityPage + 1} / {activityTotalPages()} ({activityTotal} total)
					</span>
					<div class="flex gap-1">
						<Button variant="outline" size="sm" disabled={activityPage === 0 || activityLoading}
							onclick={() => { activityPage--; loadActivity(); }}>
							{t('common.prev', 'Prev')}
						</Button>
						<Button variant="outline" size="sm" disabled={activityPage >= activityTotalPages() - 1 || activityLoading}
							onclick={() => { activityPage++; loadActivity(); }}>
							{t('common.next', 'Next')}
						</Button>
					</div>
				</div>
			{/if}
		{/if}
	</SlideOver>
{/if}

{#if editSlideOverOpen}
	<SlideOver
		bind:open={editSlideOverOpen}
		title={editViewOnly ? t('personas.view_title', 'View Persona') : t('personas.edit_title', 'Edit Persona')}
		onclose={() => { editingPersona = null; editViewOnly = false; }}
	>
		<fieldset class="space-y-4 border-0 p-0 m-0" disabled={editViewOnly}>
			<div>
				<label for="edit-persona-id" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">ID</label>
				<p class="font-mono text-xs" style="color: var(--color-foreground)">{editingPersona?.id}</p>
			</div>
			<div class="grid gap-3 sm:grid-cols-2">
				<div>
					<label for="edit-persona-disclosure" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.disclosure', 'Disclosure Mode')}</label>
					<select
						id="edit-persona-disclosure"
						class="w-full rounded-md border px-3 py-1.5 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={editForm.disclosure_mode}
					>
						<option value="disclosed">{t('personas.disclosure_disclosed', 'Disclosed')}</option>
						<option value="undisclosed">{t('personas.disclosure_undisclosed', 'Undisclosed')}</option>
					</select>
				</div>
				<div>
					<label for="edit-persona-autonomy" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.autonomy', 'Autonomy Mode')}</label>
					<select
						id="edit-persona-autonomy"
						class="w-full rounded-md border px-3 py-1.5 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={editForm.autonomy_mode}
					>
						<option value="queued">{t('personas.autonomy_queued', 'Queued (admin review)')}</option>
						<option value="autonomous">{t('personas.autonomy_autonomous', 'Autonomous')}</option>
					</select>
				</div>
			</div>
			<div>
				<label for="edit-persona-status" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.status', 'Status')}</label>
				<select
					id="edit-persona-status"
					class="w-full rounded-md border px-3 py-1.5 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					bind:value={editForm.status}
				>
					<option value="active">{t('personas.status_active', 'Active')}</option>
					<option value="paused">{t('personas.status_paused', 'Paused')}</option>
					<option value="retired">{t('personas.status_retired', 'Retired')}</option>
				</select>
			</div>
			<div>
				<label for="edit-persona-voice" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.voice', 'Voice / Tone')}</label>
				{#if voicesLoading}
					<p class="text-xs" style="color: var(--color-muted)">{t('personas.loading_voices', 'Loading voices...')}</p>
				{:else if voicesError}
					<p class="text-xs" style="color: var(--color-danger)">{voicesError}</p>
				{:else}
					<select
						id="edit-persona-voice"
						class="w-full rounded-md border px-3 py-1.5 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={editForm.voice_id}
					>
						<option value="">{t('personas.voice_none', 'No shared voice')}</option>
						{#each voiceOptions as voice (voice.id)}
							<option value={voice.id}>{voice.name}</option>
						{/each}
					</select>
				{/if}
			</div>
			<div>
				<label for="edit-persona-voice-override" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.voice_prompt', 'Voice Override Prompt (optional)')}</label>
				<textarea
					id="edit-persona-voice-override"
					class="w-full rounded-md border px-3 py-1.5 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					rows="2"
					bind:value={editForm.voice_prompt}
					placeholder={t('personas.voice_prompt_placeholder', "Describe the persona's voice and personality...")}
				></textarea>
			</div>
			{#if editForm.activity_policy}
				<details>
					<summary class="cursor-pointer text-xs font-medium" style="color: var(--color-muted)">{t('personas.policy_details', 'Activity Policy')}</summary>
					<div class="mt-2 grid gap-3 sm:grid-cols-2">
						<div>
							<label for="edit-persona-actions" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.actions_per_day', 'Actions per day')}</label>
							<input
								id="edit-persona-actions"
								type="number"
								min="0"
								max="100"
								class="w-full rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={editForm.activity_policy.actions_per_day}
							/>
						</div>
						<div>
							<label for="edit-persona-min-age" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.min_content_age', 'Min content age (hours)')}</label>
							<input
								id="edit-persona-min-age"
								type="number"
								min="0"
								class="w-full rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={editForm.activity_policy.min_content_age_hours}
							/>
						</div>
						<div class="sm:col-span-2">
							<label for="edit-persona-content-types" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.eligible_content_types', 'Eligible Content Types (comma-separated)')}</label>
							<input
								id="edit-persona-content-types"
								type="text"
								class="w-full rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								value={editForm.activity_policy.eligible_content_types.join(', ')}
								oninput={(e) => { editForm.activity_policy!.eligible_content_types = (e.target as HTMLInputElement).value.split(',').map(s => s.trim()).filter(Boolean); }}
								placeholder="blog_post, guide, discussion"
							/>
						</div>
						<div class="sm:col-span-2">
							<label for="edit-persona-spaces" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.eligible_spaces', 'Eligible Spaces (comma-separated)')}</label>
							<input
								id="edit-persona-spaces"
								type="text"
								class="w-full rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								value={editForm.activity_policy.eligible_spaces.join(', ')}
								oninput={(e) => { editForm.activity_policy!.eligible_spaces = (e.target as HTMLInputElement).value.split(',').map(s => s.trim()).filter(Boolean); }}
								placeholder="lagos, abuja"
							/>
						</div>
						<div class="sm:col-span-2">
							<label for="edit-persona-action-budget" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.action_budget', 'Action Budget (JSON)')}</label>
							<textarea
								id="edit-persona-action-budget"
								class="w-full rounded-md border px-3 py-1.5 text-sm font-mono"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								rows="2"
								value={JSON.stringify(editForm.activity_policy.action_budget)}
								oninput={(e) => {
									try {
										editForm.activity_policy!.action_budget = JSON.parse((e.target as HTMLTextAreaElement).value || '{}');
									} catch { /* leave unchanged on parse error */ }
								}}
								placeholder={'{"comment": 4, "react": 20}'}
							></textarea>
							<p class="mt-1 text-[10px]" style="color: var(--color-muted)">{t('personas.action_budget_hint', 'Map of action type to max count per day.')}</p>
						</div>
						<div class="sm:col-span-2">
							<label for="edit-persona-avoid-recent" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.avoid_recent_authors', 'Avoid Recent Authors (comma-separated user IDs)')}</label>
							<input
								id="edit-persona-avoid-recent"
								type="text"
								class="w-full rounded-md border px-3 py-1.5 text-sm font-mono"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								value={editForm.activity_policy.avoid_recent_authors.join(', ')}
								oninput={(e) => { editForm.activity_policy!.avoid_recent_authors = (e.target as HTMLInputElement).value.split(',').map(s => s.trim()).filter(Boolean); }}
								placeholder="uuid, uuid"
							/>
						</div>
						<div class="sm:col-span-2">
							<label for="edit-persona-active-hours" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.active_hours_utc', 'Active Hours UTC (start-end)')}</label>
							<div class="flex items-center gap-2">
								<input
									id="edit-persona-active-hours-start"
									type="number"
									min="0"
									max="23"
									class="w-full rounded-md border px-3 py-1.5 text-sm"
									style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
									bind:value={editForm.activity_policy.active_hours_utc[0]}
									placeholder="8"
								/>
								<span class="text-xs" style="color: var(--color-muted)">—</span>
								<input
									id="edit-persona-active-hours-end"
									type="number"
									min="0"
									max="23"
									class="w-full rounded-md border px-3 py-1.5 text-sm"
									style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
									bind:value={editForm.activity_policy.active_hours_utc[1]}
									placeholder="22"
								/>
							</div>
						</div>
						<div class="flex items-center gap-2 sm:col-span-2">
							<input
								id="edit-persona-avoid-own"
								type="checkbox"
								class="rounded"
								bind:checked={editForm.activity_policy.avoid_own_content}
							/>
							<label for="edit-persona-avoid-own" class="text-xs" style="color: var(--color-muted)">{t('personas.avoid_own_content', 'Avoid own content')}</label>
						</div>
					</div>
				</details>
			{/if}
		</fieldset>
		{#snippet footer()}
			<div class="flex justify-end gap-2">
				{#if editViewOnly}
					<Button onclick={() => (editSlideOverOpen = false)}>{t('common.close', 'Close')}</Button>
				{:else}
					<Button variant="outline" onclick={() => (editSlideOverOpen = false)}>{t('common.cancel', 'Cancel')}</Button>
					<Button onclick={submitEdit} disabled={savingEdit}>
						{savingEdit ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
					</Button>
				{/if}
			</div>
		{/snippet}
	</SlideOver>
{/if}
