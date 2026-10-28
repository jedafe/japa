<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /gamification — migrated to the EntityPage scaffold.
	// Two tabs: Badges (full CRUD + award) and Challenges (full CRUD).
	// Rich forms (ActivityKeyAutocomplete) use the formSnippet escape hatch.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Tabs } from '$lib/components/ui/tabs';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import ActivityKeyAutocomplete from '$lib/components/gamification/ActivityKeyAutocomplete.svelte';
	import UserAutocomplete from '$lib/components/ui/UserAutocomplete.svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { Trophy, Target, Pencil, Award, Trash2, Eye } from 'lucide-svelte';
	import { api, adminCatalogApi, type CatalogOption } from '$lib/api';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	type Badge = {
		id: string;
		slug: string;
		name: string;
		description: string;
		trigger_type: string;
		trigger_value: number;
		trigger_activity_key: string;
		is_active: boolean;
		is_system: boolean;
		display_order: number;
		created_at: string;
	};

	type Challenge = {
		id: string;
		slug: string;
		name: string;
		description: string;
		activity_key: string;
		target_count: number;
		reward_points: number;
		reward_badge_id: string;
		starts_at: string;
		ends_at: string | null;
		is_active: boolean;
		created_at: string;
	};

	// Permission nodes per JEOS §64 — gamification.manage does NOT exist.
	const canManageBadges = $derived(hasPermission('gamification.manage_badges'));
	const canManageChallenges = $derived(hasPermission('gamification.manage_challenges'));
	const canAwardBadge = $derived(hasPermission('gamification.award_badge'));

	let activeTab = $state('badges');
	const tabs = $derived([
		{ value: 'badges', label: t('gamification.badges', 'Badges'), icon: Trophy },
		{ value: 'challenges', label: t('gamification.challenges', 'Challenges'), icon: Target }
	]);

	// ── Controllers ────────────────────────────────────────────────────────
	let badgeController = $state<EntityPageController | undefined>(undefined);
	let challengeController = $state<EntityPageController | undefined>(undefined);

	// ── Page-local state for sub-flows ─────────────────────────────────────
	let pageError = $state('');
	let acting = $state(false);

	// Award badge modal
	let awardModalOpen = $state(false);
	let awardBadgeId = $state('');
	let awardBadgeName = $state('');
	let awardUsername = $state('');
	let awardTargetUserId = $state('');

	// Delete confirmation (shared for badges + challenges)
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<{ kind: 'badge' | 'challenge'; id: string; name: string } | null>(null);

	// Aggregate all known activity keys from already-loaded badges and
	// challenges so the autocomplete can suggest them without an extra API
	// call. The ActivityKeyAutocomplete also fetches wallet rules internally.
	let knownActivityKeys = $state<string[]>([]);
	$effect(() => {
		(async () => {
			try {
				const [badgesRes, challengesRes] = await Promise.all([
					api<Badge[]>('/admin/gamification/badges'),
					api<Challenge[]>('/admin/gamification/challenges')
				]);
				const set = new Set<string>();
				if (badgesRes.ok) {
					for (const b of badgesRes.data ?? []) {
						if (b.trigger_activity_key) set.add(b.trigger_activity_key);
					}
				}
				if (challengesRes.ok) {
					for (const c of challengesRes.data ?? []) {
						if (c.activity_key) set.add(c.activity_key);
					}
				}
				knownActivityKeys = Array.from(set);
			} catch {
				// non-fatal — autocomplete still has wallet-rule suggestions
			}
		})();
	});

	// Trigger type options matching the backend CHECK constraint (§64 schema).
	let triggerTypeOptions = $state<CatalogOption[]>([
		{ value: 'lifetime_points', label: t('gamification.trigger_lifetime_points', 'Lifetime Points') },
		{ value: 'activity_count', label: t('gamification.trigger_activity_count', 'Activity Count') },
		{ value: 'manual', label: t('gamification.trigger_manual', 'Manual') }
	]);

	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('gamification.trigger_type');
			if (res.ok && res.data?.length) triggerTypeOptions = res.data;
		})();
	});

	// ── Badge EntityPage ───────────────────────────────────────────────────

	const badgeFields = $derived<FieldDef[]>([
		{ key: 'name', label: t('gamification.name', 'Name'), type: 'text', required: true },
		{ key: 'slug', label: t('gamification.slug', 'Slug'), type: 'text' },
		{ key: 'description', label: t('gamification.description', 'Description'), type: 'textarea' },
		{ key: 'trigger_type', label: t('gamification.trigger', 'Trigger Type'), type: 'select', options: triggerTypeOptions },
		{ key: 'trigger_value', label: t('gamification.trigger_value', 'Trigger Value'), type: 'number' },
		{ key: 'trigger_activity_key', label: t('gamification.activity_key', 'Activity Key'), type: 'text' },
		{ key: 'is_active', label: t('gamification.active', 'Active'), type: 'checkbox' },
		{ key: 'display_order', label: t('gamification.display_order', 'Display Order'), type: 'number' }
	]);

	const badgeFilters = $derived<FilterDef[]>([
		{
			key: 'is_active',
			label: t('gamification.active', 'Active'),
			options: [
				{ value: 'true', label: 'Active' },
				{ value: 'false', label: 'Inactive' }
			]
		},
		{
			key: 'trigger_type',
			label: t('gamification.trigger', 'Trigger'),
			options: triggerTypeOptions
		}
	]);

	const badgeActions = $derived<ActionItem[]>([
		{
			label: t('common.view', 'View'),
			icon: Eye,
			onclick: (row: Badge, ctx) => ctx?.openView(row)
		},
		{
			label: t('gamification.award', 'Award'),
			icon: Award,
			onclick: (row: Badge) => openAwardBadge(row),
			show: () => canAwardBadge
		},
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row: Badge, ctx) => ctx?.openEdit(row),
			show: (row: Badge) => canManageBadges && !row.is_system
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row: Badge) => promptDelete('badge', row.id, row.name),
			show: (row: Badge) => canManageBadges && !row.is_system
		}
	]);

	const badgeConfig = $derived<EntityPageConfig>({
		title: t('gamification.badges', 'Badges'),
		description: t('gamification.badges_description', 'Manage achievement badges and triggers'),
		createLabel: t('gamification.create_badge', 'Create Badge'),
		createPermission: 'gamification.manage_badges',
		editPermission: 'gamification.manage_badges',
		deletePermission: 'gamification.manage_badges',
		pageSize: 25,
		columns: [
			{
				key: 'name',
				label: t('gamification.name', 'Name'),
				sortable: true,
				render: (value: string) => `<span class="font-medium text-foreground">${value}</span>`
			},
			{
				key: 'slug',
				label: t('gamification.slug', 'Slug'),
				sortable: true,
				render: (value: string) => `<span class="font-mono text-xs text-muted-foreground">${value}</span>`
			},
			{
				key: 'trigger_type',
				label: t('gamification.trigger', 'Trigger'),
				sortable: true,
				render: (value: string) => `<span class="text-foreground">${value}</span>`
			},
			{
				key: 'is_active',
				label: t('gamification.active', 'Active'),
				sortable: true,
				render: (value: boolean) => value
					? `<span class="inline-flex items-center rounded-md border border-success bg-success/15 px-2 py-0.5 text-xs font-medium text-success">${t('common.active', 'Active')}</span>`
					: `<span class="inline-flex items-center rounded-md border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">${t('common.inactive', 'Inactive')}</span>`
			},
			{
				key: 'is_system',
				label: t('gamification.system', 'System'),
				sortable: true,
				render: (value: boolean) => value ? `<span class="text-xs text-muted-foreground">${t('common.system', 'System')}</span>` : `<span class="text-xs text-muted-foreground">${t('common.custom', 'Custom')}</span>`
			}
		],
		filters: badgeFilters,
		actions: badgeActions,
		fields: badgeFields,
		load: async ({ page, pageSize, q, filter }) => {
			const result = await api<Badge[]>('/admin/gamification/badges');
			if (!result.ok) {
				throw new Error(result.error ?? t('gamification.load_error', 'Failed to load gamification data'));
			}
			let data = result.data ?? [];
			// Client-side search
			if (q) {
				const query = q.toLowerCase();
				data = data.filter((b) =>
					b.name.toLowerCase().includes(query) ||
					b.slug.toLowerCase().includes(query)
				);
			}
			// Client-side filters
			if (filter.is_active) {
				const active = filter.is_active === 'true';
				data = data.filter((b) => b.is_active === active);
			}
			if (filter.trigger_type) {
				data = data.filter((b) => b.trigger_type === filter.trigger_type);
			}
			// Client-side sort
			data.sort((a, b) => a.display_order - b.display_order || a.name.localeCompare(b.name));
			const total = data.length;
			const totalPages = Math.max(1, Math.ceil(total / pageSize));
			const start = page * pageSize;
			return { data: data.slice(start, start + pageSize), total, totalPages };
		},
		create: async (body) => {
			const res = await api('/admin/gamification/badges', { method: 'POST', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await api(`/admin/gamification/badges/${id}`, { method: 'PATCH', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/gamification/badges/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: BadgeView,
		formSnippet: BadgeForm
	});

	// ── Challenge EntityPage ───────────────────────────────────────────────

	const challengeFields: FieldDef[] = [
		{ key: 'name', label: t('gamification.name', 'Name'), type: 'text', required: true },
		{ key: 'slug', label: t('gamification.slug', 'Slug'), type: 'text' },
		{ key: 'description', label: t('gamification.description', 'Description'), type: 'textarea' },
		{ key: 'activity_key', label: t('gamification.activity', 'Activity Key'), type: 'text' },
		{ key: 'target_count', label: t('gamification.target', 'Target Count'), type: 'number' },
		{ key: 'reward_points', label: t('gamification.reward', 'Reward Points'), type: 'number' },
		{ key: 'starts_at', label: t('gamification.starts', 'Start Date'), type: 'date' },
		{ key: 'ends_at', label: t('gamification.ends', 'End Date'), type: 'date' },
		{ key: 'is_active', label: t('gamification.active', 'Active'), type: 'checkbox' }
	];

	const challengeFilters = $derived<FilterDef[]>([
		{
			key: 'is_active',
			label: t('gamification.active', 'Active'),
			options: [
				{ value: 'true', label: 'Active' },
				{ value: 'false', label: 'Inactive' }
			]
		}
	]);

	const challengeActions = $derived<ActionItem[]>([
		{
			label: t('common.view', 'View'),
			icon: Eye,
			onclick: (row: Challenge, ctx) => ctx?.openView(row)
		},
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row: Challenge, ctx) => ctx?.openEdit(row),
			show: () => canManageChallenges
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row: Challenge) => promptDelete('challenge', row.id, row.name),
			show: () => canManageChallenges
		}
	]);

	const challengeConfig = $derived<EntityPageConfig>({
		title: t('gamification.challenges', 'Challenges'),
		description: t('gamification.challenges_description', 'Manage time-bound challenges and rewards'),
		createLabel: t('gamification.create_challenge', 'Create Challenge'),
		createPermission: 'gamification.manage_challenges',
		editPermission: 'gamification.manage_challenges',
		deletePermission: 'gamification.manage_challenges',
		pageSize: 25,
		columns: [
			{
				key: 'name',
				label: t('gamification.name', 'Name'),
				sortable: true,
				render: (value: string) => `<span class="font-medium text-foreground">${value}</span>`
			},
			{
				key: 'activity_key',
				label: t('gamification.activity', 'Activity'),
				sortable: true,
				render: (value: string) => `<span class="font-mono text-xs text-muted-foreground">${value}</span>`
			},
			{
				key: 'target_count',
				label: t('gamification.target', 'Target'),
				sortable: true,
				render: (value: number) => `<span class="font-mono text-foreground">${value}</span>`
			},
			{
				key: 'reward_points',
				label: t('gamification.reward', 'Reward Points'),
				sortable: true,
				render: (value: number) => `<span class="font-mono text-foreground">${value}</span>`
			},
			{
				key: 'is_active',
				label: t('gamification.active', 'Active'),
				sortable: true,
				render: (value: boolean) => value
					? `<span class="inline-flex items-center rounded-md border border-success bg-success/15 px-2 py-0.5 text-xs font-medium text-success">${t('common.active', 'Active')}</span>`
					: `<span class="inline-flex items-center rounded-md border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">${t('common.inactive', 'Inactive')}</span>`
			},
			{
				key: 'ends_at',
				label: t('gamification.ends', 'Ends'),
				sortable: true,
				render: (value: string | null) => `<span class="text-muted-foreground text-xs">${value ? new Date(value).toLocaleDateString() : '—'}</span>`
			}
		],
		filters: challengeFilters,
		actions: challengeActions,
		fields: challengeFields,
		load: async ({ page, pageSize, q, filter }) => {
			const result = await api<Challenge[]>('/admin/gamification/challenges');
			if (!result.ok) {
				throw new Error(result.error ?? t('gamification.load_error', 'Failed to load gamification data'));
			}
			let data = result.data ?? [];
			// Client-side search
			if (q) {
				const query = q.toLowerCase();
				data = data.filter((c) =>
					c.name.toLowerCase().includes(query) ||
					c.slug.toLowerCase().includes(query) ||
					c.activity_key.toLowerCase().includes(query)
				);
			}
			// Client-side filters
			if (filter.is_active) {
				const active = filter.is_active === 'true';
				data = data.filter((c) => c.is_active === active);
			}
			// Client-side sort
			data.sort((a, b) => a.name.localeCompare(b.name));
			const total = data.length;
			const totalPages = Math.max(1, Math.ceil(total / pageSize));
			const start = page * pageSize;
			return { data: data.slice(start, start + pageSize), total, totalPages };
		},
		create: async (body) => {
			const payload = { ...body, ends_at: body.ends_at || undefined };
			const res = await api('/admin/gamification/challenges', { method: 'POST', body: payload });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const payload = { ...body, ends_at: body.ends_at || undefined };
			const res = await api(`/admin/gamification/challenges/${id}`, { method: 'PATCH', body: payload });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/gamification/challenges/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: ChallengeView,
		formSnippet: ChallengeForm
	});

	// ── Sub-flow handlers ──────────────────────────────────────────────────

	function openAwardBadge(badge: Badge) {
		awardBadgeId = badge.id;
		awardBadgeName = badge.name;
		awardUsername = '';
		awardTargetUserId = '';
		awardModalOpen = true;
	}

	async function awardBadge() {
		if (!awardTargetUserId) {
			pageError = t('gamification.select_user', 'Please select a user from the search results.');
			return;
		}
		acting = true;
		pageError = '';
		try {
			const result = await api(`/admin/gamification/badges/${awardBadgeId}/award`, { method: 'POST', body: { user_id: awardTargetUserId } });
			if (result.ok) {
				awardModalOpen = false;
				awardUsername = '';
				awardTargetUserId = '';
			} else {
				pageError = result.error;
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('gamification.award_error', 'Failed to award badge');
		} finally {
			acting = false;
		}
	}

	function promptDelete(kind: 'badge' | 'challenge', id: string, name: string) {
		deleteTarget = { kind, id, name };
		showDeleteConfirm = true;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		acting = true;
		pageError = '';
		const kind = deleteTarget.kind;
		try {
			const endpoint = kind === 'badge'
				? `/admin/gamification/badges/${deleteTarget.id}`
				: `/admin/gamification/challenges/${deleteTarget.id}`;
			const result = await api(endpoint, { method: 'DELETE' });
			if (result.ok) {
				showDeleteConfirm = false;
				deleteTarget = null;
				// Reload the appropriate tab's EntityPage
				if (kind === 'badge') {
					badgeController?.reload();
				} else {
					challengeController?.reload();
				}
			} else {
				pageError = result.error;
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete');
		} finally {
			acting = false;
		}
	}
</script>

<svelte:head>
	<title>{t('gamification.title', 'Gamification')} — {siteName()} Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg border p-3 text-sm"
		style="border-color: var(--color-danger); background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<Tabs {tabs} bind:value={activeTab} tabParam="tab" tabPreserve={[]} />

{#if activeTab === 'badges'}
	<EntityPage config={badgeConfig} bind:controller={badgeController} />
{:else}
	<EntityPage config={challengeConfig} bind:controller={challengeController} />
{/if}

<!-- Award Badge Modal -->
<Modal bind:open={awardModalOpen} title={t('gamification.award_title', 'Award Badge')} size="sm">
	<div>
		<p class="mb-3 text-sm" style="color: var(--color-foreground)">
			{t('gamification.award_badge_to', 'Award')} <strong>{awardBadgeName}</strong>
		</p>
		<label for="award-username" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.username', 'Username')}</label>
		<UserAutocomplete
			id="award-username"
			bind:value={awardUsername}
			placeholder={t('gamification.search_user', 'Search by username...')}
			onSelectUser={(u) => (awardTargetUserId = u.id)}
		/>
		{#if awardTargetUserId}
			<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('gamification.selected_user', 'Selected')}: {awardUsername} ({awardTargetUserId.slice(0, 8)}...)</p>
		{/if}
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (awardModalOpen = false)} disabled={acting}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={awardBadge} disabled={acting || !awardTargetUserId}>
			{acting ? t('common.saving', 'Saving...') : t('gamification.award', 'Award Badge')}
		</Button>
	</div>
</Modal>

<!-- Delete confirmation (per jc-admin-develop: destructive actions behind Modal) -->
<Modal bind:open={showDeleteConfirm} title={t('common.confirmDelete', 'Confirm Delete')} size="sm">
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('gamification.delete_confirm', 'Delete')}
			<strong>{deleteTarget.name}</strong>?
			{t('common.cannot_undo', 'This action cannot be undone.')}
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showDeleteConfirm = false)} disabled={acting}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmDelete} disabled={acting}>
			<Trash2 class="me-2 h-4 w-4" />
			{acting ? t('common.deleting', 'Deleting...') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>

<!-- ── View snippets ─────────────────────────────────────────────────────── -->

{#snippet BadgeView({ row }: { row: Badge })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.slug', 'Slug')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
			</div>
			<div class="col-span-2">
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.description', 'Description')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.trigger', 'Trigger Type')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.trigger_type}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.trigger_value', 'Trigger Value')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.trigger_value}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.activity_key', 'Activity Key')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.trigger_activity_key || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.display_order', 'Display Order')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.display_order}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.system', 'System')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.is_system ? 'System' : 'Custom'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.active', 'Active')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.is_active ? 'Active' : 'Inactive'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.created', 'Created')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
			</div>
		</div>
	</div>
{/snippet}

