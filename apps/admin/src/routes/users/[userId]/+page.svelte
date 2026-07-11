<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import Modal from '$lib/components/Modal.svelte';
	import { api, userManagementApi, impersonationApi, setToken } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type UserDetail = {
		id: string;
		username: string;
		email: string;
		groups: string[];
		status: string;
		avatar_url?: string;
		created_at: string;
		last_login_at?: string;
	};

	type Activity = {
		type: string;
		content: string;
		created_at: string;
	};

	type AiContextEntry = {
		id: string;
		entry_type: string;
		content: string;
		created_at: string;
	};

	type Session = {
		id: string;
		ip_address: string;
		device: string;
		started_at: string;
	};

	let user = $state<UserDetail | null>(null);
	let activities = $state<Activity[]>([]);
	let aiEntries = $state<AiContextEntry[]>([]);
	let sessions = $state<Session[]>([]);
	let loading = $state(true);
	let activeTab = $state<'overview' | 'activity' | 'ai' | 'sessions'>('overview');

	// Modal states
	let showBanConfirm = $state(false);
	let showUnbanConfirm = $state(false);
	let showSuspendConfirm = $state(false);
	let showDeleteConfirm = $state(false);
	let showWarningModal = $state(false);
	let showGroupModal = $state(false);
	let warningReason = $state('');
	let selectedGroups = $state<string[]>([]);
	let groupOptions = $state<string[]>(['user', 'moderator', 'admin', 'super_admin']);
	// name → id, filled from listGroups; membership calls need group IDs.
	let groupIdsByName = $state<Record<string, string>>({});
	let actionError = $state('');
	let actionSuccess = $state('');

	// Impersonation
	let impersonating = $state(false);
	let impersonatingUsername = $state('');

	const userId = $derived($page.params.userId ?? '');

	async function loadUser() {
		if (!userId) return;
		loading = true;
		actionError = '';
		try {
			const result = await userManagementApi.getUser(userId);
			if (result.ok) {
				user = result.data;
				selectedGroups = [...result.data.groups];
			}
		} catch {
			actionError = t('users.detail.load_error');
		} finally {
			loading = false;
		}
	}

	async function loadActivity() {
		try {
			const result = await userManagementApi.getUserActivity(userId, 'limit=20');
			if (result.ok) activities = result.data.activities;
		} catch { /* ignore */ }
	}

	async function loadAiContext() {
		try {
			const result = await userManagementApi.getAiContext(userId);
			if (result.ok) aiEntries = result.data;
		} catch { /* ignore */ }
	}

	async function loadSessions() {
		try {
			const result = await userManagementApi.getUserSessions(userId);
			if (result.ok) sessions = result.data;
		} catch { /* ignore */ }
	}

	function switchTab(tab: typeof activeTab) {
		activeTab = tab;
		if (tab === 'activity') loadActivity();
		else if (tab === 'ai') loadAiContext();
		else if (tab === 'sessions') loadSessions();
	}

	function flash(msg: string, isError = false) {
		actionSuccess = isError ? '' : msg;
		actionError = isError ? msg : '';
		setTimeout(() => { actionSuccess = ''; actionError = ''; }, 3000);
	}

	async function handleBan() {
		showBanConfirm = false;
		const result = await userManagementApi.banUser(userId);
		if (result.ok) { flash(t('users.detail.ban_success')); loadUser(); }
		else flash(t('users.detail.action_error'), true);
	}

	async function handleUnban() {
		showUnbanConfirm = false;
		const result = await userManagementApi.unbanUser(userId);
		if (result.ok) { flash(t('users.detail.unban_success')); loadUser(); }
		else flash(t('users.detail.action_error'), true);
	}

	async function handleSuspend() {
		showSuspendConfirm = false;
		const result = await userManagementApi.suspendUser(userId, { status: 'suspended' });
		if (result.ok) { flash(t('users.detail.suspend_success')); loadUser(); }
		else flash(t('users.detail.action_error'), true);
	}

	async function handleDelete() {
		showDeleteConfirm = false;
		const result = await userManagementApi.deleteUser(userId);
		if (result.ok) {
			flash(t('users.detail.delete_success'));
			window.location.href = '/users';
		} else flash(t('users.detail.action_error'), true);
	}

	async function handleWarning() {
		showWarningModal = false;
		if (!warningReason.trim()) return;
		const result = await userManagementApi.issueWarning(userId, { reason: warningReason });
		if (result.ok) { flash(t('users.detail.warning_sent')); warningReason = ''; }
		else flash(t('users.detail.action_error'), true);
	}

	async function handleGroupChange() {
		showGroupModal = false;
		const current = new Set(user?.groups ?? []);
		const selected = new Set(selectedGroups);
		const added = selectedGroups.filter((g) => !current.has(g));
		const removed = [...current].filter((g) => !selected.has(g));

		let allOk = true;
		for (const name of added) {
			const groupId = groupIdsByName[name];
			if (!groupId) { allOk = false; continue; }
			const result = await userManagementApi.addGroupMember(groupId, userId);
			if (!result.ok) allOk = false;
		}
		for (const name of removed) {
			const groupId = groupIdsByName[name];
			if (!groupId) { allOk = false; continue; }
			const result = await userManagementApi.removeGroupMember(groupId, userId);
			if (!result.ok) allOk = false;
		}

		if (allOk) { flash(t('users.detail.group_changed')); loadUser(); }
		else flash(t('users.detail.action_error'), true);
	}

	async function deleteAiEntry(entryId: string) {
		const result = await userManagementApi.deleteAiContextEntry(userId, entryId);
		if (result.ok) {
			flash(t('users.detail.ai_entry_deleted'));
			aiEntries = aiEntries.filter(e => e.id !== entryId);
		} else flash(t('users.detail.ai_entry_delete_error'), true);
	}

	async function startImpersonate() {
		if (!user) return;
		const result = await impersonationApi.start(user.id);
		if (result.ok) {
			const data = result.data as { token: string };
			setToken(data.token);
			window.location.href = '/';
		} else flash(t('users.detail.action_error'), true);
	}

	onMount(async () => {
		await loadUser();
		const status = await impersonationApi.status();
		if (status.ok) {
			const d = status.data as { impersonating: boolean; username?: string };
			impersonating = d.impersonating;
			impersonatingUsername = d.username ?? '';
		}
		try {
			const groupsResult = await userManagementApi.listGroups();
			if (groupsResult.ok) {
				groupOptions = groupsResult.data.groups.map((g) => g.name);
				groupIdsByName = Object.fromEntries(
					groupsResult.data.groups.map((g) => [g.name, g.id])
				);
			}
		} catch {
			// Keep default group options
		}
	});
