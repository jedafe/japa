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

	// BUG-008: each tab gates create/edit/delete on its own permission node,
	// matching the backend RBAC wiring (spam.ip_block.manage,
	// spam.censored_words.manage, spam.user_block.manage). The previous code
	// used a single 'spam.ip_block.manage' for all three tabs.
	const IP_BLOCK_PERM = 'spam.ip_block.manage';
	const CENSORED_PERM = 'spam.censored_words.manage';
	const USER_BLOCK_PERM = 'spam.user_block.manage';

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
		createPermission: IP_BLOCK_PERM,
		editPermission: IP_BLOCK_PERM,
		deletePermission: IP_BLOCK_PERM,
		pageSize: 25,
		columns: [
			{ key: 'cidr', label: t('spam.cidr', 'IP / CIDR') },
			{ key: 'block_type', label: t('spam.block_type', 'Type') },
			{ key: 'reason', label: t('spam.reason', 'Reason'), render: (v: string) => v || '—' },
			{ key: 'expires_at', label: t('spam.expires', 'Expires'), render: (v: string) => v || t('spam.never', 'Never') },
			{ key: 'created_at', label: t('common.created', 'Created'), render: (v: string) => (v ? new Date(v).toLocaleDateString() : '—') }
		],
		filters: [
			{ key: 'block_type', label: t('spam.block_type', 'Block Type'), options: blockTypeOptions }
		],
		bulkActions: [
			{ label: t('common.deleteSelected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDeleteIpBlocks }
		],
		fields: ipFields,
		load: async ({ page, pageSize, q, filter }) => {
			// BUG-010: server-side pagination. The backend now accepts
			// page_size + page_offset + optional block_type filter and
			// returns total/total_pages at the top level.
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (filter.block_type) params.set('block_type', filter.block_type);
			const result = await spamApi.getIpBlocks(params.toString());
			if (result.ok) {
				const d = result.data;
				let items = (d.data as IpBlock[]) ?? [];
				// q (text search) is still client-side — the backend list
				// endpoint does not support a q param for IP blocks.
				if (q) {
					const lq = q.toLowerCase();
					items = items.filter((b) => b.cidr?.toLowerCase().includes(lq) || b.reason?.toLowerCase().includes(lq));
				}
				return {
					data: items,
					total: d.total ?? items.length,
					totalPages: d.total_pages ?? 1
				};
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
		createPermission: CENSORED_PERM,
		editPermission: CENSORED_PERM,
		deletePermission: CENSORED_PERM,
		pageSize: 25,
		columns: [
			{ key: 'pattern', label: t('spam.pattern', 'Pattern') },
			{ key: 'action', label: t('spam.action', 'Action') },
			{ key: 'replacement', label: t('spam.replacement', 'Replacement'), render: (v: string) => v || '—' },
			{ key: 'created_at', label: t('common.created', 'Created'), render: (v: string) => (v ? new Date(v).toLocaleDateString() : '—') }
		],
		filters: [{ key: 'action', label: t('spam.action', 'Action'), options: actionOptions }],
		bulkActions: [
			{ label: t('common.deleteSelected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDeleteCensoredWords }
		],
		fields: cwFields,
		load: async ({ page, pageSize, q, filter }) => {
			// BUG-010: server-side pagination. The backend now accepts
			// page_size + page_offset + optional action filter and returns
			// total/total_pages at the top level.
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (filter.action) params.set('action', filter.action);
			const result = await spamApi.getCensoredWords(params.toString());
			if (result.ok) {
				const d = result.data;
				let items = (d.data as CensoredWord[]) ?? [];
				// q (text search) is still client-side.
				if (q) {
					const lq = q.toLowerCase();
					items = items.filter((w) => w.pattern?.toLowerCase().includes(lq) || w.action?.toLowerCase().includes(lq));
				}
				return {
					data: items,
					total: d.total ?? items.length,
					totalPages: d.total_pages ?? 1
				};
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
		deletePermission: USER_BLOCK_PERM,
		pageSize: 25,
		columns: [
			{
				key: 'blocker_display_name',
				label: t('spam.blocker', 'Blocker'),
				render: (v: string, row: UserBlock) => v || row.blocker_username || 'anonymous'
			},
			{
				key: 'blocked_display_name',
				label: t('spam.blocked', 'Blocked'),
				render: (v: string, row: UserBlock) => v || row.blocked_username || 'anonymous'
			},
			{ key: 'created_at', label: t('common.created', 'Created'), render: (v: string) => (v ? new Date(v).toLocaleDateString() : '—') }
		],
		fields: [],
		actions: [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) },
			{
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (_row, ctx) => ctx?.openDelete(_row),
				show: () => hasPermission(USER_BLOCK_PERM)
			}
		],
		load: async ({ page, pageSize, q }) => {
			// BUG-010: server-side pagination. The backend now accepts
			// page_size + page_offset and returns total/total_pages.
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			const result = await spamApi.getUserBlocks(params.toString());
			if (result.ok) {
				const d = result.data;
				let items = ((d.data as UserBlock[]) ?? []).map(mapUserBlock);
				// q (text search) is still client-side.
				if (q) {
					const lq = q.toLowerCase();
					items = items.filter(
						(b) => b.blocker_id?.toLowerCase().includes(lq) || b.blocked_id?.toLowerCase().includes(lq)
					);
				}
				return {
					data: items,
					total: d.total ?? items.length,
					totalPages: d.total_pages ?? 1
				};
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
