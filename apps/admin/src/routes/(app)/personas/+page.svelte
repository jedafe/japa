<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { Bot, Plus, Trash2, Activity, Pencil } from 'lucide-svelte';
	import { api, apiRaw } from '$lib/api';

	// Persona type matches the backend PersonaDetail / PersonaListItem DTOs
	// (internal/personas/service.go). Ref: §68, ADR-068.
	type Persona = {
		id: string;
		user_id: string;
		profile_id: string;
		disclosure_mode: string; // 'disclosed' | 'undisclosed' (§68 schema CHECK)
		autonomy_mode: string;   // 'queued' | 'autonomous' (§68 schema CHECK)
		voice_prompt: string | null;
		activity_policy: Record<string, unknown>;
		status: string;          // 'active' | 'paused' | 'retired' (§68 schema CHECK)
		created_at: string;
		updated_at: string;
		backing_username?: string;
		profile_name?: string;
	};

	// PersonaInput matches the backend PersonaInput struct. The backend
	// requires name and slug (service.go validates both). Ref: §68.
	type PersonaInput = {
		name: string;
		slug: string;
		tagline?: string;
		voice_prompt?: string;
		disclosure_mode: string;
		autonomy_mode: string;
		activity_policy?: ActivityPolicy;
	};

	// ActivityPolicy matches the backend ActivityPolicy struct (§68).
	type ActivityPolicy = {
		actions_per_day: number;
		action_budget: Record<string, number>;
		eligible_content_types: string[];
		eligible_spaces: string[];
		min_content_age_hours: number;
		avoid_own_content: boolean;
		active_hours_utc: [number, number];
	};

	// UpdateInput matches the backend UpdatePersonaInput. Status is included
	// so admins can pause/retire personas from the UI.
	type UpdateInput = {
		disclosure_mode?: string;
		autonomy_mode?: string;
		voice_prompt?: string;
		status?: string;
		activity_policy?: ActivityPolicy;
	};

	const canManage = $derived(hasPermission('persona.manage'));
	const canView = $derived(hasPermission('persona.view'));

	let personas = $state<Persona[]>([]);
	let loading = $state(true);
	let error = $state('');
	let createSlideOverOpen = $state(false);
	let activitySlideOverOpen = $state(false);
	let activityLog = $state<unknown[]>([]);
	let activityLoading = $state(false);
	let activityTotal = $state(0);
	let activityPage = $state(0);
	let activityPageSize = $state(10);
	let activityPersonaId = $state('');
	let saving = $state(false);
	let message = $state('');
	let messageType = $state('');

	// Server-side pagination state (BUG-003: list was not paginated — only 20
	// personas loaded with no way to see more).
	let listPage = $state(0);
	let listPageSize = $state(10);
	let listTotal = $state(0);
	let listTotalPages = $state(1);
	let statusFilter = $state('');

	function defaultPolicy(): ActivityPolicy {
		return {
			actions_per_day: 10,
			action_budget: { comment: 4, forum_answer: 2, forum_question: 1, follow: 5, react: 20, bookmark: 10 },
			eligible_content_types: ['blog_post', 'guide', 'discussion', 'forum_thread'],
			eligible_spaces: [],
			min_content_age_hours: 1,
			avoid_own_content: true,
			active_hours_utc: [8, 22]
		};
	}

	function newPersonaInput(): PersonaInput {
		return {
			name: '',
			slug: '',
			tagline: '',
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
	let savingEdit = $state(false);

	async function load() {
		loading = true;
		error = '';
		try {
			// BUG-003: use apiRaw to access top-level meta.total alongside data,
			// and send page/limit params for server-side pagination.
			const params = new URLSearchParams();
			params.set('limit', String(listPageSize));
			params.set('page', String(listPage + 1));
			if (statusFilter) params.set('status', statusFilter);
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
	// way to see more. Now fetches with page/limit and supports pagination.
	async function loadActivity() {
		activityLoading = true;
		try {
			const params = new URLSearchParams();
			params.set('limit', String(activityPageSize));
			params.set('page', String(activityPage + 1));
			const result = await apiRaw<{ data: unknown[]; meta: { total: number } }>(
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

	function openEdit(persona: Persona) {
		editingPersona = persona;
		// BUG-005: include activity_policy in the edit form so it is editable
		// after creation (spec §68 PATCH updates activity_policy).
		const policy = persona.activity_policy as ActivityPolicy | undefined;
		editForm = {
			disclosure_mode: persona.disclosure_mode,
			autonomy_mode: persona.autonomy_mode,
			voice_prompt: persona.voice_prompt ?? '',
			status: persona.status,
			activity_policy: policy ?? defaultPolicy()
		};
		editSlideOverOpen = true;
	}

	async function submitEdit() {
		if (!editingPersona) return;
		savingEdit = true;
		message = '';
		try {
			const body: UpdateInput = { ...editForm };
			// Don't send empty voice_prompt as an update
			if (body.voice_prompt === '') delete body.voice_prompt;
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
			label: t('personas.name', 'Name'),
			sortable: true,
			render: (value: string) => `<span class="text-foreground font-medium">${value ?? '—'}</span>`
		},
		{
			key: 'backing_username',
			label: t('personas.username', 'Username'),
			sortable: true,
			render: (value: string) => `<span class="font-mono text-xs text-muted-foreground">${value ?? '—'}</span>`
		},
		{
			key: 'disclosure_mode',
			label: t('personas.disclosure', 'Disclosure'),
			sortable: true,
			render: (value: string) => `<span class="text-foreground">${value}</span>`
		},
		{
			key: 'autonomy_mode',
			label: t('personas.autonomy', 'Autonomy'),
			sortable: true,
			render: (value: string) => `<span class="text-foreground">${value}</span>`
		},
		{
			key: 'status',
			label: t('personas.status', 'Status'),
			sortable: true,
			render: (value: string) => {
				const cls = value === 'active' ? 'bg-success/15 text-success border-success' : 'bg-muted text-muted-foreground border-border';
				return `<span class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${cls}">${value}</span>`;
			}
		},
		{
			key: 'created_at',
			label: t('personas.created', 'Created'),
			sortable: true,
			render: (value: string) => `<span class="text-muted-foreground text-xs">${value ? new Date(value).toLocaleDateString() : '—'}</span>`
		}
	]);

	const rowActions: ActionItem[] = [
		{ label: t('personas.activity_log', 'Activity'), icon: Activity, onclick: (row: Persona) => viewActivity(row.id) },
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row: Persona) => openEdit(row),
			show: () => canManage
		}
	];

	load();
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
			onchange={(e) => { statusFilter = (e.target as HTMLSelectElement).value; listPage = 0; load(); }}
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
		page={listPage}
		totalPages={listTotalPages}
		pageSize={listPageSize}
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
			{#each newPersonas as persona, idx}
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
						<label for={`persona-voice-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.voice_prompt', 'Voice Prompt (optional)')}</label>
						<textarea
							id={`persona-voice-${idx}`}
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
									min="1"
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
				{#each activityLog as entry}
					<div class="rounded-md border p-2 text-xs" style="border-color: var(--color-border); color: var(--color-foreground)">
						{JSON.stringify(entry, null, 2)}
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
		title={t('personas.edit_title', 'Edit Persona')}
	>
		<div class="space-y-4">
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
				<label for="edit-persona-voice" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.voice_prompt', 'Voice Prompt (optional)')}</label>
				<textarea
					id="edit-persona-voice"
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
								min="1"
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
		</div>
		{#snippet footer()}
			<div class="flex justify-end gap-2">
				<Button variant="outline" onclick={() => (editSlideOverOpen = false)}>{t('common.cancel', 'Cancel')}</Button>
				<Button onclick={submitEdit} disabled={savingEdit}>
					{savingEdit ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
				</Button>
			</div>
		{/snippet}
	</SlideOver>
{/if}