</script>

<svelte:head>
	<title>{user?.username ?? 'User'} — JapaConnect Admin</title>
</svelte:head>

<!-- Impersonation Banner -->
{#if impersonating}
	<div class="mb-4 flex items-center justify-between rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-warning); background-color: color-mix(in srgb, var(--color-warning) 10%, transparent)">
		<span style="color: var(--color-warning)">Impersonating <strong>{impersonatingUsername}</strong></span>
		<a href="/users" class="rounded-md border px-3 py-1 text-xs font-medium" style="border-color: var(--color-warning); color: var(--color-warning)">{t('common.back')}</a>
	</div>
{/if}

<a href="/users" class="mb-4 inline-flex items-center gap-1 text-sm font-medium transition-colors hover:underline" style="color: var(--color-primary)">
	&larr; {t('common.back')}
</a>

{#if actionSuccess}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
		{actionSuccess}
	</div>
{/if}

{#if actionError}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{actionError}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if user}
	<!-- User Header -->
	<div class="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
		<div class="flex items-center gap-4">
			<div class="flex h-16 w-16 items-center justify-center rounded-2xl text-2xl font-bold text-white" style="background: var(--gradient-primary)">
				{user.username.charAt(0).toUpperCase()}
			</div>
			<div>
				<h1 class="text-2xl font-bold tracking-tight">{user.username}</h1>
				<p class="text-sm" style="color: var(--color-muted)">{user.email}</p>
				<div class="mt-1 flex items-center gap-2">
					<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {user.status === 'active' ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : user.status === 'banned' ? 'bg-red-500/15 text-red-700 dark:text-red-400' : 'bg-amber-500/15 text-amber-700 dark:text-amber-400'}">
						{user.status}
					</span>
					{#each user.groups as g}
						<span class="inline-block rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-white/10">{g}</span>
					{/each}
				</div>
			</div>
		</div>

		<!-- Actions -->
		<div class="flex flex-wrap gap-2">
			<button class="btn btn-secondary text-xs" onclick={() => { showWarningModal = true; warningReason = ''; }}>
				{t('users.detail.warning')}
			</button>
			{#if user.status === 'active'}
				<button class="btn btn-secondary text-xs" onclick={() => showBanConfirm = true} style="color: var(--color-danger)">
					{t('users.detail.ban')}
				</button>
				<button class="btn btn-secondary text-xs" onclick={() => showSuspendConfirm = true} style="color: var(--color-warning)">
					{t('users.detail.suspend')}
				</button>
			{:else if user.status === 'banned'}
				<button class="btn btn-secondary text-xs" onclick={() => showUnbanConfirm = true}>
					{t('users.detail.unban')}
				</button>
			{/if}
			<button class="btn btn-secondary text-xs" onclick={() => { selectedGroups = [...user!.groups]; showGroupModal = true; }}>
				{t('users.detail.change_group')}
			</button>
			<button class="btn btn-secondary text-xs" onclick={startImpersonate}>
				{t('users.detail.impersonate')}
			</button>
			<button class="btn btn-secondary text-xs" onclick={() => showDeleteConfirm = true} style="color: var(--color-danger)">
				{t('users.detail.delete')}
			</button>
		</div>
	</div>

	<!-- Tabs -->
	<div class="mb-4 flex gap-1 rounded-lg p-1" style="background: var(--color-surface)">
		{#each [
			{ key: 'overview', label: 'users.detail.overview' },
			{ key: 'activity', label: 'users.detail.activity' },
			{ key: 'ai', label: 'users.detail.ai_context' },
			{ key: 'sessions', label: 'users.detail.sessions' }
		] as tab}
			<button
				class="rounded-md px-3 py-1.5 text-sm font-medium transition-all"
				style="background: {activeTab === tab.key ? 'var(--color-primary)' : 'transparent'}; color: {activeTab === tab.key ? '#fff' : 'var(--color-muted)'}"
				onclick={() => switchTab(tab.key as typeof activeTab)}
			>
				{t(tab.label)}
			</button>
		{/each}
	</div>

	<!-- Tab Content -->
	<div class="rounded-2xl border p-5" style="border-color: var(--color-border); background: var(--gradient-surface)">
		{#if activeTab === 'overview'}
			<div class="space-y-4">
				<div class="grid gap-4 sm:grid-cols-2">
					<div>
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.detail.username')}</p>
						<p class="mt-1 text-sm font-medium">{user.username}</p>
					</div>
					<div>
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.detail.email')}</p>
						<p class="mt-1 text-sm font-medium">{user.email}</p>
					</div>
					<div>
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.detail.status')}</p>
						<p class="mt-1 text-sm font-medium">{user.status}</p>
					</div>
					<div>
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.detail.created')}</p>
						<p class="mt-1 text-sm font-medium">{user.created_at}</p>
					</div>
					<div>
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.detail.last_login')}</p>
						<p class="mt-1 text-sm font-medium">{user.last_login_at ?? '—'}</p>
					</div>
					<div>
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('users.detail.groups')}</p>
						<div class="mt-1 flex flex-wrap gap-1">
							{#each user.groups as g}
								<span class="inline-block rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-white/10">{g}</span>
							{/each}
						</div>
					</div>
				</div>
			</div>

		{:else if activeTab === 'activity'}
			{#if activities.length > 0}
				<div class="overflow-x-auto">
					<table class="w-full text-sm">
						<thead>
							<tr>
								<th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">{t('users.detail.activity_type')}</th>
								<th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">{t('users.detail.activity_content')}</th>
								<th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">{t('users.detail.activity_date')}</th>
							</tr>
						</thead>
						<tbody>
							{#each activities as act}
								<tr class="border-t" style="border-color: var(--color-border)">
									<td class="px-3 py-2">
										<span class="inline-block rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-white/10">{act.type}</span>
									</td>
									<td class="px-3 py-2 text-xs" style="color: var(--color-muted)">{act.content}</td>
									<td class="px-3 py-2 text-xs" style="color: var(--color-muted)">{act.created_at}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{:else}
				<p class="py-4 text-center text-sm" style="color: var(--color-muted)">{t('users.detail.no_activity')}</p>
			{/if}

		{:else if activeTab === 'ai'}
			{#if aiEntries.length > 0}
				<div class="overflow-x-auto">
					<table class="w-full text-sm">
						<thead>
							<tr>
								<th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">{t('users.detail.ai_entry_type')}</th>
								<th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">{t('users.detail.ai_entry_content')}</th>
								<th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">{t('users.detail.ai_entry_created')}</th>
								<th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)"></th>
							</tr>
						</thead>
						<tbody>
							{#each aiEntries as entry}
								<tr class="border-t" style="border-color: var(--color-border)">
									<td class="px-3 py-2">
										<span class="inline-block rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-white/10">{entry.entry_type}</span>
									</td>
									<td class="px-3 py-2 text-xs" style="color: var(--color-muted)">{entry.content}</td>
									<td class="px-3 py-2 text-xs" style="color: var(--color-muted)">{entry.created_at}</td>
									<td class="px-3 py-2">
										<button
											class="rounded px-2 py-1 text-xs transition-colors hover:bg-black/5 dark:hover:bg-white/10"
											style="color: var(--color-danger)"
											onclick={() => deleteAiEntry(entry.id)}
										>
											{t('users.detail.ai_entry_delete')}
										</button>
									</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{:else}
				<p class="py-4 text-center text-sm" style="color: var(--color-muted)">{t('users.detail.no_ai_context')}</p>
			{/if}

		{:else if activeTab === 'sessions'}
			{#if sessions.length > 0}
				<div class="overflow-x-auto">
					<table class="w-full text-sm">
						<thead>
							<tr>
								<th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">{t('users.detail.session_ip')}</th>
								<th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">{t('users.detail.session_device')}</th>
								<th class="px-3 py-2 text-left text-xs font-medium" style="color: var(--color-muted)">{t('users.detail.session_started')}</th>
							</tr>
						</thead>
						<tbody>
							{#each sessions as sess}
								<tr class="border-t" style="border-color: var(--color-border)">
									<td class="px-3 py-2 font-mono text-xs">{sess.ip_address}</td>
									<td class="px-3 py-2 text-xs" style="color: var(--color-muted)">{sess.device}</td>
									<td class="px-3 py-2 text-xs" style="color: var(--color-muted)">{sess.started_at}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{:else}
				<p class="py-4 text-center text-sm" style="color: var(--color-muted)">{t('users.detail.no_sessions')}</p>
			{/if}
		{/if}
	</div>
{/if}

<!-- Ban Confirm Modal -->
<Modal bind:open={showBanConfirm} title={t('users.detail.ban')} onConfirm={handleBan} confirmLabel={t('users.detail.ban')}>
	<p class="text-sm" style="color: var(--color-muted)">{t('users.detail.ban_confirm')}</p>
</Modal>

<!-- Unban Confirm Modal -->
<Modal bind:open={showUnbanConfirm} title={t('users.detail.unban')} onConfirm={handleUnban} confirmLabel={t('users.detail.unban')}>
	<p class="text-sm" style="color: var(--color-muted)">{t('users.detail.unban_confirm')}</p>
</Modal>

<!-- Suspend Confirm Modal -->
<Modal bind:open={showSuspendConfirm} title={t('users.detail.suspend')} onConfirm={handleSuspend} confirmLabel={t('users.detail.suspend')}>
	<p class="text-sm" style="color: var(--color-muted)">{t('users.detail.suspend_confirm')}</p>
</Modal>

<!-- Delete Confirm Modal -->
<Modal bind:open={showDeleteConfirm} title={t('users.detail.delete')} onConfirm={handleDelete} confirmLabel={t('users.detail.delete')}>
	<p class="text-sm" style="color: var(--color-muted)">{t('users.detail.delete_confirm')}</p>
</Modal>

<!-- Warning Modal -->
<Modal bind:open={showWarningModal} title={t('users.detail.warning')} onConfirm={handleWarning} confirmLabel={t('common.confirm')}>
	<div>
		<label for="warning-reason" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('ai.reject_reason')}</label>
		<textarea
			id="warning-reason"
			rows="3"
			class="w-full rounded-md border px-3 py-2 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
			placeholder={t('ai.reject_reason_placeholder')}
			bind:value={warningReason}
		></textarea>
	</div>
</Modal>

<!-- Change Group Modal -->
<Modal bind:open={showGroupModal} title={t('users.detail.change_group')} onConfirm={handleGroupChange} confirmLabel={t('common.save')}>
	<div class="space-y-2">
		{#each groupOptions as group}
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" value={group} bind:group={selectedGroups} class="rounded" />
				{group}
			</label>
		{/each}
	</div>
</Modal>
