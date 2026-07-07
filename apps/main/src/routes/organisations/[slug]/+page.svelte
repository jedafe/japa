<script lang="ts">
	import { Building2, BadgeCheck, Users, Mail, UserPlus, Loader2, Shield } from 'lucide-svelte';
	import { api } from '$lib/api';

	let { data } = $props();

	let org = $state(data.org);
	let members = $state(data.members);

	// Invitation state
	let showInvite = $state(false);
	let inviteEmail = $state('');
	let inviteRole = $state('member');
	let inviting = $state(false);
	let inviteMessage = $state('');

	async function sendInvite(e: Event) {
		e.preventDefault();
		if (!inviteEmail.trim()) return;
		inviting = true;
		inviteMessage = '';
		try {
			await api.inviteToOrganization(org.id, inviteEmail.trim(), inviteRole);
			inviteMessage = `Invitation sent to ${inviteEmail}`;
			inviteEmail = '';
			showInvite = false;
		} catch {
			inviteMessage = 'Failed to send invitation — check your permissions.';
		} finally {
			inviting = false;
		}
	}

	async function removeMember(userId: string) {
		try {
			await api.removeOrganizationMember(org.id, userId);
			members = members.filter((m) => m.user_id !== userId);
		} catch {
			// ignore — likely a permissions error
		}
	}

	// Ownership claim (for unclaimed / seeded organisations)
	let showClaim = $state(false);
	let claimEvidence = $state('');
	let claiming = $state(false);
	let claimMessage = $state('');

	async function submitClaim(e: Event) {
		e.preventDefault();
		if (!claimEvidence.trim()) return;
		claiming = true;
		claimMessage = '';
		try {
			await api.submitOwnershipClaim({
				entity_type: 'organization',
				entity_id: org.id,
				evidence: claimEvidence.trim()
			});
			claimMessage = 'Claim submitted — our team will review it shortly.';
			claimEvidence = '';
			showClaim = false;
		} catch {
			claimMessage = 'Failed to submit claim. Please try again.';
		} finally {
			claiming = false;
		}
	}

	const roleColors: Record<string, string> = {
		owner: 'var(--color-primary)',
		admin: 'var(--color-warning, #f59e0b)',
		member: 'var(--color-muted)'
	};
</script>

<svelte:head>
	<title>{org.name} | JapaConnect</title>
	<meta name="description" content={org.headline ?? `${org.name} on JapaConnect`} />
</svelte:head>

