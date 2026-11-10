<script lang="ts">
	import { t } from '#lib/stores/i18n.svelte.js';
	import { hasPermission } from '#lib/stores/permissions.svelte.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Modal } from '#lib/components/ui/modal/index.js';
	import { EntityPage, type EntityPageConfig, type FieldDef, type EntityPageController } from '#lib/components/entity/index.js';
	import type { ActionItem } from '#lib/components/ui/data-table/index.js';
	import { Plus, Trash2, Activity, Pencil, Pause, Play, Eye, Image as ImageIcon } from 'lucide-svelte';
	import { api, apiRaw } from '#lib/api.js';
	import { JsonValueEditor } from '@japaconnect/ui';
	import type { BulkAction } from '#lib/components/ui/data-table/index.js';
	import AutocompleteSelect from '#lib/components/ui/AutocompleteSelect.svelte';
	import GeoSpaceCascade from '#lib/components/ui/GeoSpaceCascade.svelte';
	import MediaPickerModal from '#lib/components/ui/MediaPickerModal.svelte';

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
		profile_tagline?: string | null;
		geo_space_id?: string;
		geo_space_name?: string | null;
		geo_space_slug?: string | null;
		geo_space_level?: string | null;
		avatar_url?: string | null;
		cover_url?: string | null;
		avatar_media_id?: string;
		header_media_id?: string;
		profession?: string | null;
		location?: string | null;
		website_url?: string | null;
		education?: string | null;
		interests?: string[];
		tips_enabled?: boolean;
		points_balance?: number;
		total_actions?: number;
	};

	// PersonaInput matches the backend PersonaInput struct. The backend
	// requires name and slug (service.go validates both). Ref: §68.
	type PersonaInput = {
		name: string;
		slug: string;
		tagline?: string;
		voice_id?: string;
		voice_prompt?: string;
		disclosure_mode: string;
		autonomy_mode: string;
		activity_policy?: ActivityPolicy;
		geo_space?: string;
	};

	// PersonaVoiceOption matches the backend slim picker projection
	// (GET /admin/persona-voices?slim=1 → {id, name}). Ref: ADR-114.
	type PersonaVoiceOption = {
		id: string;
		name: string;
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
	// so admins can pause/retire personas from the UI; name/tagline/username
	// edit the backing bot's byline and handle (§68 identity fields).
	type UpdateInput = {
		disclosure_mode?: string;
		autonomy_mode?: string;
		voice_id?: string;
		voice_prompt?: string;
		status?: string;
		activity_policy?: ActivityPolicy;
		name?: string;
		tagline?: string;
		username?: string;
		geo_space?: string;
		avatar_media_id?: string;
		header_media_id?: string;
		profession?: string;
		location?: string;
		website_url?: string;
		education?: string;
		interests?: string[];
		tips_enabled?: boolean;
	};

	const canManage = $derived(hasPermission('persona.manage'));
	// The activity endpoint is OR-gated persona.view | participation.view
	// (BUG-PER-002), so surface the action when the user holds either node.
	const canViewActivity = $derived(hasPermission('persona.view') || hasPermission('participation.view'));

	let entityController = $state<EntityPageController | undefined>(undefined);
	let createModalOpen = $state(false);
	let activityModalOpen = $state(false);
	let activityLog = $state<ActivityEntry[]>([]);
	let activityLoading = $state(false);
	let activityError = $state('');
	let activityTotal = $state(0);
	let activityPage = $state(0);
	let activityPageSize = $state(10);
	let activityPersonaId = $state('');
	let saving = $state(false);
	let message = $state('');
	let messageType = $state('');

	// Shared persona voices for the create/edit selectors (ADR-114).
	// Loaded from /admin/persona-voices per Rule 16.
	let voiceOptions = $state<PersonaVoiceOption[]>([]);
	let voicesLoading = $state(false);
	let voicesError = $state('');

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
			disclosure_mode: 'undisclosed',
			autonomy_mode: 'queued',
			activity_policy: defaultPolicy(),
			geo_space: ''
		};
	}

	let newPersonas = $state<PersonaInput[]>([newPersonaInput()]);

	// Edit Modal
	let editModalOpen = $state(false);
	let editingPersona = $state<Persona | null>(null);
	let editForm = $state<UpdateInput>({});
	let editViewOnly = $state(false);
	let savingEdit = $state(false);
	// Geo space for the edit cascade — the new selection diff-sends as
	// geo_space on save (mirrors the name/tagline/username pattern).
	let editGeoSpace = $state('');
	let editGeoCleared = $state(false);
	// Profile editing (Plan B): media pickers + interests text input. The
	// pickers set editForm.*_media_id; *_url fields on the form are preview-
	// only — the backend resolves the canonical URL from the media id.
	let avatarPickerOpen = $state(false);
	let coverPickerOpen = $state(false);
	let editInterestsText = $state('');
	let editAvatarUrl = $state('');
	let editCoverUrl = $state('');

	async function loadPersonaVoices() {
		voicesLoading = true;
		voicesError = '';
		try {
			// slim=1 → {id, name} picker rows; page_size=5000 covers the full
			// shared voice catalog (ORDER BY name ASC — a bare limit=100 only
			// returns the "Academic …" prefix of the alphabetical list).
			const result = await api<PersonaVoiceOption[]>('/admin/persona-voices?slim=1&page_size=5000');
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

	async function viewActivity(id: string) {
		activityPersonaId = id;
		activityPage = 0;
		activityModalOpen = true;
		await loadActivity();
	}

	// BUG-004: activity log was not paginated — only 20 entries loaded with no
	// way to see more. Now fetches with page_size/page_offset (BUG-002) and
	// supports pagination.
	async function loadActivity() {
		activityLoading = true;
		activityError = '';
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
				activityError = result.error ?? t('personas.activity_error', 'Failed to load activity');
			}
		} catch (e) {
			activityLog = [];
			activityTotal = 0;
			activityError = e instanceof Error ? e.message : t('personas.activity_error', 'Failed to load activity');
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
				createModalOpen = false;
				newPersonas = [newPersonaInput()];
				entityController?.reload();
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

	function seedEditForm(persona: Persona, viewOnly: boolean) {
		editingPersona = persona;
		editViewOnly = viewOnly;
		editGeoSpace = '';
		editGeoCleared = false;
		editInterestsText = (persona.interests ?? []).join(', ');
		editAvatarUrl = persona.avatar_url ?? '';
		editCoverUrl = persona.cover_url ?? '';
		// Merge a partial backend policy over the canonical default so the edit
		// form always has the required fields (e.g. active_hours_utc[0]).
		const policy = persona.activity_policy as Partial<ActivityPolicy> | undefined;
		editForm = {
			name: persona.profile_name ?? '',
			username: persona.backing_username ?? '',
			tagline: persona.profile_tagline ?? '',
			disclosure_mode: persona.disclosure_mode,
			autonomy_mode: persona.autonomy_mode,
			voice_id: persona.voice_id ?? '',
			// Seed ONLY the per-persona override — not the resolved prompt.
			// voice_prompt on the row is COALESCE(override, shared prompt), so
			// seeding it would freeze a copy of the shared voice as an override
			// on the next save, cutting the persona off from shared-voice
			// updates (ADR-114 resolution order).
			voice_prompt: persona.voice_prompt_override ?? '',
			status: persona.status,
			activity_policy: policy ? { ...defaultPolicy(), ...policy } as ActivityPolicy : defaultPolicy(),
			avatar_media_id: persona.avatar_media_id ?? '',
			header_media_id: persona.header_media_id ?? '',
			profession: persona.profession ?? '',
			location: persona.location ?? '',
			website_url: persona.website_url ?? '',
			education: persona.education ?? '',
			tips_enabled: persona.tips_enabled ?? false
		};
		editModalOpen = true;
	}

	function openView(persona: Persona) {
		seedEditForm(persona, true);
	}

	function openEdit(persona: Persona) {
		// BUG-005: include activity_policy in the edit form so it is editable
		// after creation (spec §68 PATCH updates activity_policy).
		seedEditForm(persona, false);
	}

	let statusUpdating = $state(false);

	async function updatePersonaStatus(persona: Persona, status: string) {
		if (statusUpdating) return;
		statusUpdating = true;
		message = '';
		try {
			const result = await api(`/admin/personas/${persona.id}`, { method: 'PATCH', body: { status } });
			if (result.ok) {
				messageType = 'success';
				message = t('personas.status_success', 'Persona status updated to {status}.').replace('{status}', status);
				entityController?.reload();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('personas.update_error', 'Failed to update persona');
		} finally {
			statusUpdating = false;
		}
	}

	// Retire is a destructive lifecycle transition — the DataTable spec puts
	// destructive actions behind a confirmation dialog, so the row action and
	// the bulk action both funnel through this modal.
	let retireConfirmOpen = $state(false);
	let retireIds = $state<Set<string>>(new Set());

	function requestRetire(ids: Iterable<string>) {
		retireIds = new Set(ids);
		retireConfirmOpen = true;
	}

	async function confirmRetire() {
		retireConfirmOpen = false;
		await bulkUpdateStatus(retireIds, 'retired');
		// EntityPage's post-bulk reload fires when the handler returns — the
		// confirm modal defers the PATCHes past that point, so reload here.
		entityController?.reload();
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
			}
			// EntityPage auto-reloads and clears selection after bulk handlers.
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
			onclick: (ids: Set<string>) => requestRetire(ids),
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
			// Send identity fields only when they actually changed — an
			// untouched name would otherwise pin display_name to the
			// username fallback and defeat the COALESCE fallback on later
			// renames (profile_name reads COALESCE(display_name, username)).
			if (body.name === (editingPersona.profile_name ?? '')) delete body.name;
			if (body.tagline === (editingPersona.profile_tagline ?? '')) delete body.tagline;
			if (body.username === (editingPersona.backing_username ?? '')) delete body.username;
			// Profile fields (Plan B): same diff-only-send pattern. Media ids
			// send '' to clear; interests is converted from the comma input —
			// send [] to clear, omit when unchanged.
			if (body.avatar_media_id === (editingPersona.avatar_media_id ?? '')) delete body.avatar_media_id;
			if (body.header_media_id === (editingPersona.header_media_id ?? '')) delete body.header_media_id;
			if (body.profession === (editingPersona.profession ?? '')) delete body.profession;
			if (body.location === (editingPersona.location ?? '')) delete body.location;
			if (body.website_url === (editingPersona.website_url ?? '')) delete body.website_url;
			if (body.education === (editingPersona.education ?? '')) delete body.education;
			const interests = editInterestsText.split(',').map((s) => s.trim()).filter(Boolean);
			if (interests.join(',') !== (editingPersona.interests ?? []).join(',')) body.interests = interests;
			if (body.tips_enabled === (editingPersona.tips_enabled ?? false)) delete body.tips_enabled;
			// Send geo_space only when the admin picked a new chain or hit
			// Clear — the cascade can't pre-seed ancestors, so untouched
			// stays omitted. "" clears the geo assignment (§68).
			if (editGeoCleared) body.geo_space = '';
			else if (editGeoSpace !== '') body.geo_space = editGeoSpace;
			// voice_prompt is always sent as a string (bound to a textarea).
			// An empty string clears it on the backend because the PATCH
			// handler decodes VoicePrompt as *string — empty is non-nil, so
			// COALESCE sets the column to '' rather than keeping the old value.
			const result = await api(`/admin/personas/${editingPersona.id}`, { method: 'PATCH', body });
			if (result.ok) {
				messageType = 'success';
				message = t('personas.update_success', 'Persona updated.');
				editModalOpen = false;
				editingPersona = null;
				entityController?.reload();
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
			key: 'geo_space_name',
			label: t('personas.geo_space', 'Geo Space'),
			render: (value: string, row: Persona) =>
				`<span class="text-muted-foreground text-xs">${value ? `${value}${row.geo_space_level ? ` · ${row.geo_space_level}` : ''}` : '—'}</span>`
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
			onclick: (row: Persona) => requestRetire([row.id]),
			variant: 'destructive',
			show: (row: any) => canManage && row.status !== 'retired'
		}
	];

	// EntityPage config — the scaffold owns the list (server-side table,
	// URL-synced sort/search/page/filters, bulk bar, action menu). The batch
	// create flow and the edit/activity sub-flows stay page-local modals
	// because EntityForm can't express a multi-row batch input or the
	// activity log viewer (per the AGENTS.md rich-page escape hatches).
	const fields: FieldDef[] = [];

	const config = $derived<EntityPageConfig>({
		title: t('personas.title', 'AI Personas'),
		description: t('personas.description', 'Manage AI persona accounts for content generation and participation.'),
		columns,
		filters: [
			{
				key: 'status',
				label: t('personas.status', 'Status'),
				type: 'select',
				options: [
					{ value: '', label: t('personas.all_statuses', 'All statuses') },
					{ value: 'active', label: t('personas.status_active', 'Active') },
					{ value: 'paused', label: t('personas.status_paused', 'Paused') },
					{ value: 'retired', label: t('personas.status_retired', 'Retired') }
				]
			}
		],
		actions: rowActions,
		bulkActions: canManage ? bulkActionsList : [],
		hideCreate: true, // batch create lives in the custom modal below
		bulkDelete: false, // personas retire, never soft-delete (§68)
		editPermission: 'persona.manage',
		fields,
		pageSize: 10,
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams();
			params.set('page_size', String(pageSize));
			params.set('page_offset', String(page * pageSize));
			const status = (filter?.status as string) || '';
			if (status) params.set('status', status);
			if (q) params.set('q', q);
			// EntityPage emits sort as "key:dir"; backend expects them separate.
			const [sortKey, sortDir] = sort ? sort.split(':') : [];
			if (sortKey) params.set('sort', sortKey);
			if (sortDir) params.set('dir', sortDir);
			const result = await apiRaw<{ data: Persona[]; meta: { total: number } }>(
				`/admin/personas?${params.toString()}`
			);
			const data = result.ok ? (result.data?.data ?? []) : [];
			const total = result.ok ? (result.data?.meta?.total ?? data.length) : 0;
			return { data, total, totalPages: Math.max(1, Math.ceil(total / pageSize)) };
		}
	});

	loadPersonaVoices();
</script>

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

{#if canManage}
	<div class="mb-3 flex justify-end">
		<Button onclick={() => (createModalOpen = true)}>
			<Plus class="h-4 w-4" /> {t('personas.create', 'Create Personas')}
		</Button>
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

{#if createModalOpen}
	<Modal
		bind:open={createModalOpen}
		title={t('personas.create_title', 'Create AI Personas')}
	>
		<div class="space-y-4">
			{#if message && messageType === 'error'}
				<div
					class="rounded-lg border p-3 text-sm"
					style="border-color: var(--color-danger); background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
				>
					{message}
				</div>
			{/if}
			<p class="text-sm" style="color: var(--color-muted)">
				{t('personas.create_hint', 'Provision one or more AI personas. All personas in a batch are created in a single transaction.')}
			</p>
			{#each newPersonas as persona, idx (persona)}
				<div class="rounded-lg border p-3" style="border-color: var(--color-border)">
					<div class="mb-2 flex items-center justify-between gap-2 flex-wrap">
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
					<div class="mt-2">
						<label for={`persona-geo-${idx}-country`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.geo_space', 'Geo Space')}</label>
						<GeoSpaceCascade id={`persona-geo-${idx}`} bind:value={persona.geo_space} />
					</div>
					<div class="mt-2 grid gap-3 sm:grid-cols-2">
						<div>
							<label for={`persona-disclosure-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.disclosure', 'Disclosure Mode')}</label>
							<AutocompleteSelect
								id={`persona-disclosure-${idx}`}
								bind:value={persona.disclosure_mode}
								options={[{ value: 'disclosed', label: t('personas.disclosure_disclosed', 'Disclosed') }, { value: 'undisclosed', label: t('personas.disclosure_undisclosed', 'Undisclosed') }]}
								class="w-full"
							/>
						</div>
						<div>
							<label for={`persona-autonomy-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.autonomy', 'Autonomy Mode')}</label>
							<AutocompleteSelect
								id={`persona-autonomy-${idx}`}
								bind:value={persona.autonomy_mode}
								options={[{ value: 'queued', label: t('personas.autonomy_queued', 'Queued (admin review)') }, { value: 'autonomous', label: t('personas.autonomy_autonomous', 'Autonomous') }]}
								class="w-full"
							/>
						</div>
					</div>
					<div class="mt-2">
						<label for={`persona-voice-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.voice', 'Voice / Tone')}</label>
						{#if voicesLoading}
							<p class="text-xs" style="color: var(--color-muted)">{t('personas.loading_voices', 'Loading voices...')}</p>
						{:else if voicesError}
							<p class="text-xs" style="color: var(--color-danger)">{voicesError}</p>
						{:else}
							<AutocompleteSelect
								id={`persona-voice-${idx}`}
								bind:value={persona.voice_id}
								options={[{ value: '', label: t('personas.voice_none', 'Create new voice from prompt below') }, ...voiceOptions.map((voice) => ({ value: voice.id, label: voice.name }))]}
								class="w-full"
							/>
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
								<JsonValueEditor
									id={`persona-action-budget-${idx}`}
									value={JSON.stringify(persona.activity_policy!.action_budget ?? {})}
									onchange={(json) => {
										try {
											persona.activity_policy!.action_budget = json.trim() ? JSON.parse(json) : {};
										} catch { /* leave unchanged on parse error */ }
									}}
									placeholder={'{"comment": 4, "react": 20}'}
								/>
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
				<Button variant="outline" onclick={() => (createModalOpen = false)}>{t('common.cancel', 'Cancel')}</Button>
				<Button onclick={createPersonas} disabled={saving}>
					{saving ? t('common.saving', 'Saving...') : t('personas.provision', 'Provision Personas')}
				</Button>
			</div>
		{/snippet}
	</Modal>
{/if}

{#if activityModalOpen}
	<Modal
		bind:open={activityModalOpen}
		title={t('personas.activity_log', 'Persona Activity Log')}
	>
		{#if activityLoading}
			<div class="p-4 text-center" style="color: var(--color-muted)">
				<Activity class="mx-auto mb-2 h-6 w-6 animate-pulse" />
				{t('personas.loading_activity', 'Loading activity...')}
			</div>
		{:else if activityError}
			<p class="p-4 text-center text-sm" style="color: var(--color-danger)">{activityError}</p>
		{:else if activityLog.length === 0}
			<p class="p-4 text-center text-sm" style="color: var(--color-muted)">{t('personas.no_activity', 'No activity recorded.')}</p>
		{:else}
			<div class="max-h-96 space-y-2 overflow-y-auto">
				{#each activityLog as entry (entry.id)}
					<div class="rounded-md border p-2 text-xs" style="border-color: var(--color-border); color: var(--color-foreground)">
						<div class="flex items-center justify-between flex-wrap gap-2">
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
				<div class="mt-3 flex items-center justify-between gap-2 flex-wrap">
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
	</Modal>
{/if}

{#if retireConfirmOpen}
	<Modal
		bind:open={retireConfirmOpen}
		title={t('personas.retire_title', 'Retire Persona')}
	>
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('personas.retire_confirm', 'Retire {count} persona(s)? Retired personas stop all autonomous and queued activity. This can be reversed by editing the persona later.').replace('{count}', String(retireIds.size))}
		</p>
		{#snippet footer()}
			<div class="flex justify-end gap-2">
				<Button variant="outline" onclick={() => (retireConfirmOpen = false)}>{t('common.cancel', 'Cancel')}</Button>
				<Button variant="destructive" onclick={confirmRetire} disabled={bulkProcessing}>
					{bulkProcessing ? t('common.saving', 'Saving...') : t('personas.bulk_retire', 'Retire')}
				</Button>
			</div>
		{/snippet}
	</Modal>
{/if}

{#if editModalOpen}
	<Modal
		bind:open={editModalOpen}
		title={editViewOnly ? t('personas.view_title', 'View Persona') : t('personas.edit_title', 'Edit Persona')}
		onclose={() => { editingPersona = null; editViewOnly = false; }}
	>
		{#if message && messageType === 'error'}
			<div
				class="mb-3 rounded-lg border p-3 text-sm"
				style="border-color: var(--color-danger); background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
			>
				{message}
			</div>
		{/if}
		<fieldset class="space-y-4 border-0 p-0 m-0" disabled={editViewOnly}>
			<div class="flex items-center justify-between gap-3">
				<div>
					<label for="edit-persona-id" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">ID</label>
					<p class="font-mono text-xs" style="color: var(--color-foreground)">{editingPersona?.id}</p>
				</div>
				<div class="text-right">
					<p class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.points_balance', 'Points')}</p>
					<p class="text-sm font-semibold" style="color: var(--color-foreground)">{(editingPersona?.points_balance ?? 0).toLocaleString()}</p>
				</div>
			</div>
			<div class="grid gap-3 sm:grid-cols-2">
				<div>
					<label for="edit-persona-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.name_label', 'Name')}</label>
					<input
						id="edit-persona-name"
						type="text"
						class="w-full rounded-md border px-3 py-1.5 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={editForm.name}
						placeholder={t('personas.name_placeholder', 'e.g. Adaora Okafor')}
					/>
				</div>
				<div>
					<label for="edit-persona-username" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.username', 'Username')}</label>
					<input
						id="edit-persona-username"
						type="text"
						class="w-full rounded-md border px-3 py-1.5 text-sm font-mono"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={editForm.username}
						placeholder={t('personas.slug_placeholder', 'e.g. adaora-okafor')}
					/>
					<p class="mt-1 text-[10px]" style="color: var(--color-muted)">{t('personas.username_hint', 'Renames the backing bot handle — e.g. intel-visa → a custom slug.')}</p>
				</div>
			</div>
			<div>
				<label for="edit-persona-tagline" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.tagline', 'Tagline')}</label>
				<input
					id="edit-persona-tagline"
					type="text"
					class="w-full rounded-md border px-3 py-1.5 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					bind:value={editForm.tagline}
					placeholder={t('personas.tagline_placeholder', 'Short bio / expertise summary')}
				/>
			</div>
			<div class="grid gap-3 sm:grid-cols-2">
				<div>
					<p class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.avatar', 'Avatar')}</p>
					<div class="flex items-center gap-2">
						{#if editAvatarUrl}
							<img src={editAvatarUrl} alt="" class="h-12 w-12 rounded-full border object-cover" style="border-color: var(--color-border)" />
						{:else}
							<div class="grid h-12 w-12 place-items-center rounded-full border" style="border-color: var(--color-border); background: var(--color-surface)">
								<ImageIcon class="h-5 w-5" style="color: var(--color-muted)" />
							</div>
						{/if}
						<div class="flex flex-col gap-1">
							<Button type="button" variant="outline" size="sm" onclick={() => (avatarPickerOpen = true)}>
								<ImageIcon class="me-1 h-4 w-4" />
								{t('personas.choose_avatar', 'Choose Avatar')}
							</Button>
							{#if editForm.avatar_media_id || editAvatarUrl}
								<button
									type="button"
									class="text-left text-[10px] underline"
									style="color: var(--color-danger)"
									onclick={() => { editForm.avatar_media_id = ''; editAvatarUrl = ''; }}
								>
									{t('personas.remove_avatar', 'Remove')}
								</button>
							{/if}
						</div>
					</div>
				</div>
				<div>
					<p class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.cover_photo', 'Cover Photo')}</p>
					<div class="flex items-center gap-2">
						{#if editCoverUrl}
							<img src={editCoverUrl} alt="" class="h-12 w-20 rounded-lg border object-cover" style="border-color: var(--color-border)" />
						{:else}
							<div class="grid h-12 w-20 place-items-center rounded-lg border" style="border-color: var(--color-border); background: var(--color-surface)">
								<ImageIcon class="h-5 w-5" style="color: var(--color-muted)" />
							</div>
						{/if}
						<div class="flex flex-col gap-1">
							<Button type="button" variant="outline" size="sm" onclick={() => (coverPickerOpen = true)}>
								<ImageIcon class="me-1 h-4 w-4" />
								{t('personas.choose_cover', 'Choose Cover')}
							</Button>
							{#if editForm.header_media_id || editCoverUrl}
								<button
									type="button"
									class="text-left text-[10px] underline"
									style="color: var(--color-danger)"
									onclick={() => { editForm.header_media_id = ''; editCoverUrl = ''; }}
								>
									{t('personas.remove_cover', 'Remove')}
								</button>
							{/if}
						</div>
					</div>
				</div>
			</div>
			<div class="grid gap-3 sm:grid-cols-2">
				<div>
					<label for="edit-persona-profession" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.profession', 'Profession')}</label>
					<input
						id="edit-persona-profession"
						type="text"
						class="w-full rounded-md border px-3 py-1.5 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={editForm.profession}
						placeholder={t('personas.profession_placeholder', 'e.g. Immigration Consultant')}
					/>
				</div>
				<div>
					<label for="edit-persona-location" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.location', 'Location')}</label>
					<input
						id="edit-persona-location"
						type="text"
						class="w-full rounded-md border px-3 py-1.5 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={editForm.location}
						placeholder={t('personas.location_placeholder', 'e.g. London, UK')}
					/>
				</div>
				<div>
					<label for="edit-persona-website" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.website_url', 'Website')}</label>
					<input
						id="edit-persona-website"
						type="url"
						class="w-full rounded-md border px-3 py-1.5 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={editForm.website_url}
						placeholder="https://"
					/>
				</div>
				<div>
					<label for="edit-persona-education" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.education', 'Education')}</label>
					<input
						id="edit-persona-education"
						type="text"
						class="w-full rounded-md border px-3 py-1.5 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={editForm.education}
						placeholder={t('personas.education_placeholder', 'e.g. MSc, University of Lagos')}
					/>
				</div>
			</div>
			<div>
				<label for="edit-persona-interests" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.interests', 'Interests (comma-separated)')}</label>
				<input
					id="edit-persona-interests"
					type="text"
					class="w-full rounded-md border px-3 py-1.5 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					bind:value={editInterestsText}
					placeholder={t('personas.interests_placeholder', 'e.g. visas, fintech, community')}
				/>
			</div>
			<div class="flex items-center gap-2">
				<input
					id="edit-persona-tips"
					type="checkbox"
					class="rounded"
					bind:checked={editForm.tips_enabled}
				/>
				<label for="edit-persona-tips" class="text-xs" style="color: var(--color-muted)">{t('personas.tips_enabled', 'Accept tips on this persona\'s profile')}</label>
			</div>
			<div class="grid gap-3 sm:grid-cols-2">
				<div>
					<label for="edit-persona-disclosure" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.disclosure', 'Disclosure Mode')}</label>
					<AutocompleteSelect
						id="edit-persona-disclosure"
						bind:value={editForm.disclosure_mode}
						options={[{ value: 'disclosed', label: t('personas.disclosure_disclosed', 'Disclosed') }, { value: 'undisclosed', label: t('personas.disclosure_undisclosed', 'Undisclosed') }]}
						class="w-full"
					/>
				</div>
				<div>
					<label for="edit-persona-autonomy" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.autonomy', 'Autonomy Mode')}</label>
					<AutocompleteSelect
						id="edit-persona-autonomy"
						bind:value={editForm.autonomy_mode}
						options={[{ value: 'queued', label: t('personas.autonomy_queued', 'Queued (admin review)') }, { value: 'autonomous', label: t('personas.autonomy_autonomous', 'Autonomous') }]}
						class="w-full"
					/>
				</div>
			</div>
			<div>
				<label for="edit-persona-status" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.status', 'Status')}</label>
				<AutocompleteSelect
					id="edit-persona-status"
					bind:value={editForm.status}
					options={[{ value: 'active', label: t('personas.status_active', 'Active') }, { value: 'paused', label: t('personas.status_paused', 'Paused') }, { value: 'retired', label: t('personas.status_retired', 'Retired') }]}
					class="w-full"
				/>
			</div>
			<div>
				<label for="edit-persona-geo-country" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.geo_space', 'Geo Space')}</label>
				{#if editingPersona?.geo_space_name && !editGeoCleared}
					<p class="mb-1 text-[10px]" style="color: var(--color-muted)">
						{t('personas.geo_current', 'Current')}: {editingPersona.geo_space_name}{editingPersona.geo_space_level ? ` · ${editingPersona.geo_space_level}` : ''}
						<button type="button" class="underline" style="color: var(--color-danger)" onclick={() => { editGeoCleared = true; editGeoSpace = ''; }}>
							{t('personas.geo_clear', 'Clear')}
						</button>
					</p>
				{:else if editGeoCleared}
					<p class="mb-1 text-[10px]" style="color: var(--color-danger)">
						{t('personas.geo_will_clear', 'Geo space will be cleared on save.')}
						<button type="button" class="underline" style="color: var(--color-muted)" onclick={() => (editGeoCleared = false)}>
							{t('common.undo', 'Undo')}
						</button>
					</p>
				{/if}
				{#if !editGeoCleared}
					<GeoSpaceCascade id="edit-persona-geo" bind:value={editGeoSpace} />
				{/if}
			</div>
			<div>
				<label for="edit-persona-voice" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.voice', 'Voice / Tone')}</label>
				{#if voicesLoading}
					<p class="text-xs" style="color: var(--color-muted)">{t('personas.loading_voices', 'Loading voices...')}</p>
				{:else if voicesError}
					<p class="text-xs" style="color: var(--color-danger)">{voicesError}</p>
				{:else}
					<AutocompleteSelect
						id="edit-persona-voice"
						bind:value={editForm.voice_id}
						options={[{ value: '', label: t('personas.voice_none', 'No shared voice') }, ...voiceOptions.map((voice) => ({ value: voice.id, label: voice.name }))]}
						class="w-full"
					/>
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
							<JsonValueEditor
								id="edit-persona-action-budget"
								value={JSON.stringify(editForm.activity_policy.action_budget ?? {})}
								onchange={(json) => {
									try {
										editForm.activity_policy!.action_budget = json.trim() ? JSON.parse(json) : {};
									} catch { /* leave unchanged on parse error */ }
								}}
								placeholder={'{"comment": 4, "react": 20}'}
							/>
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
					<Button onclick={() => (editModalOpen = false)}>{t('common.close', 'Close')}</Button>
				{:else}
					<Button variant="outline" onclick={() => (editModalOpen = false)}>{t('common.cancel', 'Cancel')}</Button>
					<Button onclick={submitEdit} disabled={savingEdit}>
						{savingEdit ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
					</Button>
				{/if}
			</div>
		{/snippet}
	</Modal>
{/if}

<MediaPickerModal
	bind:open={avatarPickerOpen}
	onselect={(media: { id: string; url: string }) => {
		editForm.avatar_media_id = media.id;
		editAvatarUrl = media.url;
	}}
/>
<MediaPickerModal
	bind:open={coverPickerOpen}
	onselect={(media: { id: string; url: string }) => {
		editForm.header_media_id = media.id;
		editCoverUrl = media.url;
	}}
/>
