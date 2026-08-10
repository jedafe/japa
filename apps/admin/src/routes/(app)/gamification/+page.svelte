<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import { Tabs } from '$lib/components/ui/tabs';
	import ActivityKeyAutocomplete from '$lib/components/ui/ActivityKeyAutocomplete.svelte';
	import UserAutocomplete from '$lib/components/ui/UserAutocomplete.svelte';
	import { Trophy, Plus, RefreshCw, Target, Pencil, Award, Trash2, Eye } from 'lucide-svelte';
	import { api } from '$lib/api';

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
	let badges = $state<Badge[]>([]);
	let challenges = $state<Challenge[]>([]);
	let loading = $state(true);
	let error = $state('');
	let createSlideOverOpen = $state(false);
	let createType = $state<'badge' | 'challenge'>('badge');
	let saving = $state(false);
	let message = $state('');
	let messageType = $state('');
	let awardModalOpen = $state(false);
	let awardBadgeId = $state('');
	let awardUsername = $state('');
	let awardTargetUserId = $state('');

	// View slide-over
	let viewSlideOverOpen = $state(false);
	let viewItem = $state<{ kind: 'badge' | 'challenge'; data: Badge | Challenge } | null>(null);

	// Edit badge slide-over
	let editBadgeSlideOverOpen = $state(false);
	let editingBadge = $state<Badge | null>(null);
	let editBadgeForm = $state({
		name: '',
		description: '',
		trigger_type: 'activity_count',
		trigger_value: 0,
		trigger_activity_key: '',
		is_active: true,
		display_order: 0
	});
	let savingEditBadge = $state(false);

	// Edit challenge slide-over
	let editChallengeSlideOverOpen = $state(false);
	let editingChallenge = $state<Challenge | null>(null);
	let editChallengeForm = $state({
		name: '',
		description: '',
		activity_key: '',
		target_count: 10,
		reward_points: 100,
		starts_at: new Date().toISOString().slice(0, 10),
		ends_at: '',
		is_active: true
	});
	let savingEditChallenge = $state(false);

	// Delete confirmation
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<{ kind: 'badge' | 'challenge'; id: string; name: string } | null>(null);
	let deleting = $state(false);

	let newBadge = $state({
		slug: '',
		name: '',
		description: '',
		trigger_type: 'activity_count',
		trigger_value: 0,
		trigger_activity_key: '',
		is_active: true,
		display_order: 0
	});

	let newChallenge = $state({
		slug: '',
		name: '',
		description: '',
		activity_key: '',
		target_count: 10,
		reward_points: 100,
		starts_at: new Date().toISOString().slice(0, 10),
		ends_at: '',
		is_active: true
	});

	const tabs = $derived([
		{ value: 'badges', label: t('gamification.badges', 'Badges'), icon: Trophy },
		{ value: 'challenges', label: t('gamification.challenges', 'Challenges'), icon: Target }
	]);

	// Trigger type options matching the backend CHECK constraint (§64 schema).
	const triggerTypeOptions = [
		{ value: 'lifetime_points', label: t('gamification.trigger_lifetime_points', 'Lifetime Points') },
		{ value: 'activity_count', label: t('gamification.trigger_activity_count', 'Activity Count') },
		{ value: 'manual', label: t('gamification.trigger_manual', 'Manual') }
	];

	// Aggregate all known activity keys from already-loaded badges and
	// challenges so the autocomplete can suggest them without an extra API
	// call. The ActivityKeyAutocomplete also fetches wallet rules internally.
	const knownActivityKeys = $derived.by(() => {
		const set = new Set<string>();
		for (const b of badges) {
			if (b.trigger_activity_key) set.add(b.trigger_activity_key);
		}
		for (const c of challenges) {
			if (c.activity_key) set.add(c.activity_key);
		}
		return Array.from(set);
	});

	async function load() {
		loading = true;
		error = '';
		try {
			const [badgesRes, challengesRes] = await Promise.all([
				api<Badge[]>('/admin/gamification/badges'),
				api<Challenge[]>('/admin/gamification/challenges')
			]);
			if (badgesRes.ok) {
				badges = badgesRes.data ?? [];
			}
			if (challengesRes.ok) {
				challenges = challengesRes.data ?? [];
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('gamification.load_error', 'Failed to load gamification data');
		} finally {
			loading = false;
		}
	}

	async function createBadge() {
		saving = true;
		message = '';
		try {
			const result = await api('/admin/gamification/badges', { method: 'POST', body: newBadge });
			if (result.ok) {
				messageType = 'success';
				message = t('gamification.badge_created', 'Badge created.');
				createSlideOverOpen = false;
				newBadge = { slug: '', name: '', description: '', trigger_type: 'activity_count', trigger_value: 0, trigger_activity_key: '', is_active: true, display_order: 0 };
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('gamification.create_error', 'Failed to create badge');
		} finally {
			saving = false;
		}
	}

	async function createChallenge() {
		saving = true;
		message = '';
		try {
			const body = {
				...newChallenge,
				ends_at: newChallenge.ends_at || undefined
			};
			const result = await api('/admin/gamification/challenges', { method: 'POST', body });
			if (result.ok) {
				messageType = 'success';
				message = t('gamification.challenge_created', 'Challenge created.');
				createSlideOverOpen = false;
				newChallenge = { slug: '', name: '', description: '', activity_key: '', target_count: 10, reward_points: 100, starts_at: new Date().toISOString().slice(0, 10), ends_at: '', is_active: true };
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('gamification.create_error', 'Failed to create challenge');
		} finally {
			saving = false;
		}
	}

	async function awardBadge() {
		if (!awardTargetUserId) {
			messageType = 'error';
			message = t('gamification.select_user', 'Please select a user from the search results.');
			return;
		}
		saving = true;
		message = '';
		try {
			const result = await api(`/admin/gamification/badges/${awardBadgeId}/award`, { method: 'POST', body: { user_id: awardTargetUserId } });
			if (result.ok) {
				messageType = 'success';
				message = t('gamification.badge_awarded', 'Badge awarded successfully.');
				awardModalOpen = false;
				awardUsername = '';
				awardTargetUserId = '';
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('gamification.award_error', 'Failed to award badge');
		} finally {
			saving = false;
		}
	}

	function openViewBadge(badge: Badge) {
		viewItem = { kind: 'badge', data: badge };
		viewSlideOverOpen = true;
	}

	function openViewChallenge(challenge: Challenge) {
		viewItem = { kind: 'challenge', data: challenge };
		viewSlideOverOpen = true;
	}

	function openEditBadge(badge: Badge) {
		editingBadge = badge;
		editBadgeForm = {
			name: badge.name,
			description: badge.description,
			trigger_type: badge.trigger_type,
			trigger_value: badge.trigger_value,
			trigger_activity_key: badge.trigger_activity_key,
			is_active: badge.is_active,
			display_order: badge.display_order
		};
		editBadgeSlideOverOpen = true;
	}

	function openEditChallenge(challenge: Challenge) {
		editingChallenge = challenge;
		editChallengeForm = {
			name: challenge.name,
			description: challenge.description,
			activity_key: challenge.activity_key,
			target_count: challenge.target_count,
			reward_points: challenge.reward_points,
			starts_at: challenge.starts_at ? new Date(challenge.starts_at).toISOString().slice(0, 10) : new Date().toISOString().slice(0, 10),
			ends_at: challenge.ends_at ? new Date(challenge.ends_at).toISOString().slice(0, 10) : '',
			is_active: challenge.is_active
		};
		editChallengeSlideOverOpen = true;
	}

	async function submitEditBadge() {
		if (!editingBadge) return;
		savingEditBadge = true;
		message = '';
		try {
			const result = await api(`/admin/gamification/badges/${editingBadge.id}`, { method: 'PATCH', body: editBadgeForm });
			if (result.ok) {
				messageType = 'success';
				message = t('gamification.badge_updated', 'Badge updated.');
				editBadgeSlideOverOpen = false;
				editingBadge = null;
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('gamification.update_error', 'Failed to update badge');
		} finally {
			savingEditBadge = false;
		}
	}

	async function submitEditChallenge() {
		if (!editingChallenge) return;
		savingEditChallenge = true;
		message = '';
		try {
			const body = {
				...editChallengeForm,
				ends_at: editChallengeForm.ends_at || undefined
			};
			const result = await api(`/admin/gamification/challenges/${editingChallenge.id}`, { method: 'PATCH', body });
			if (result.ok) {
				messageType = 'success';
				message = t('gamification.challenge_updated', 'Challenge updated.');
				editChallengeSlideOverOpen = false;
				editingChallenge = null;
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('gamification.update_error', 'Failed to update challenge');
		} finally {
			savingEditChallenge = false;
		}
	}

	function openAwardBadge(badge: Badge) {
		awardBadgeId = badge.id;
		awardUsername = '';
		awardTargetUserId = '';
		awardModalOpen = true;
	}

	function promptDeleteBadge(badge: Badge) {
		deleteTarget = { kind: 'badge', id: badge.id, name: badge.name };
		showDeleteConfirm = true;
	}

	function promptDeleteChallenge(challenge: Challenge) {
		deleteTarget = { kind: 'challenge', id: challenge.id, name: challenge.name };
		showDeleteConfirm = true;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		deleting = true;
		message = '';
		try {
			const endpoint = deleteTarget.kind === 'badge'
				? `/admin/gamification/badges/${deleteTarget.id}`
				: `/admin/gamification/challenges/${deleteTarget.id}`;
			const result = await api(endpoint, { method: 'DELETE' });
			if (result.ok) {
				messageType = 'success';
				message = deleteTarget.kind === 'badge'
					? t('gamification.badge_deleted', 'Badge deleted.')
					: t('gamification.challenge_deleted', 'Challenge deleted.');
				showDeleteConfirm = false;
				deleteTarget = null;
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('common.error', 'Failed to delete');
		} finally {
			deleting = false;
		}
	}

	const badgeColumns = $derived([
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
				? `<span class="inline-flex items-center rounded-md border border-success bg-success/15 px-2 py-0.5 text-xs font-medium text-success">Active</span>`
				: `<span class="inline-flex items-center rounded-md border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">Inactive</span>`
		},
		{
			key: 'is_system',
			label: t('gamification.system', 'System'),
			sortable: true,
			render: (value: boolean) => value ? `<span class="text-xs text-muted-foreground">System</span>` : `<span class="text-xs text-muted-foreground">Custom</span>`
		}
	]);

	const badgeRowActions: ActionItem[] = [
		{
			label: t('common.view', 'View'),
			icon: Eye,
			onclick: (row: Badge) => openViewBadge(row)
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
			onclick: (row: Badge) => openEditBadge(row),
			show: (row: Badge) => canManageBadges && !row.is_system
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row: Badge) => promptDeleteBadge(row),
			show: (row: Badge) => canManageBadges && !row.is_system
		}
	];

	const challengeColumns = $derived([
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
				? `<span class="inline-flex items-center rounded-md border border-success bg-success/15 px-2 py-0.5 text-xs font-medium text-success">Active</span>`
				: `<span class="inline-flex items-center rounded-md border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">Inactive</span>`
		},
		{
			key: 'ends_at',
			label: t('gamification.ends', 'Ends'),
			sortable: true,
			render: (value: string | null) => `<span class="text-muted-foreground text-xs">${value ? new Date(value).toLocaleDateString() : '—'}</span>`
		}
	]);

	const challengeRowActions: ActionItem[] = [
		{
			label: t('common.view', 'View'),
			icon: Eye,
			onclick: (row: Challenge) => openViewChallenge(row)
		},
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row: Challenge) => openEditChallenge(row),
			show: () => canManageChallenges
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row: Challenge) => promptDeleteChallenge(row),
			show: () => canManageChallenges
		}
	];

	// Load once on mount — both tabs' data is fetched together.
	$effect(() => {
		load();
	});
</script>

<PageHeader
	title={t('gamification.title', 'Gamification')}
	description={t('gamification.description', 'Manage badges, challenges, and leaderboard settings.')}
	actions={(canManageBadges || canManageChallenges) ? [
		{ label: t('gamification.create_badge', 'Create Badge'), icon: Trophy, onclick: () => { createType = 'badge'; createSlideOverOpen = true; } },
		{ label: t('gamification.create_challenge', 'Create Challenge'), icon: Target, onclick: () => { createType = 'challenge'; createSlideOverOpen = true; } },
		{ label: t('common.refresh', 'Refresh'), icon: RefreshCw, variant: 'ghost', onclick: load }
	] : [
		{ label: t('common.refresh', 'Refresh'), icon: RefreshCw, variant: 'ghost', onclick: load }
	]}
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
	<TableSkeleton rows={6} columns={6} />
{:else if error}
	<div class="rounded-lg border p-4" style="border-color: var(--color-danger); color: var(--color-danger)">
		{error}
	</div>
{:else}
	<Tabs {tabs} bind:value={activeTab} />

	{#if activeTab === 'badges'}
		<DataTable
			data={badges}
			columns={badgeColumns}
			actions={badgeRowActions}
			searchable={true}
			searchPlaceholder={t('gamification.search_badges', 'Search badges...')}
			pageSize={25}
			emptyMessage={t('gamification.no_badges', 'No badges found. Create one to get started.')}
		/>
	{:else}
		<DataTable
			data={challenges}
			columns={challengeColumns}
			actions={challengeRowActions}
			searchable={true}
			searchPlaceholder={t('gamification.search_challenges', 'Search challenges...')}
			pageSize={25}
			emptyMessage={t('gamification.no_challenges', 'No challenges found. Create one to get started.')}
		/>
	{/if}
{/if}

<!-- Create SlideOver -->
<SlideOver bind:open={createSlideOverOpen} title={createType === 'badge' ? t('gamification.create_badge_title', 'Create Badge') : t('gamification.create_challenge_title', 'Create Challenge')}>
	{#if createType === 'badge'}
		<div class="space-y-3">
			<div>
				<label for="badge-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</label>
				<input id="badge-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newBadge.name} placeholder={t('app.gamification.ph_top_contributor', 'Top Contributor')} />
			</div>
			<div>
				<label for="badge-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.slug', 'Slug')}</label>
				<input id="badge-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newBadge.slug} placeholder="top-contributor" />
			</div>
			<div>
				<label for="badge-desc" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.description', 'Description')}</label>
				<textarea id="badge-desc" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="2" bind:value={newBadge.description}></textarea>
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div>
					<label for="badge-trigger" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.trigger', 'Trigger Type')}</label>
					<select id="badge-trigger" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newBadge.trigger_type}>
						{#each triggerTypeOptions as opt (opt)}
							<option value={opt.value}>{opt.label}</option>
						{/each}
					</select>
				</div>
				<div>
					<label for="badge-trigger-value" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.trigger_value', 'Trigger Value')}</label>
					<input id="badge-trigger-value" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newBadge.trigger_value} />
				</div>
			</div>
			<div>
				<label for="badge-activity" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.activity_key', 'Activity Key')}</label>
				<ActivityKeyAutocomplete id="badge-activity" bind:value={newBadge.trigger_activity_key} placeholder="post.create" extraKeys={knownActivityKeys} />
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div>
					<label for="badge-order" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.display_order', 'Display Order')}</label>
					<input id="badge-order" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newBadge.display_order} />
				</div>
				<div class="flex items-center gap-2 pt-6">
					<input type="checkbox" id="badge-active-create" bind:checked={newBadge.is_active} />
					<label for="badge-active-create" class="text-sm" style="color: var(--color-foreground)">{t('gamification.active', 'Active')}</label>
				</div>
			</div>
		</div>
	{:else}
		<div class="space-y-3">
			<div>
				<label for="challenge-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</label>
				<input id="challenge-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newChallenge.name} placeholder={t('app.gamification.ph_30day_posting_streak', '30-Day Posting Streak')} />
			</div>
			<div>
				<label for="challenge-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.slug', 'Slug')}</label>
				<input id="challenge-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newChallenge.slug} placeholder="30-day-streak" />
			</div>
			<div>
				<label for="challenge-desc" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.description', 'Description')}</label>
				<textarea id="challenge-desc" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="2" bind:value={newChallenge.description}></textarea>
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div>
					<label for="challenge-activity" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.activity', 'Activity Key')}</label>
					<ActivityKeyAutocomplete id="challenge-activity" bind:value={newChallenge.activity_key} placeholder="post.create" extraKeys={knownActivityKeys} />
				</div>
				<div>
					<label for="challenge-target" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.target', 'Target Count')}</label>
					<input id="challenge-target" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newChallenge.target_count} />
				</div>
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div>
					<label for="challenge-reward" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.reward', 'Reward Points')}</label>
					<input id="challenge-reward" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newChallenge.reward_points} />
				</div>
				<div>
					<label for="challenge-ends" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.ends', 'End Date')}</label>
					<input id="challenge-ends" type="date" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newChallenge.ends_at} />
				</div>
			</div>
			<div class="flex items-center gap-2">
				<input type="checkbox" id="challenge-active-create" bind:checked={newChallenge.is_active} />
				<label for="challenge-active-create" class="text-sm" style="color: var(--color-foreground)">{t('gamification.active', 'Active')}</label>
			</div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="mt-6 flex justify-end gap-2">
			<Button variant="outline" onclick={() => (createSlideOverOpen = false)} disabled={saving}>{t('common.cancel', 'Cancel')}</Button>
			<Button onclick={createType === 'badge' ? createBadge : createChallenge} disabled={saving}>
				{saving ? t('common.saving', 'Saving...') : t('common.create', 'Create')}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- View SlideOver -->