<main class="mx-auto max-w-4xl px-4 py-8">
	<!-- Header -->
	<header
		class="mb-8 rounded-2xl border p-6"
		style="border-color: var(--color-border); background: var(--color-surface)"
	>
		<div class="flex items-start gap-4">
			{#if org.logo_url}
				<img src={org.logo_url} alt={org.name} class="h-16 w-16 rounded-xl object-cover" />
			{:else}
				<div
					class="flex h-16 w-16 shrink-0 items-center justify-center rounded-xl text-xl font-bold text-white"
					style="background: var(--gradient-primary, var(--color-primary))"
				>
					{org.name.charAt(0).toUpperCase()}
				</div>
			{/if}
			<div class="min-w-0 flex-1">
				<div class="flex items-center gap-2">
					<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">{org.name}</h1>
					{#if org.is_public}
						<BadgeCheck class="h-5 w-5" style="color: var(--color-primary)" />
					{/if}
				</div>
				{#if org.headline}
					<p class="mt-1 text-sm" style="color: var(--color-muted)">{org.headline}</p>
				{/if}
				<div class="mt-2 flex items-center gap-4 text-xs" style="color: var(--color-muted)">
					<span class="flex items-center gap-1">
						<Users class="h-3.5 w-3.5" />
						{(org.member_count ?? members.length).toLocaleString()} members
					</span>
				</div>
			</div>
		</div>
		{#if org.bio}
			<p class="mt-4 text-sm leading-relaxed" style="color: var(--color-foreground)">{org.bio}</p>
		{/if}
	</header>

	<!-- Members -->
	<section
		class="rounded-2xl border p-6"
		style="border-color: var(--color-border); background: var(--color-surface)"
	>
		<div class="mb-4 flex items-center justify-between">
			<h2 class="flex items-center gap-2 text-lg font-semibold" style="color: var(--color-foreground)">
				<Users class="h-5 w-5" /> Members
			</h2>
			<button
				onclick={() => (showInvite = !showInvite)}
				class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
				style="background: var(--color-primary)"
			>
				<UserPlus class="h-4 w-4" /> Invite
			</button>
		</div>

		{#if inviteMessage}
			<p class="mb-3 text-sm" style="color: var(--color-primary)">{inviteMessage}</p>
		{/if}

		{#if showInvite}
			<form
				onsubmit={sendInvite}
				class="mb-4 flex flex-col gap-2 rounded-xl border p-4 sm:flex-row sm:items-end"
				style="border-color: var(--color-border); background: var(--color-background)"
			>
				<div class="flex-1">
					<label for="invite-email" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">
						Email
					</label>
					<div class="relative">
						<Mail class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
						<input
							id="invite-email"
							type="email"
							bind:value={inviteEmail}
							required
							placeholder="colleague@example.com"
							class="w-full rounded-lg border py-2 pl-9 pr-3 text-sm outline-none focus:border-[var(--color-primary)]"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						/>
					</div>
				</div>
				<div>
					<label for="invite-role" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">
						Role
					</label>
					<select
						id="invite-role"
						bind:value={inviteRole}
						class="rounded-lg border px-3 py-2 text-sm outline-none"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					>
						<option value="member">Member</option>
						<option value="admin">Admin</option>
					</select>
				</div>
				<button
					type="submit"
					disabled={inviting}
					class="rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background: var(--color-primary)"
				>
					{#if inviting}
						<Loader2 class="h-4 w-4 animate-spin" />
					{:else}
						Send
					{/if}
				</button>
			</form>
		{/if}

		{#if members.length === 0}
			<p class="py-6 text-center text-sm" style="color: var(--color-muted)">No members yet.</p>
		{:else}
			<ul class="divide-y" style="border-color: var(--color-border)">
				{#each members as member (member.user_id)}
					<li class="flex items-center gap-3 py-3">
						<a href="/profile/{member.username}" class="flex min-w-0 flex-1 items-center gap-3">
							{#if member.avatar_url}
								<img src={member.avatar_url} alt={member.username} class="h-9 w-9 rounded-full object-cover" />
							{:else}
								<div
									class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-bold text-white"
									style="background: var(--color-primary)"
								>
									{(member.display_name ?? member.username).charAt(0).toUpperCase()}
								</div>
							{/if}
							<div class="min-w-0">
								<p class="truncate text-sm font-medium" style="color: var(--color-foreground)">
									{member.display_name ?? member.username}
								</p>
								<p class="truncate text-xs" style="color: var(--color-muted)">@{member.username}</p>
							</div>
						</a>
						<span
							class="flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium capitalize"
							style="background: color-mix(in srgb, {roleColors[member.role] ?? 'var(--color-muted)'} 12%, transparent); color: {roleColors[member.role] ?? 'var(--color-muted)'}"
						>
							{#if member.role === 'owner' || member.role === 'admin'}
								<Shield class="h-3 w-3" />
							{/if}
							{member.role}
						</span>
						{#if member.role !== 'owner'}
							<button
								onclick={() => removeMember(member.user_id)}
								class="text-xs transition-colors hover:underline"
								style="color: var(--color-destructive, #ef4444)"
							>
								Remove
							</button>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</section>

	<!-- Ownership claim (for unclaimed organisations) -->
	<section class="mt-4 rounded-2xl border p-5" style="border-color: var(--color-border); background: var(--color-surface)">
		<div class="flex items-center justify-between">
			<div>
				<h2 class="text-sm font-semibold" style="color: var(--color-foreground)">Is this your organisation?</h2>
				<p class="mt-0.5 text-xs" style="color: var(--color-muted)">
					Claim ownership to manage members, invitations, and the profile.
				</p>
			</div>
			<button
				onclick={() => (showClaim = !showClaim)}
				class="shrink-0 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors"
				style="border-color: var(--color-primary); color: var(--color-primary)"
			>
				Claim ownership
			</button>
		</div>

		{#if claimMessage}
			<p class="mt-3 text-sm" style="color: var(--color-primary)">{claimMessage}</p>
		{/if}

		{#if showClaim}
			<form onsubmit={submitClaim} class="mt-3 space-y-2">
				<label for="claim-evidence" class="block text-xs font-medium" style="color: var(--color-muted)">
					Evidence of ownership (work email, official document reference, etc.)
				</label>
				<textarea
					id="claim-evidence"
					bind:value={claimEvidence}
					required
					rows="3"
					placeholder="Describe how you're affiliated with this organisation…"
					class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
					style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
				></textarea>
				<button
					type="submit"
					disabled={claiming}
					class="rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background: var(--color-primary)"
				>
					{#if claiming}<Loader2 class="inline h-4 w-4 animate-spin" />{:else}Submit claim{/if}
				</button>
			</form>
		{/if}
	</section>
</main>
