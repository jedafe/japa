<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	// /persona-voices — shared persona voice / tone definitions (ADR-114, §68).
	import { EntityPage, type EntityPageConfig, type FieldDef } from '#lib/components/entity/index.js';
	import type { ActionItem } from '#lib/components/ui/data-table/index.js';
	import { Eye, Pencil, Trash2 } from 'lucide-svelte';
	import { personaVoicesAdminApi, type PersonaVoice } from '#lib/api.js';
	import { t } from '#lib/stores/i18n.svelte.js';

	const fields: FieldDef[] = [
		{
			key: 'name',
			label: t('persona_voices.name', 'Name'),
			type: 'text',
			required: true,
			placeholder: 'e.g. Professional Travel Blogger'
		},
		{
			key: 'slug',
			label: t('persona_voices.slug', 'Slug'),
			type: 'text',
			required: true,
			placeholder: 'e.g. professional-travel-blogger'
		},
		{
			key: 'description',
			label: t('persona_voices.description_field', 'Description'),
			type: 'textarea',
			placeholder: 'Short summary of this voice'
		},
		{
			key: 'voice_prompt',
			label: t('persona_voices.voice_prompt', 'Voice Prompt'),
			type: 'textarea',
			required: true,
			placeholder: t('persona_voices.voice_prompt_placeholder', 'Describe the voice, personality, and style...')
		},
		{
			key: 'is_system',
			label: t('persona_voices.is_system', 'System voice'),
			type: 'checkbox',
			help: 'System voices are reserved for platform-managed defaults.'
		}
	];

	// System voices are platform-managed defaults (ADR-114) — the backend
	// rejects DELETE on them, so the row action is hidden rather than left
	// to 403.
	const rowActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row, ctx) => ctx?.openView(row) },
		{ label: t('common.edit', 'Edit'), icon: Pencil, onclick: (row, ctx) => ctx?.openEdit(row) },
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row, ctx) => ctx?.openDelete(row),
			show: (row: PersonaVoice) => !row.is_system
		}
	];

	const config: EntityPageConfig = {
		title: t('persona_voices.title', 'Persona Voices'),
		description: t('persona_voices.description', 'Manage shared voice and tone definitions for AI personas and content generation.'),
		createLabel: t('persona_voices.create', 'Create Voice'),
		createPermission: 'persona.manage',
		editPermission: 'persona.manage',
		deletePermission: 'persona.manage',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('persona_voices.name', 'Name'), sortable: true },
			{
				key: 'slug',
				label: t('persona_voices.slug', 'Slug'),
				sortable: true,
				render: (v: string) => `<span class="font-mono text-xs text-muted-foreground">${v ?? '—'}</span>`
			},
			{
				key: 'voice_prompt',
				label: t('persona_voices.prompt_preview', 'Prompt preview'),
				render: (v: string) => `<span class="text-muted-foreground text-xs truncate">${v ? (v.length > 80 ? v.slice(0, 80) + '…' : v) : '—'}</span>`
			},
			{
				key: 'is_system',
				label: t('persona_voices.is_system', 'System'),
				render: (v: boolean) =>
					v
						? `<span class="bg-muted text-muted-foreground border-border inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium">${t('persona_voices.system', 'system')}</span>`
						: '<span class="text-muted-foreground text-xs">—</span>'
			},
			{
				key: 'created_at',
				label: t('persona_voices.created', 'Created'),
				sortable: true,
				render: (v: string) => `<span class="text-muted-foreground text-xs">${v ? new Date(v).toLocaleDateString() : '—'}</span>`
			},
			{
				key: 'updated_at',
				label: t('persona_voices.updated', 'Updated'),
				sortable: true,
				render: (v: string) => `<span class="text-muted-foreground text-xs">${v ? new Date(v).toLocaleDateString() : '—'}</span>`
			}
		],
		actions: rowActions,
		fields,
		load: async ({ page, pageSize, q, sort }) => {
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			if (pageSize) {
				params.set('page_size', String(pageSize));
				params.set('page_offset', String(page * pageSize));
			}
			// EntityPage emits sort as "key:dir"; backend expects them separate.
			const [sortKey, sortDir] = sort ? sort.split(':') : [];
			if (sortKey) params.set('sort', sortKey);
			if (sortDir) params.set('dir', sortDir);
			const result = await personaVoicesAdminApi.list(params.toString() || undefined);
			if (result.ok) {
				const body = result.data;
				const data = body?.data ?? [];
				const total = body?.meta?.total ?? data.length;
				const totalPages = pageSize ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('persona_voices.load_error', 'Failed to load voices'));
		},
		create: async (body) => {
			const res = await personaVoicesAdminApi.create({
				name: String(body.name ?? '').trim(),
				slug: String(body.slug ?? '').trim(),
				description: String(body.description ?? '').trim() || undefined,
				voice_prompt: String(body.voice_prompt ?? '').trim(),
				is_system: !!body.is_system
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const payload: Record<string, unknown> = {};
			if (body.name !== undefined) payload.name = String(body.name ?? '').trim();
			if (body.slug !== undefined) payload.slug = String(body.slug ?? '').trim();
			if (body.description !== undefined) {
				// Send the raw string — the backend treats null as "unchanged" and
				// "" as "clear", so null here would make the field un-clearable.
				payload.description = String(body.description ?? '').trim();
			}
			if (body.voice_prompt !== undefined) payload.voice_prompt = String(body.voice_prompt ?? '').trim();
			if (body.is_system !== undefined) payload.is_system = !!body.is_system;
			const res = await personaVoicesAdminApi.update(id, payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await personaVoicesAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		}
	};
</script>

<svelte:head>
	<title>{t('persona_voices.title', 'Persona Voices')} — {siteName()} Admin</title>
</svelte:head>

<EntityPage {config} />
