<script lang="ts">
	import { Building2, Users, Mail, Trash2, Save, Loader2, Shield, UserMinus } from 'lucide-svelte';
	import { api, type Organization, type OrganizationMember, type OrganizationInvitation } from '$lib/api';
	import Badge from '$lib/components/ui/Badge.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';

	let { data } = $props();

	let org = $state<Organization>({} as Organization);
	let members = $state<OrganizationMember[]>([]);
	let invitations = $state<OrganizationInvitation[]>([]);

	// Edit state
	let editing = $state(false);
	let editName = $state('');
	let editHeadline = $state('');
	let editBio = $state('');
	let editPublic = $state(false);

	$effect(() => {
		org = data.org;
		members = data.members;
		invitations = data.invitations as OrganizationInvitation[];
		editName = data.org.name;
		editHeadline = data.org.headline ?? '';
		editBio = data.org.bio ?? '';
		editPublic = data.org.is_public;
	});
	let saving = $state(false);
	let saveMessage = $state('');

	// Invite state
	let inviteEmail = $state('');
	let inviteRole = $state('member');
	let inviting = $state(false);

	async function saveSettings(e: Event) {
		e.preventDefault();
		saving = true;
		saveMessage = '';
		try {
			await api.updateOrganization(org.id, {
				name: editName,
				headline: editHeadline || undefined,
				bio: editBio || undefined,
				is_public: editPublic
			});
			org = { ...org, name: editName, headline: editHeadline, bio: editBio, is_public: editPublic };
			editing = false;
			saveMessage = 'Settings saved.';
		} catch {
			saveMessage = 'Failed to save settings.';
		} finally {
			saving = false;
		}
	}

	async function sendInvite(e: Event) {
		e.preventDefault();
		if (!inviteEmail.trim()) return;
		inviting = true;
		try {
			await api.inviteToOrganization(org.id, inviteEmail.trim(), inviteRole);
			inviteEmail = '';
			// Refresh invitations
			const result = await api.listOrganizationInvitations(org.id);
			invitations = result ?? [];
		} catch {
			// ignore
		} finally {
			inviting = false;
		}
	}

	async function removeMember(userId: string) {
		try {
			await api.removeOrganizationMember(org.id, userId);
			members = members.filter((m) => m.user_id !== userId);
		} catch {
			// ignore
		}
	}

	async function updateMemberRole(userId: string, role: string) {
		try {
			await api.updateOrganizationMember(org.id, userId, role);
			members = members.map((m) => m.user_id === userId ? { ...m, role } : m);
		} catch {
			// ignore
		}
	}

	function roleVariant(role: string): 'info' | 'warning' | 'default' {
		switch (role) {
			case 'owner': return 'info';
			case 'admin': return 'warning';
			default: return 'default';
		}
	}
</script>

<svelte:head>
	<title>Manage {org.name} — JapaConnect</title>
</svelte:head>

