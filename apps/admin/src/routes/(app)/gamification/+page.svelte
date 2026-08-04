<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import { Tabs } from '$lib/components/ui/tabs';
	import { Trophy, Plus, RefreshCw, Target, Pencil, Award, Trash2 } from 'lucide-svelte';
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

	const canManage = $derived(hasPermission('gamification.manage'));

	let activeTab = $state('badges');
	let badges = $state<Badge[]>([]);
	let challenges = $state<Challenge[]>([]);
	let loading = $state(true);
	let error = $state('');
	let createModalOpen = $state(false);
	let createType = $state<'badge' | 'challenge'>('badge');
	let saving = $state(false);
	let message = $state('');
	let messageType = $state('');
	let awardModalOpen = $state(false);
	let awardBadgeId = $state('');
	let awardUserId = $state('');

	// Edit badge modal
	let editBadgeModalOpen = $state(false);
	let editingBadge = $state<Badge | null>(null);
	let editBadgeForm = $state({
		name: '',
		description: '',
		trigger_type: 'count',
		trigger_value: 0,
		trigger_activity_key: '',
		is_active: true,
		display_order: 0
	});
	let savingEditBadge = $state(false);

	// Delete confirmation
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<{ kind: 'badge' | 'challenge'; id: string; name: string } | null>(null);
	let deleting = $state(false);

	let newBadge = $state({
		slug: '',
		name: '',
		description: '',
		trigger_type: 'count',
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
				createModalOpen = false;
				newBadge = { slug: '', name: '', description: '', trigger_type: 'count', trigger_value: 0, trigger_activity_key: '', is_active: true, display_order: 0 };
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
				createModalOpen = false;
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
		saving = true;
		message = '';
		try {
			const result = await api(`/admin/gamification/badges/${awardBadgeId}/award`, { method: 'POST', body: { user_id: awardUserId } });
			if (result.ok) {
				messageType = 'success';
				message = t('gamification.badge_awarded', 'Badge awarded successfully.');
				awardModalOpen = false;
				awardUserId = '';
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
		editBadgeModalOpen = true;
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
				editBadgeModalOpen = false;
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

	function openAwardBadge(badge: Badge) {
		awardBadgeId = badge.id;
		awardUserId = '';
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
		},
		{
			key: 'actions',
			label: t('common.actions', 'Actions'),
			class: 'w-32',
			componentSnippet: BadgeActionsCell
		}
	]);

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
		},
		{
			key: 'actions',
			label: t('common.actions', 'Actions'),
			class: 'w-24',
			componentSnippet: ChallengeActionsCell
		}
	]);

	// Ref: jc-admin-develop — reactive $effect so tab changes re-fetch
	$effect(() => {
		load();
	});
</script>

<PageHeader
	title={t('gamification.title', 'Gamification')}
	description={t('gamification.description', 'Manage badges, challenges, and leaderboard settings.')}
	actions={canManage ? [
		{ label: t('gamification.create_badge', 'Create Badge'), icon: Trophy, onclick: () => { createType = 'badge'; createModalOpen = true; } },
		{ label: t('gamification.create_challenge', 'Create Challenge'), icon: Target, onclick: () => { createType = 'challenge'; createModalOpen = true; } },
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
			searchable={true}
			searchPlaceholder={t('gamification.search_badges', 'Search badges...')}
			pageSize={25}
			emptyMessage={t('gamification.no_badges', 'No badges found. Create one to get started.')}
		/>
	{:else}
		<DataTable
			data={challenges}
			columns={challengeColumns}
			searchable={true}
			searchPlaceholder={t('gamification.search_challenges', 'Search challenges...')}
			pageSize={25}
			emptyMessage={t('gamification.no_challenges', 'No challenges found. Create one to get started.')}
		/>
	{/if}
{/if}

<!-- Create Modal (replaces inline div per jc-admin-develop: use Modal component) -->
<Modal bind:open={createModalOpen} title={createType === 'badge' ? t('gamification.create_badge_title', 'Create Badge') : t('gamification.create_challenge_title', 'Create Challenge')}>
	{#if createType === 'badge'}
		<div class="space-y-3">
			<div>
				<label for="badge-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</label>
				<input id="badge-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newBadge.name} placeholder="Top Contributor" />
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
						<option value="count">count</option>
						<option value="streak">streak</option>
						<option value="manual">manual</option>
					</select>
				</div>
				<div>
					<label for="badge-trigger-value" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.trigger_value', 'Trigger Value')}</label>
					<input id="badge-trigger-value" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newBadge.trigger_value} />
				</div>
			</div>
			<div>
				<label for="badge-activity" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.activity_key', 'Activity Key')}</label>
				<input id="badge-activity" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newBadge.trigger_activity_key} placeholder="post.create" />
			</div>
		</div>
	{:else}
		<div class="space-y-3">
			<div>
				<label for="challenge-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</label>
				<input id="challenge-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newChallenge.name} placeholder="30-Day Posting Streak" />
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
					<input id="challenge-activity" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newChallenge.activity_key} placeholder="post.create" />
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
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (createModalOpen = false)} disabled={saving}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={createType === 'badge' ? createBadge : createChallenge} disabled={saving}>
			{saving ? t('common.saving', 'Saving...') : t('common.create', 'Create')}
		</Button>
	</div>