<SlideOver bind:open={viewSlideOverOpen} title={viewItem?.kind === 'badge' ? t('gamification.badge_details', 'Badge Details') : t('gamification.challenge_details', 'Challenge Details')}>
	{#if viewItem?.kind === 'badge' && viewItem.data}
		{@const badge = viewItem.data as Badge}
		<div class="space-y-3">
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</span>
				<p class="text-sm" style="color: var(--color-foreground)">{badge.name}</p>
			</div>
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.slug', 'Slug')}</span>
				<p class="font-mono text-xs" style="color: var(--color-foreground)">{badge.slug}</p>
			</div>
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.description', 'Description')}</span>
				<p class="text-sm" style="color: var(--color-foreground)">{badge.description || '—'}</p>
			</div>
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.trigger', 'Trigger Type')}</span>
				<p class="text-sm" style="color: var(--color-foreground)">{badge.trigger_type}</p>
			</div>
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.trigger_value', 'Trigger Value')}</span>
				<p class="font-mono text-sm" style="color: var(--color-foreground)">{badge.trigger_value}</p>
			</div>
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.activity_key', 'Activity Key')}</span>
				<p class="font-mono text-xs" style="color: var(--color-foreground)">{badge.trigger_activity_key || '—'}</p>
			</div>
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.system', 'System')}</span>
				<p class="text-sm" style="color: var(--color-foreground)">{badge.is_system ? 'System' : 'Custom'}</p>
			</div>
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.active', 'Active')}</span>
				<p class="text-sm" style="color: var(--color-foreground)">{badge.is_active ? 'Active' : 'Inactive'}</p>
			</div>
		</div>
	{:else if viewItem?.kind === 'challenge' && viewItem.data}
		{@const challenge = viewItem.data as Challenge}
		<div class="space-y-3">
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</span>
				<p class="text-sm" style="color: var(--color-foreground)">{challenge.name}</p>
			</div>
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.slug', 'Slug')}</span>
				<p class="font-mono text-xs" style="color: var(--color-foreground)">{challenge.slug}</p>
			</div>
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.description', 'Description')}</span>
				<p class="text-sm" style="color: var(--color-foreground)">{challenge.description || '—'}</p>
			</div>
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.activity', 'Activity Key')}</span>
				<p class="font-mono text-xs" style="color: var(--color-foreground)">{challenge.activity_key}</p>
			</div>
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.target', 'Target Count')}</span>
				<p class="font-mono text-sm" style="color: var(--color-foreground)">{challenge.target_count}</p>
			</div>
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.reward', 'Reward Points')}</span>
				<p class="font-mono text-sm" style="color: var(--color-foreground)">{challenge.reward_points}</p>
			</div>
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.ends', 'Ends')}</span>
				<p class="text-sm" style="color: var(--color-foreground)">{challenge.ends_at ? new Date(challenge.ends_at).toLocaleDateString() : '—'}</p>
			</div>
			<div>
				<span class="text-sm font-medium" style="color: var(--color-muted)">{t('gamification.active', 'Active')}</span>
				<p class="text-sm" style="color: var(--color-foreground)">{challenge.is_active ? 'Active' : 'Inactive'}</p>
			</div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="mt-6 flex justify-end">
			<Button variant="outline" onclick={() => (viewSlideOverOpen = false)}>{t('common.close', 'Close')}</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Award Modal -->
<Modal bind:open={awardModalOpen} title={t('gamification.award_title', 'Award Badge')} size="sm">
	<div>
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
		<Button variant="outline" onclick={() => (awardModalOpen = false)} disabled={saving}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={awardBadge} disabled={saving || !awardTargetUserId}>
			{saving ? t('common.saving', 'Saving...') : t('gamification.award', 'Award Badge')}
		</Button>
	</div>
</Modal>

<!-- Edit Badge SlideOver -->
<SlideOver bind:open={editBadgeSlideOverOpen} title={t('gamification.edit_badge_title', 'Edit Badge')}>
	<div class="space-y-3">
		<div>
			<label for="edit-badge-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</label>
			<input id="edit-badge-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editBadgeForm.name} />
		</div>
		<div>
			<label for="edit-badge-desc" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.description', 'Description')}</label>
			<textarea id="edit-badge-desc" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="2" bind:value={editBadgeForm.description}></textarea>
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="edit-badge-trigger" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.trigger', 'Trigger Type')}</label>
				<select id="edit-badge-trigger" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editBadgeForm.trigger_type}>
					{#each triggerTypeOptions as opt (opt)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>
			<div>
				<label for="edit-badge-trigger-value" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.trigger_value', 'Trigger Value')}</label>
				<input id="edit-badge-trigger-value" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editBadgeForm.trigger_value} />
			</div>
		</div>
		<div>
			<label for="edit-badge-activity" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.activity_key', 'Activity Key')}</label>
			<ActivityKeyAutocomplete id="edit-badge-activity" bind:value={editBadgeForm.trigger_activity_key} placeholder="post.create" extraKeys={knownActivityKeys} />
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="edit-badge-order" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.display_order', 'Display Order')}</label>
				<input id="edit-badge-order" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editBadgeForm.display_order} />
			</div>
			<div class="flex items-center gap-2 pt-6">
				<input type="checkbox" id="edit-badge-active" bind:checked={editBadgeForm.is_active} />
				<label for="edit-badge-active" class="text-sm" style="color: var(--color-foreground)">{t('gamification.active', 'Active')}</label>
			</div>
		</div>
	</div>
	{#snippet footer()}
		<div class="mt-6 flex justify-end gap-2">
			<Button variant="outline" onclick={() => (editBadgeSlideOverOpen = false)} disabled={savingEditBadge}>{t('common.cancel', 'Cancel')}</Button>
			<Button onclick={submitEditBadge} disabled={savingEditBadge}>
				{savingEditBadge ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Edit Challenge SlideOver -->
<SlideOver bind:open={editChallengeSlideOverOpen} title={t('gamification.edit_challenge_title', 'Edit Challenge')}>
	<div class="space-y-3">
		<div>
			<label for="edit-challenge-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</label>
			<input id="edit-challenge-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editChallengeForm.name} />
		</div>
		<div>
			<label for="edit-challenge-desc" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.description', 'Description')}</label>
			<textarea id="edit-challenge-desc" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="2" bind:value={editChallengeForm.description}></textarea>
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="edit-challenge-activity" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.activity', 'Activity Key')}</label>
				<ActivityKeyAutocomplete id="edit-challenge-activity" bind:value={editChallengeForm.activity_key} placeholder="post.create" extraKeys={knownActivityKeys} />
			</div>
			<div>
				<label for="edit-challenge-target" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.target', 'Target Count')}</label>
				<input id="edit-challenge-target" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editChallengeForm.target_count} />
			</div>
		</div>
		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="edit-challenge-reward" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.reward', 'Reward Points')}</label>
				<input id="edit-challenge-reward" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editChallengeForm.reward_points} />
			</div>
			<div>
				<label for="edit-challenge-ends" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.ends', 'End Date')}</label>
				<input id="edit-challenge-ends" type="date" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editChallengeForm.ends_at} />
			</div>
		</div>
		<div class="flex items-center gap-2">
			<input type="checkbox" id="edit-challenge-active" bind:checked={editChallengeForm.is_active} />
			<label for="edit-challenge-active" class="text-sm" style="color: var(--color-foreground)">{t('gamification.active', 'Active')}</label>
		</div>
	</div>
	{#snippet footer()}
		<div class="mt-6 flex justify-end gap-2">
			<Button variant="outline" onclick={() => (editChallengeSlideOverOpen = false)} disabled={savingEditChallenge}>{t('common.cancel', 'Cancel')}</Button>
			<Button onclick={submitEditChallenge} disabled={savingEditChallenge}>
				{savingEditChallenge ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
			</Button>
		</div>
	{/snippet}
</SlideOver>

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
		<Button variant="outline" onclick={() => (showDeleteConfirm = false)} disabled={deleting}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmDelete} disabled={deleting}>
			<Trash2 class="mr-2 h-4 w-4" />
			{deleting ? t('common.deleting', 'Deleting...') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>
