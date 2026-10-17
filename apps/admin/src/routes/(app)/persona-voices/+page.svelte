<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /persona-voices — shared persona voice / tone definitions (ADR-114, §68).
	import { EntityPage, type EntityPageConfig, type FieldDef } from '$lib/components/entity';
	import { personaVoicesAdminApi, type PersonaVoice } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

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

	const config: EntityPageConfig = {
		title: t('persona_voices.title', 'Persona Voices'),
		description: t('persona_voices.description', 'Manage shared voice and tone definitions for AI personas and content generation.'),
		createLabel: t('persona_voices.create', 'Create Voice'),
		createPermission: 'persona.manage',
		editPermission: 'persona.manage',
		deletePermission: 'persona.manage',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('persona_voices.name', 'Name') },
			{
				key: 'slug',
				label: t('persona_voices.slug', 'Slug'),
				render: (v: string) => `<span class="font-mono text-xs text-muted-foreground">${v ?? '—'}</span>`
			},
			{
				key: 'voice_prompt',
				label: t('persona_voices.prompt_preview', 'Prompt preview'),
				render: (v: string) => `<span class="text-muted-foreground text-xs truncate">${v ? (v.length > 80 ? v.slice(0, 80) + '…' : v) : '—'}</span>`
			},
			{
				key: 'created_at',
				label: t('persona_voices.created', 'Created'),
				render: (v: string) => `<span class="text-muted-foreground text-xs">${v ? new Date(v).toLocaleDateString() : '—'}</span>`
			},
			{
				key: 'updated_at',
				label: t('persona_voices.updated', 'Updated'),
				render: (v: string) => `<span class="text-muted-foreground text-xs">${v ? new Date(v).toLocaleDateString() : '—'}</span>`
			}
		],
		fields,
		load: async ({ page, pageSize, q }) => {
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			if (pageSize) {
				params.set('page_size', String(pageSize));
				params.set('page_offset', String(page * pageSize));
			}
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
				const d = String(body.description ?? '').trim();
				payload.description = d || null;
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
