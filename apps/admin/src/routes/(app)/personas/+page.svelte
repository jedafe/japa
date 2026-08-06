<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { Bot, Plus, Trash2, Activity, Pencil } from 'lucide-svelte';
	import { api } from '$lib/api';

	type Persona = {
		id: string;
		user_id: string;
		profile_id: string;
		disclosure_mode: string;
		autonomy_mode: string;
		voice_prompt: string | null;
		status: string;
		created_at: string;
		updated_at: string;
	};

	type PersonaInput = {
		disclosure_mode: string;
		autonomy_mode: string;
		voice_prompt?: string;
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
	let saving = $state(false);
	let message = $state('');
	let messageType = $state('');
	let newPersonas = $state<PersonaInput[]>([{ disclosure_mode: 'always', autonomy_mode: 'supervised', voice_prompt: '' }]);

	// Edit SlideOver
	let editSlideOverOpen = $state(false);
	let editingPersona = $state<Persona | null>(null);
	let editForm = $state<PersonaInput>({ disclosure_mode: 'always', autonomy_mode: 'supervised', voice_prompt: '' });
	let savingEdit = $state(false);

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await api<Persona[]>('/admin/personas');
			if (result.ok) {
				personas = result.data ?? [];
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
		activitySlideOverOpen = true;
		activityLoading = true;
		try {
			const result = await api<unknown[]>(`/admin/personas/${id}/activity`);
			if (result.ok) {
				activityLog = result.data ?? [];
			}
		} catch {
			activityLog = [];
		} finally {
			activityLoading = false;
		}
	}

	async function createPersonas() {
		saving = true;
		message = '';
		try {
			const inputs = newPersonas.filter(p => p.disclosure_mode && p.autonomy_mode);
			if (inputs.length === 0) {
				messageType = 'error';
				message = t('personas.empty_batch', 'At least one persona is required');
				saving = false;
				return;
			}
			const result = await api('/admin/personas', { method: 'POST', body: inputs });
			if (result.ok) {
				messageType = 'success';
				message = t('personas.create_success', '{count} persona(s) provisioned.').replace('{count}', String(inputs.length));
				createSlideOverOpen = false;
				newPersonas = [{ disclosure_mode: 'always', autonomy_mode: 'supervised', voice_prompt: '' }];
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
		newPersonas = [...newPersonas, { disclosure_mode: 'always', autonomy_mode: 'supervised', voice_prompt: '' }];
	}

	function removePersonaRow(idx: number) {
		newPersonas = newPersonas.filter((_, i) => i !== idx);
	}

	function openEdit(persona: Persona) {
		editingPersona = persona;
		editForm = {
			disclosure_mode: persona.disclosure_mode,
			autonomy_mode: persona.autonomy_mode,
			voice_prompt: persona.voice_prompt ?? ''
		};
		editSlideOverOpen = true;
	}

	async function submitEdit() {
		if (!editingPersona) return;
		savingEdit = true;
		message = '';
		try {
			const result = await api(`/admin/personas/${editingPersona.id}`, { method: 'PATCH', body: editForm });
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
			key: 'id',
			label: t('personas.id', 'ID'),
			sortable: true,
			render: (value: string) => `<span class="font-mono text-xs text-foreground">${value?.slice(0, 8) ?? '—'}…</span>`
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
	<DataTable
		data={personas}
		{columns}
		actions={rowActions}
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
							<label for={`persona-disclosure-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.disclosure', 'Disclosure Mode')}</label>
							<select
								id={`persona-disclosure-${idx}`}
								class="w-full rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={persona.disclosure_mode}
							>
								<option value="always">{t('personas.disclosure_always', 'Always disclose')}</option>
								<option value="never">{t('personas.disclosure_never', 'Never disclose')}</option>
								<option value="on_ask">{t('personas.disclosure_on_ask', 'Disclose on ask')}</option>
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
								<option value="supervised">{t('personas.autonomy_supervised', 'Supervised')}</option>
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
						<option value="always">{t('personas.disclosure_always', 'Always disclose')}</option>
						<option value="never">{t('personas.disclosure_never', 'Never disclose')}</option>
						<option value="on_ask">{t('personas.disclosure_on_ask', 'Disclose on ask')}</option>
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
						<option value="supervised">{t('personas.autonomy_supervised', 'Supervised')}</option>
						<option value="autonomous">{t('personas.autonomy_autonomous', 'Autonomous')}</option>
					</select>
				</div>
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