{#snippet ChallengeView({ row }: { row: Challenge })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.slug', 'Slug')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
			</div>
			<div class="col-span-2">
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.description', 'Description')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.activity', 'Activity Key')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.activity_key}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.target', 'Target Count')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.target_count}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.reward', 'Reward Points')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.reward_points}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.starts', 'Starts')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.starts_at ? new Date(row.starts_at).toLocaleDateString() : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.ends', 'Ends')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.ends_at ? new Date(row.ends_at).toLocaleDateString() : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.active', 'Active')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.is_active ? 'Active' : 'Inactive'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.created', 'Created')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
			</div>
		</div>
	</div>
{/snippet}

<!-- ── Form snippets (rich forms with ActivityKeyAutocomplete) ───────────── -->

{#snippet BadgeForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
	<div class="space-y-3">
		<div>
			<label for="badge-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</label>
			<input id="badge-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.name} placeholder={t('app.gamification.ph_top_contributor', 'Top Contributor')} />
		</div>
		<div>
			<label for="badge-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.slug', 'Slug')}</label>
			<input id="badge-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.slug} placeholder="top-contributor" />
		</div>
		<div>
			<label for="badge-desc" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.description', 'Description')}</label>
			<textarea id="badge-desc" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="2" bind:value={value.description}></textarea>
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="badge-trigger" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.trigger', 'Trigger Type')}</label>
				<AutocompleteSelect
					id="badge-trigger"
					bind:value={value.trigger_type}
					options={triggerTypeOptions}
					class="w-full"
				/>
			</div>
			<div>
				<label for="badge-trigger-value" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.trigger_value', 'Trigger Value')}</label>
				<input id="badge-trigger-value" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.trigger_value} />
			</div>
		</div>
		<div>
			<label for="badge-activity" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.activity_key', 'Activity Key')}</label>
			<ActivityKeyAutocomplete id="badge-activity" bind:value={value.trigger_activity_key} placeholder="post.create" extraKeys={knownActivityKeys} />
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="badge-order" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.display_order', 'Display Order')}</label>
				<input id="badge-order" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.display_order} />
			</div>
			<div class="flex items-center gap-2 pt-6">
				<input type="checkbox" id="badge-active" bind:checked={value.is_active} />
				<label for="badge-active" class="text-sm" style="color: var(--color-foreground)">{t('gamification.active', 'Active')}</label>
			</div>
		</div>
	</div>
{/snippet}

