<script lang="ts">
	// /spam — migrated to the EntityPage scaffold. This page has 3 distinct entity
	// types (ip-blocks, censored-words, user-blocks) presented in tabs; each tab
	// renders its own `<EntityPage>` with a config specific to that entity type.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Tabs } from '$lib/components/ui/tabs';
	import { spamApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { datetimeLocalToPostgresTimestamptz } from '$lib/utils/date';
	import { Ban, MessageSquareOff, Users, Trash2, Eye } from 'lucide-svelte';

	type IpBlock = {
		id: string;
		cidr: string;
		block_type: string;
		reason: string | null;
		expires_at?: string | null;
		created_at?: string;
	};
	type CensoredWord = {
		id: string;
		pattern: string;
		action: string;
		replacement: string | null;
		created_at?: string;
	};
	type UserBlock = {
		id: string; // composite: blockerId:blockedId
		blocker_id: string;
		blocker_username?: string;
		blocker_display_name?: string;
		blocked_id: string;
		blocked_username?: string;
		blocked_display_name?: string;
		created_at?: string;
	};

	type Tab = 'ip-blocks' | 'censored-words' | 'user-blocks';

	const PERMISSION = 'spam.ip_block.manage';

	let activeTab = $state<Tab>('ip-blocks');

	const tabs = $derived([
		{ value: 'ip-blocks', label: t('spam.ip_blocks', 'IP Blocks'), icon: Ban },
		{ value: 'censored-words', label: t('spam.censored_words', 'Censored Words'), icon: MessageSquareOff },
		{ value: 'user-blocks', label: t('spam.user_blocks', 'User Blocks'), icon: Users }
	]);

	const blockTypeOptions = [
		{ value: 'full', label: t('spam.block_type_full', 'Full Block') },
		{ value: 'registration', label: t('spam.block_type_registration', 'Registration') },
		{ value: 'posting', label: t('spam.block_type_posting', 'Posting') },
		{ value: 'api', label: t('spam.block_type_api', 'API') }
	];

	const actionOptions = [
		{ value: 'block', label: t('spam.action_block', 'Block') },
		{ value: 'replace', label: t('spam.action_replace', 'Replace') },
		{ value: 'flag', label: t('spam.action_flag', 'Flag') }
	];

	// ── Bulk-delete in-flight guards (one per mutable tab) ───────────────────
	let ipController = $state<EntityPageController | undefined>(undefined);
	let ipPageError = $state('');
	let ipBulkDeleting = $state(false);

	async function bulkDeleteIpBlocks(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (ipBulkDeleting) return; // in-flight guard
		ipBulkDeleting = true;
		ipPageError = '';
		try {
			await Promise.all(idArr.map((id) => spamApi.deleteIpBlock(id)));
		} catch (e) {
			ipPageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete IP blocks');
		} finally {
			ipBulkDeleting = false;
		}
	}

	let cwController = $state<EntityPageController | undefined>(undefined);
	let cwPageError = $state('');
	let cwBulkDeleting = $state(false);

	async function bulkDeleteCensoredWords(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (cwBulkDeleting) return; // in-flight guard
		cwBulkDeleting = true;
		cwPageError = '';
		try {
			await Promise.all(idArr.map((id) => spamApi.deleteCensoredWord(id)));
		} catch (e) {
			cwPageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete censored words');
		} finally {
			cwBulkDeleting = false;
		}
	}

	// ── IP Blocks config ─────────────────────────────────────────────────────
	const ipFields: FieldDef[] = [
		{ key: 'cidr', label: t('spam.cidr', 'IP / CIDR'), type: 'text', placeholder: '192.168.1.0/24', required: true },
		{ key: 'block_type', label: t('spam.block_type', 'Block Type'), type: 'select', options: blockTypeOptions, required: true },
		{ key: 'reason', label: t('spam.reason', 'Reason'), type: 'text', placeholder: t('spam.reason_placeholder', 'Spam source') },
		{ key: 'expires_at', label: t('spam.expires', 'Expires At'), type: 'datetime-local', help: t('common.optional', 'optional') }
	];

	const ipConfig: EntityPageConfig = {
		title: t('spam.ip_blocks', 'IP Blocks'),
		description: t('spam.description', 'Manage IP blocks, censored words, and user blocks'),
		createLabel: t('spam.add_ip_block', 'Add IP Block'),
		createPermission: PERMISSION,
		editPermission: PERMISSION,
		deletePermission: PERMISSION,
		pageSize: 25,
		columns: [
			{ key: 'cidr', label: t('spam.cidr', 'IP / CIDR'), sortable: true },
			{ key: 'block_type', label: t('spam.block_type', 'Type'), sortable: true },
			{ key: 'reason', label: t('spam.reason', 'Reason'), sortable: true, render: (v: string) => v || '—' },
			{ key: 'expires_at', label: t('spam.expires', 'Expires'), sortable: true, render: (v: string) => v || t('spam.never', 'Never') },
			{ key: 'created_at', label: t('common.created', 'Created'), sortable: true, render: (v: string) => (v ? new Date(v).toLocaleDateString() : '—') }
		],
		filters: [
			{ key: 'block_type', label: t('spam.block_type', 'Block Type'), options: blockTypeOptions }
		],
		bulkActions: [
			{ label: t('common.deleteSelected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDeleteIpBlocks }
		],
		fields: ipFields,
		load: async ({ q, filter }) => {
			const params = new URLSearchParams();
			if (filter.block_type) params.set('block_type', filter.block_type);
			const result = await spamApi.getIpBlocks(params.toString() || undefined);
			if (result.ok) {
				let items = (result.data as IpBlock[]) ?? [];
				if (q) {
					const lq = q.toLowerCase();
					items = items.filter((b) => b.cidr?.toLowerCase().includes(lq) || b.reason?.toLowerCase().includes(lq));
				}
				return { data: items, total: items.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load IP blocks'));
		},
		create: async (body) => {
			const payload: { cidr: string; block_type: string; reason?: string; expires_at?: string } = {
				cidr: String(body.cidr ?? '').trim(),
				block_type: String(body.block_type ?? 'full')
			};
			if (String(body.reason ?? '').trim()) payload.reason = String(body.reason).trim();
			if (body.expires_at) payload.expires_at = datetimeLocalToPostgresTimestamptz(String(body.expires_at));
			const res = await spamApi.createIpBlock(payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const payload: { cidr?: string; block_type?: string; reason?: string; expires_at?: string } = {
				cidr: String(body.cidr ?? '').trim(),
				block_type: String(body.block_type ?? 'full')
			};
			if (String(body.reason ?? '').trim()) payload.reason = String(body.reason).trim();
			else payload.reason = '';
			if (body.expires_at) payload.expires_at = datetimeLocalToPostgresTimestamptz(String(body.expires_at));
			const res = await spamApi.updateIpBlock(id, payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await spamApi.deleteIpBlock(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		}
	};

	// ── Censored Words config ────────────────────────────────────────────────
	const cwFields: FieldDef[] = [
		{ key: 'pattern', label: t('spam.pattern', 'Pattern'), type: 'text', placeholder: t('spam.pattern_placeholder', 'badword'), required: true },
		{ key: 'action', label: t('spam.action', 'Action'), type: 'select', options: actionOptions, required: true },
		{ key: 'replacement', label: t('spam.replacement', 'Replacement'), type: 'text', placeholder: '***', help: t('common.optional', 'optional') }
	];

	const cwConfig: EntityPageConfig = {
		title: t('spam.censored_words', 'Censored Words'),
		description: t('spam.description', 'Manage IP blocks, censored words, and user blocks'),
		createLabel: t('spam.add_pattern', 'Add Pattern'),
		createPermission: PERMISSION,
		editPermission: PERMISSION,
		deletePermission: PERMISSION,
		pageSize: 25,
		columns: [
			{ key: 'pattern', label: t('spam.pattern', 'Pattern'), sortable: true },
			{ key: 'action', label: t('spam.action', 'Action'), sortable: true },
			{ key: 'replacement', label: t('spam.replacement', 'Replacement'), sortable: true, render: (v: string) => v || '—' },
			{ key: 'created_at', label: t('common.created', 'Created'), sortable: true, render: (v: string) => (v ? new Date(v).toLocaleDateString() : '—') }
		],
		filters: [{ key: 'action', label: t('spam.action', 'Action'), options: actionOptions }],
		bulkActions: [
			{ label: t('common.deleteSelected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDeleteCensoredWords }
		],
		fields: cwFields,
		load: async ({ q, filter }) => {
			const params = new URLSearchParams();
			if (filter.action) params.set('action', filter.action);
			const result = await spamApi.getCensoredWords(params.toString() || undefined);
			if (result.ok) {
				let items = (result.data as CensoredWord[]) ?? [];
				if (q) {
					const lq = q.toLowerCase();
					items = items.filter((w) => w.pattern?.toLowerCase().includes(lq) || w.action?.toLowerCase().includes(lq));
				}
				return { data: items, total: items.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load censored words'));
		},
		create: async (body) => {
			const payload: { pattern: string; action: string; replacement?: string } = {
				pattern: String(body.pattern ?? '').trim(),
				action: String(body.action ?? 'block')
			};
			if (String(body.replacement ?? '').trim()) payload.replacement = String(body.replacement).trim();
			const res = await spamApi.createCensoredWord(payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const payload: { pattern?: string; action?: string; replacement?: string } = {
				pattern: String(body.pattern ?? '').trim(),
				action: String(body.action ?? 'block'),
				replacement: String(body.replacement ?? '').trim()
			};
			const res = await spamApi.updateCensoredWord(id, payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await spamApi.deleteCensoredWord(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		}
	};

	// ── User Blocks config (read-only; composite key blockerId:blockedId) ───
	function mapUserBlock(raw: any): UserBlock {
		return { ...raw, id: `${raw.blocker_id}:${raw.blocked_id}` };
	}

	const ubConfig: EntityPageConfig = {
		title: t('spam.user_blocks', 'User Blocks'),
		description: t('spam.description', 'Manage IP blocks, censored words, and user blocks'),
		readonly: true,
		deletePermission: PERMISSION,
		pageSize: 25,
		columns: [
			{
				key: 'blocker_display_name',
				label: t('spam.blocker', 'Blocker'),
				sortable: true,
				render: (v: string, row: UserBlock) => v || row.blocker_username || 'anonymous'
			},
			{
				key: 'blocked_display_name',
				label: t('spam.blocked', 'Blocked'),
				sortable: true,
				render: (v: string, row: UserBlock) => v || row.blocked_username || 'anonymous'
			},
			{ key: 'created_at', label: t('common.created', 'Created'), sortable: true, render: (v: string) => (v ? new Date(v).toLocaleDateString() : '—') }
		],
		fields: [],
		actions: [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) },
			{
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (_row, ctx) => ctx?.openDelete(_row),
				show: () => hasPermission(PERMISSION)
			}
		],
		load: async ({ q }) => {
			const result = await spamApi.getUserBlocks();
			if (result.ok) {
				let items = ((result.data as UserBlock[]) ?? []).map(mapUserBlock);
				if (q) {
					const lq = q.toLowerCase();
					items = items.filter(
						(b) => b.blocker_id?.toLowerCase().includes(lq) || b.blocked_id?.toLowerCase().includes(lq)
					);
				}
				return { data: items, total: items.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load user blocks'));
		},
		remove: async (id) => {
			// Composite key: blockerId:blockedId
			const sep = id.indexOf(':');
			if (sep < 0) return { ok: false, error: t('common.error', 'Invalid block id') };
			const blockerId = id.slice(0, sep);
			const blockedId = id.slice(sep + 1);
			const res = await spamApi.deleteUserBlock(blockerId, blockedId);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		}
	};
</script>

<svelte:head>
	<title>{t('spam.title', 'Spam Management')} — JapaConnect Admin</title>
</svelte:head>

<Tabs {tabs} bind:value={activeTab} class="mb-4" />

{#if activeTab === 'ip-blocks'}
	{#if ipPageError}
		<div
			class="mb-4 rounded-lg p-3 text-sm"
			style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
		>
			{ipPageError}
		</div>
	{/if}
	<EntityPage config={ipConfig} bind:controller={ipController} />
{:else if activeTab === 'censored-words'}
	{#if cwPageError}
		<div
			class="mb-4 rounded-lg p-3 text-sm"
			style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
		>
			{cwPageError}
		</div>
	{/if}
	<EntityPage config={cwConfig} bind:controller={cwController} />
{:else if activeTab === 'user-blocks'}
	<EntityPage config={ubConfig} />
{/if}