<div class="mx-auto max-w-3xl space-y-8">
	<!-- Header -->
	<div class="flex items-center gap-3">
		<a href="/organisations/{org.slug}" class="text-sm hover:underline" style="color: var(--color-primary)">
			{org.name}
		</a>
		<span style="color: var(--color-muted)">/</span>
		<h1 class="text-xl font-bold tracking-tight">Settings</h1>
	</div>

	{#if saveMessage}
		<div class="rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
			{saveMessage}
		</div>
	{/if}

	<!-- Settings -->
	<section class="rounded-lg border p-6" style="border-color: var(--color-border)">
		<h2 class="mb-4 flex items-center gap-2 text-lg font-semibold">
			<Building2 class="h-5 w-5" />
			Organisation Settings
		</h2>

		{#if editing}
			<form onsubmit={saveSettings} class="space-y-4">
				<div>
					<label for="org-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Name</label>
					<input id="org-name" type="text" bind:value={editName} class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" />
				</div>
				<div>
					<label for="org-headline" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Headline</label>
					<input id="org-headline" type="text" bind:value={editHeadline} class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" />
				</div>
				<div>
					<label for="org-bio" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Bio</label>
					<textarea id="org-bio" bind:value={editBio} rows={3} class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)"></textarea>
				</div>
				<div class="flex items-center gap-2">
					<input id="org-public" type="checkbox" bind:checked={editPublic} class="rounded" />
					<label for="org-public" class="text-sm">Public organisation</label>
				</div>
				<div class="flex gap-2">
					<button type="submit" disabled={saving} class="flex items-center gap-1.5 rounded-md px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50" style="background-color: var(--color-primary)">
						{#if saving}<Loader2 class="h-4 w-4 animate-spin" />{:else}<Save class="h-4 w-4" />{/if}
						Save
					</button>
					<button type="button" onclick={() => (editing = false)} class="rounded-md border px-4 py-2 text-sm" style="border-color: var(--color-border)">
						Cancel
					</button>
				</div>
			</form>
		{:else}
			<div class="space-y-3">
				<div>
					<p class="text-xs font-medium" style="color: var(--color-muted)">Name</p>
					<p class="text-sm">{org.name}</p>
				</div>
				{#if org.headline}
					<div>
						<p class="text-xs font-medium" style="color: var(--color-muted)">Headline</p>
						<p class="text-sm">{org.headline}</p>
					</div>
				{/if}
				{#if org.bio}
					<div>
						<p class="text-xs font-medium" style="color: var(--color-muted)">Bio</p>
						<p class="text-sm">{org.bio}</p>
					</div>
				{/if}
				<div>
					<p class="text-xs font-medium" style="color: var(--color-muted)">Visibility</p>
					<p class="text-sm">{org.is_public ? 'Public' : 'Private'}</p>
				</div>
				<button onclick={() => (editing = true)} class="mt-2 rounded-md border px-4 py-2 text-sm font-medium hover:bg-black/5 dark:hover:bg-white/10" style="border-color: var(--color-border)">
					Edit Settings
				</button>
			</div>
		{/if}
	</section>

	<!-- Members -->
	<section class="rounded-lg border p-6" style="border-color: var(--color-border)">
		<h2 class="mb-4 flex items-center gap-2 text-lg font-semibold">
			<Users class="h-5 w-5" />
			Members ({members.length})
		</h2>

		{#if members.length === 0}
			<EmptyState title="No members" message="Invite people to join this organisation." />
		{:else}
			<div class="space-y-2">
				{#each members as member}
					<div class="flex items-center justify-between rounded-md border px-4 py-3" style="border-color: var(--color-border)">
						<div class="flex items-center gap-3">
							{#if member.avatar_url}
								<img src={member.avatar_url} alt="" class="h-8 w-8 rounded-full object-cover" />
							{:else}
								<div class="flex h-8 w-8 items-center justify-center rounded-full text-xs font-medium text-white" style="background-color: var(--color-primary)">
									{(member.display_name ?? member.username ?? 'U').charAt(0).toUpperCase()}
								</div>
							{/if}
							<div>
								<p class="text-sm font-medium">{member.display_name ?? member.username}</p>
								<p class="text-xs" style="color: var(--color-muted)">@{member.username}</p>
							</div>
							<Badge variant={roleVariant(member.role)}>{member.role}</Badge>
						</div>
						<div class="flex items-center gap-2">
							{#if member.role !== 'owner'}
								<select
									class="rounded border px-2 py-1 text-xs"
									style="border-color: var(--color-border); background-color: var(--color-surface)"
									value={member.role}
									onchange={(e) => updateMemberRole(member.user_id, (e.target as HTMLSelectElement).value)}
								>
									<option value="member">Member</option>
									<option value="admin">Admin</option>
								</select>
								<button
									class="rounded p-1 hover:bg-red-50 dark:hover:bg-red-950"
									style="color: var(--color-danger)"
									onclick={() => removeMember(member.user_id)}
									aria-label="Remove member"
								>
									<UserMinus class="h-4 w-4" />
								</button>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</section>

	<!-- Invitations -->
	<section class="rounded-lg border p-6" style="border-color: var(--color-border)">
		<h2 class="mb-4 flex items-center gap-2 text-lg font-semibold">
			<Mail class="h-5 w-5" />
			Invitations
		</h2>

		<!-- Invite form -->
		<form onsubmit={sendInvite} class="mb-4 flex gap-2">
			<input
				type="email"
				placeholder="Email address"
				bind:value={inviteEmail}
				class="flex-1 rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				required
			/>
			<select
				bind:value={inviteRole}
				class="rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
			>
				<option value="member">Member</option>
				<option value="admin">Admin</option>
			</select>
			<button type="submit" disabled={inviting || !inviteEmail.trim()} class="flex items-center gap-1.5 rounded-md px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50" style="background-color: var(--color-primary)">
				{#if inviting}<Loader2 class="h-4 w-4 animate-spin" />{:else}Invite{/if}
			</button>
		</form>

		{#if invitations.length === 0}
			<p class="py-4 text-center text-xs" style="color: var(--color-muted)">No pending invitations.</p>
		{:else}
			<div class="space-y-1">
				{#each invitations as inv}
					<div class="flex items-center justify-between rounded px-3 py-2 text-sm">
						<span>{inv.email}</span>
						<div class="flex items-center gap-2">
							<Badge variant="default">{inv.role}</Badge>
							<span class="text-xs" style="color: var(--color-muted)">sent {inv.created_at}</span>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</section>
</div>