{#snippet ChallengeForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
	<div class="space-y-3">
		<div>
			<label for="challenge-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</label>
			<input id="challenge-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.name} placeholder={t('app.gamification.ph_30day_posting_streak', '30-Day Posting Streak')} />
		</div>
		<div>
			<label for="challenge-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.slug', 'Slug')}</label>
			<input id="challenge-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.slug} placeholder="30-day-streak" />
		</div>
		<div>
			<label for="challenge-desc" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.description', 'Description')}</label>
			<textarea id="challenge-desc" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="2" bind:value={value.description}></textarea>
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="challenge-activity" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.activity', 'Activity Key')}</label>
				<ActivityKeyAutocomplete id="challenge-activity" bind:value={value.activity_key} placeholder="post.create" extraKeys={knownActivityKeys} />
			</div>
			<div>
				<label for="challenge-target" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.target', 'Target Count')}</label>
				<input id="challenge-target" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.target_count} />
			</div>
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="challenge-reward" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.reward', 'Reward Points')}</label>
				<input id="challenge-reward" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.reward_points} />
			</div>
			<div>
				<label for="challenge-starts" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.starts', 'Start Date')}</label>
				<input id="challenge-starts" type="date" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.starts_at} />
			</div>
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="challenge-ends" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.ends', 'End Date')}</label>
				<input id="challenge-ends" type="date" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.ends_at} />
			</div>
			<div class="flex items-center gap-2 pt-6">
				<input type="checkbox" id="challenge-active" bind:checked={value.is_active} />
				<label for="challenge-active" class="text-sm" style="color: var(--color-foreground)">{t('gamification.active', 'Active')}</label>
			</div>
		</div>
	</div>
{/snippet}