</Modal>

<!-- Award Modal -->
<Modal bind:open={awardModalOpen} title={t('gamification.award_title', 'Award Badge')} size="sm">
	<div>
		<label for="award-user-id" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.user_id', 'User ID')}</label>
		<input id="award-user-id" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={awardUserId} placeholder="UUID" />
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (awardModalOpen = false)} disabled={saving}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={awardBadge} disabled={saving || !awardUserId}>
			{saving ? t('common.saving', 'Saving...') : t('gamification.award', 'Award Badge')}
		</Button>
	</div>
</Modal>

<!-- Edit Badge Modal -->
<Modal bind:open={editBadgeModalOpen} title={t('gamification.edit_badge_title', 'Edit Badge')}>
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
					<option value="count">count</option>
					<option value="streak">streak</option>
					<option value="manual">manual</option>
				</select>
			</div>
			<div>
				<label for="edit-badge-trigger-value" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.trigger_value', 'Trigger Value')}</label>
				<input id="edit-badge-trigger-value" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editBadgeForm.trigger_value} />
			</div>
		</div>
		<div>
			<label for="edit-badge-activity" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.activity_key', 'Activity Key')}</label>
			<input id="edit-badge-activity" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={editBadgeForm.trigger_activity_key} placeholder="post.create" />
		</div>
		<div class="flex items-center gap-2">
			<input type="checkbox" id="edit-badge-active" bind:checked={editBadgeForm.is_active} />
			<label for="edit-badge-active" class="text-sm" style="color: var(--color-foreground)">{t('gamification.active', 'Active')}</label>
		</div>
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (editBadgeModalOpen = false)} disabled={savingEditBadge}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={submitEditBadge} disabled={savingEditBadge}>
			{savingEditBadge ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
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
		<Button variant="outline" onclick={() => (showDeleteConfirm = false)} disabled={deleting}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmDelete} disabled={deleting}>
			<Trash2 class="mr-2 h-4 w-4" />
			{deleting ? t('common.deleting', 'Deleting...') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>

{#snippet BadgeActionsCell({ value, row }: { value: any; row: Badge })}
	<div class="flex items-center gap-1">
		{#if canManage}
			<Button variant="ghost" size="sm" onclick={() => openAwardBadge(row)} aria-label={t('gamification.award', 'Award')}>
				<Award class="h-3.5 w-3.5" />
			</Button>
			{#if !row.is_system}
				<Button variant="ghost" size="sm" onclick={() => openEditBadge(row)} aria-label={t('common.edit', 'Edit')}>
					<Pencil class="h-3.5 w-3.5" />
				</Button>
				<Button variant="ghost" size="sm" onclick={() => promptDeleteBadge(row)} style="color: var(--color-danger)" aria-label={t('common.delete', 'Delete')}>
					<Trash2 class="h-3.5 w-3.5" />
				</Button>
			{/if}
		{/if}
	</div>
{/snippet}

{#snippet ChallengeActionsCell({ value, row }: { value: any; row: Challenge })}
	<div class="flex items-center gap-1">
		{#if canManage}
			<Button variant="ghost" size="sm" onclick={() => promptDeleteChallenge(row)} style="color: var(--color-danger)" aria-label={t('common.delete', 'Delete')}>
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
