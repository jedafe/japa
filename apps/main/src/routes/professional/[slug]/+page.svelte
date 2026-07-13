<script lang="ts">
	import { ArrowLeft, Pencil, Briefcase, MapPin, FileText, Shield, Plus, Loader2, Trash2, UserPlus, Users, Heart } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { isFeatureEnabled } from '$lib/stores/features.svelte';
	import Reviews from '$lib/components/Reviews.svelte';

	let { data } = $props();
	let profile = $derived(data.profile);
	let currentUser = $state<string | null>(null);

	// svelte-ignore state_referenced_locally
	let relocations = $state(profile.relocations || []);
	// svelte-ignore state_referenced_locally
	let documents = $state(profile.documents || []);
	// svelte-ignore state_referenced_locally
	let verifiers = $state(profile.verifiers || []);

	let showRelocationForm = $state(false);
	let showDocumentForm = $state(false);
	let relocationCountry = $state('');
	let relocationCity = $state('');
	let relocationStatus = $state('planning');
	let relocationYear = $state('');
	let docType = $state('');
	let docTitle = $state('');
	let docDescription = $state('');
	let formLoading = $state(false);
	let formError = $state('');

	// Members
	let members = $state<Array<{ user_id: string; username: string; display_name: string; role: string; joined_at: string }>>([]);
	let showMemberForm = $state(false);
	let memberUsername = $state('');
	let memberRole = $state('member');
	let loadingMembers = $state(false);

	// Follow
	let isFollowing = $state(false);
	let followLoading = $state(false);

	const relocationStatuses = [
		{ value: 'planning', label: 'Planning' },
		{ value: 'applied', label: 'Applied' },
		{ value: 'approved', label: 'Approved' },
		{ value: 'relocated', label: 'Relocated' }
	];

	const isOwner = $derived(currentUser === profile.author_user_id);

	async function addRelocation(e: Event) {
		e.preventDefault();
		formLoading = true;
		formError = '';
		try {
			const reloData: { country: string; status: string; city?: string; year?: number } = {
				country: relocationCountry,
				status: relocationStatus
			};
			if (relocationCity.trim()) reloData.city = relocationCity.trim();
			if (relocationYear.trim()) reloData.year = parseInt(relocationYear, 10);

			const result = await api.addRelocation(profile.id, reloData);
			relocations = [...relocations, result];
			showRelocationForm = false;
			relocationCountry = '';
			relocationCity = '';
			relocationStatus = 'planning';
			relocationYear = '';
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Network error. Please try again.';
		} finally {
			formLoading = false;
		}
	}

	async function addDocument(e: Event) {
		e.preventDefault();
		formLoading = true;
		formError = '';
		try {
			const docData: { document_type: string; title: string; description?: string } = {
				document_type: docType,
				title: docTitle
			};
			if (docDescription.trim()) docData.description = docDescription.trim();

			const result = await api.addDocument(profile.id, docData);
			documents = [...documents, result];
			showDocumentForm = false;
			docType = '';
			docTitle = '';
			docDescription = '';
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Network error. Please try again.';
		} finally {
			formLoading = false;
		}
	}

	async function toggleFollow() {
		if (followLoading) return;
		followLoading = true;
		try {
			if (isFollowing) {
				await api.unfollowProfile(profile.id);
				isFollowing = false;
			} else {
				await api.followProfile(profile.id);
				isFollowing = true;
			}
		} catch {
			// silently fail
		} finally {
			followLoading = false;
		}
	}

	async function loadMembers() {
		loadingMembers = true;
		try {
			const result = await api.listProfileMembers(profile.id);
			members = Array.isArray(result) ? result : [];
		} catch {
			members = [];
		} finally {
			loadingMembers = false;
		}
	}

	async function addMember(e: Event) {
		e.preventDefault();
		formLoading = true;
		formError = '';
		try {
			await api.addProfileMember(profile.id, { user_id: memberUsername, role: memberRole });
			await loadMembers();
			showMemberForm = false;
			memberUsername = '';
			memberRole = 'member';
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to add member';
		} finally {
			formLoading = false;
		}
	}

	async function removeMember(userId: string) {
		if (!confirm('Remove this member?')) return;
		try {
			await api.removeProfileMember(profile.id, userId);
			await loadMembers();
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to remove member';
		}
	}
</script>

<svelte:head>
	<title>{profile.name} | Professional Profile | JapaConnect</title>
	<meta name="description" content={profile.headline} />
</svelte:head>

{#if !isFeatureEnabled('feature.professional')}
	<main class="mx-auto max-w-2xl px-6 py-16">
		<div class="text-center">
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">Professional Profiles is not available</h1>
			<p class="mt-2" style="color: var(--color-muted)">This feature is currently disabled.</p>
			<a href="/" class="mt-4 inline-block" style="color: var(--color-primary)">Go home</a>
		</div>
	</main>
{:else}
	<main class="mx-auto max-w-3xl px-4 py-8">
	<a
		href="/professional"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to profiles
	</a>

	<!-- Profile Header -->
	<div class="rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
		<header class="flex items-start justify-between">
			<div class="flex items-start gap-4">
				<div
					class="flex h-14 w-14 items-center justify-center rounded-full text-lg font-bold text-white"
					style="background-color: var(--color-primary)"
				>
					{profile.name.charAt(0).toUpperCase()}
				</div>
				<div>
					<h1 class="text-xl font-bold" style="color: var(--color-foreground)">
						{profile.name}
					</h1>
					<p class="text-sm" style="color: var(--color-muted)">{profile.headline}</p>
					<div class="mt-2 flex items-center gap-3 text-xs" style="color: var(--color-muted)">
						<span
							class="rounded-full px-2 py-0.5"
							style="background-color: {profile.is_public ? 'color-mix(in srgb, var(--color-success, #16a34a) 15%, transparent)' : 'var(--color-background)'}; color: {profile.is_public ? 'var(--color-success, #16a34a)' : 'var(--color-muted)'}"
						>
							{profile.is_public ? 'Public' : 'Private'}
						</span>
					</div>
				</div>
			</div>
			{#if isOwner}
				<a
					href="/professional/{profile.slug}/edit"
					class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
					style="background-color: var(--color-background); color: var(--color-foreground); border: 1px solid var(--color-border)"
				>
					<Pencil class="h-3.5 w-3.5" />
					Edit
				</a>
			{:else}
				<button
					onclick={toggleFollow}
					disabled={followLoading}
					class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors disabled:opacity-50"
					style="background-color: {isFollowing ? 'var(--color-primary)' : 'var(--color-background)'}; color: {isFollowing ? 'white' : 'var(--color-foreground)'}; border: 1px solid {isFollowing ? 'var(--color-primary)' : 'var(--color-border)'}"
				>
					<Heart class="h-3.5 w-3.5" fill={isFollowing ? 'currentColor' : 'none'} />
					{isFollowing ? 'Following' : 'Follow'}
				</button>
			{/if}
		</header>

		{#if profile.skills.length > 0}
			<div class="mt-4 flex flex-wrap gap-1.5">
				{#each profile.skills as skill}
					<span
						class="rounded-full px-2.5 py-1 text-xs font-medium"
						style="background-color: var(--color-primary); color: white"
					>
						{skill}
					</span>
				{/each}
			</div>
		{/if}
	</div>

	{#if formError}
		<div
			class="mt-4 rounded-lg border px-4 py-3 text-sm"
			style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
		>
			{formError}
		</div>
	{/if}

	<!-- Relocations Section -->
	<section class="mt-6 rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
		<div class="flex items-center justify-between">
			<h2 class="flex items-center gap-2 text-lg font-semibold" style="color: var(--color-foreground)">
				<MapPin class="h-5 w-5" style="color: var(--color-primary)" />
				Relocation Timeline
			</h2>
			{#if isOwner}
				<button
					onclick={() => (showRelocationForm = !showRelocationForm)}
					class="flex items-center gap-1 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
					style="background-color: var(--color-background); color: var(--color-foreground); border: 1px solid var(--color-border)"
				>
					<Plus class="h-3.5 w-3.5" />
					Add
				</button>
			{/if}
		</div>

		{#if showRelocationForm}
			<form onsubmit={addRelocation} class="mt-4 space-y-3 rounded-lg p-4" style="background-color: var(--color-background);">
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label for="relo_country" class="mb-1 block text-xs font-medium" style="color: var(--color-foreground)">Country</label>
						<input
							id="relo_country"
							type="text"
							required
							bind:value={relocationCountry}
							placeholder="e.g. Canada"
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
							style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
						/>
					</div>
					<div>
						<label for="relo_city" class="mb-1 block text-xs font-medium" style="color: var(--color-foreground)">City (optional)</label>
						<input
							id="relo_city"
							type="text"
							bind:value={relocationCity}
							placeholder="e.g. Toronto"
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
							style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
						/>
					</div>
				</div>
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label for="relo_status" class="mb-1 block text-xs font-medium" style="color: var(--color-foreground)">Status</label>
						<select
							id="relo_status"
							bind:value={relocationStatus}
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
							style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
						>
							{#each relocationStatuses as s}
								<option value={s.value}>{s.label}</option>
							{/each}
						</select>
					</div>
					<div>
						<label for="relo_year" class="mb-1 block text-xs font-medium" style="color: var(--color-foreground)">Year (optional)</label>
						<input
							id="relo_year"
							type="number"
							min="2000"
							max="2099"
							bind:value={relocationYear}
							placeholder="2025"
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
							style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
						/>
					</div>
				</div>
				<div class="flex justify-end gap-2">
					<button
						type="button"
						onclick={() => (showRelocationForm = false)}
						class="rounded-lg px-3 py-1.5 text-sm font-medium"
						style="color: var(--color-muted)"
					>
						Cancel
					</button>
					<button
						type="submit"
						disabled={formLoading || !relocationCountry.trim()}
						class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
						style="background-color: var(--color-primary)"
					>
						{#if formLoading}
							<Loader2 class="h-3.5 w-3.5 animate-spin" />
						{:else}
							<Plus class="h-3.5 w-3.5" />
						{/if}
						Save
					</button>
				</div>
			</form>
		{/if}

		{#if relocations.length === 0}
			<p class="mt-4 text-sm" style="color: var(--color-muted)">No relocations added yet.</p>
		{:else}
			<div class="mt-4 space-y-3">
				{#each relocations as relo (relo.id)}
					<div class="flex items-center gap-3 rounded-lg px-4 py-3" style="background-color: var(--color-background);">
						<div class="flex h-8 w-8 items-center justify-center rounded-full" style="background-color: color-mix(in srgb, var(--color-primary) 15%, transparent);">
							<MapPin class="h-4 w-4" style="color: var(--color-primary)" />
						</div>
						<div class="flex-1 min-w-0">
							<span class="text-sm font-medium" style="color: var(--color-foreground)">
								{relo.city ? `${relo.city}, ` : ''}{relo.country}
							</span>
							<div class="flex items-center gap-2 text-xs" style="color: var(--color-muted)">
								<span
									class="rounded-full px-2 py-0.5"
									style="background-color: var(--color-surface)"
								>
									{relo.status}
								</span>
								{#if relo.year}
									<span>{relo.year}</span>
								{/if}
							</div>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</section>

	<!-- Documents Section -->
	<section class="mt-6 rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
		<div class="flex items-center justify-between">
			<h2 class="flex items-center gap-2 text-lg font-semibold" style="color: var(--color-foreground)">
				<FileText class="h-5 w-5" style="color: var(--color-primary)" />
				Documents
			</h2>
			{#if isOwner}
				<button
					onclick={() => (showDocumentForm = !showDocumentForm)}
					class="flex items-center gap-1 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
					style="background-color: var(--color-background); color: var(--color-foreground); border: 1px solid var(--color-border)"
				>
					<Plus class="h-3.5 w-3.5" />
					Add
				</button>
			{/if}
		</div>

		{#if showDocumentForm}
			<form onsubmit={addDocument} class="mt-4 space-y-3 rounded-lg p-4" style="background-color: var(--color-background);">
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label for="doc_type" class="mb-1 block text-xs font-medium" style="color: var(--color-foreground)">Document Type</label>
						<input
							id="doc_type"
							type="text"
							required
							bind:value={docType}
							placeholder="e.g. Passport, Visa"
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
							style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
						/>
					</div>
					<div>
						<label for="doc_title" class="mb-1 block text-xs font-medium" style="color: var(--color-foreground)">Title</label>
						<input
							id="doc_title"
							type="text"
							required
							bind:value={docTitle}
							placeholder="Document title"
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
							style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
						/>
					</div>
				</div>
				<div>
					<label for="doc_desc" class="mb-1 block text-xs font-medium" style="color: var(--color-foreground)">Description (optional)</label>
					<textarea
						id="doc_desc"
						rows="2"
						bind:value={docDescription}
						placeholder="Brief description"
						class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
						style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
					></textarea>
				</div>
				<div class="flex justify-end gap-2">
					<button
						type="button"
						onclick={() => (showDocumentForm = false)}
						class="rounded-lg px-3 py-1.5 text-sm font-medium"
						style="color: var(--color-muted)"
					>
						Cancel
					</button>
					<button
						type="submit"
						disabled={formLoading || !docType.trim() || !docTitle.trim()}
						class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
						style="background-color: var(--color-primary)"
					>
						{#if formLoading}
							<Loader2 class="h-3.5 w-3.5 animate-spin" />
						{:else}
							<Plus class="h-3.5 w-3.5" />
						{/if}
						Save
					</button>
				</div>
			</form>
		{/if}

		{#if documents.length === 0}
			<p class="mt-4 text-sm" style="color: var(--color-muted)">No documents added yet.</p>
		{:else}
			<div class="mt-4 space-y-2">
				{#each documents as doc (doc.id)}
					<div class="flex items-center gap-3 rounded-lg px-4 py-3" style="background-color: var(--color-background);">
						<FileText class="h-4 w-4 shrink-0" style="color: var(--color-muted)" />
						<div class="flex-1 min-w-0">
							<div class="flex items-center gap-2">
								<span class="text-sm font-medium" style="color: var(--color-foreground)">{doc.title}</span>
								<span class="text-xs" style="color: var(--color-muted)">{doc.document_type}</span>
								{#if doc.verified}
									<span class="rounded-full px-1.5 py-0.5 text-[10px] font-medium" style="background-color: color-mix(in srgb, var(--color-success, #16a34a) 15%, transparent); color: var(--color-success, #16a34a)">
										Verified
									</span>
								{/if}
							</div>
							{#if doc.description}
								<p class="mt-0.5 text-xs" style="color: var(--color-muted)">{doc.description}</p>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</section>

	<!-- Verifiers Section -->
	{#if isOwner}
		<section class="mt-6 rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
			<h2 class="flex items-center gap-2 text-lg font-semibold" style="color: var(--color-foreground)">
				<Shield class="h-5 w-5" style="color: var(--color-primary)" />
				Verifiers
			</h2>

			{#if verifiers.length === 0}
				<p class="mt-4 text-sm" style="color: var(--color-muted)">No verifiers assigned yet.</p>
			{:else}
				<div class="mt-4 space-y-2">
					{#each verifiers as v (v.id)}
						<div class="flex items-center gap-3 rounded-lg px-4 py-3" style="background-color: var(--color-background);">
							<div class="flex h-8 w-8 items-center justify-center rounded-full text-xs font-bold text-white" style="background-color: var(--color-primary);">
								{v.user_name.charAt(0).toUpperCase()}
							</div>
							<div class="flex-1 min-w-0">
								<span class="text-sm font-medium" style="color: var(--color-foreground)">{v.user_name}</span>
								<div class="flex items-center gap-2 text-xs" style="color: var(--color-muted)">
									<span>{v.role}</span>
									<span>&middot;</span>
									<span>{v.status}</span>
								</div>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</section>
	{/if}

	<!-- Members Section -->
	{#if isOwner}
		<section class="mt-6 rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
			<div class="flex items-center justify-between">
				<h2 class="flex items-center gap-2 text-lg font-semibold" style="color: var(--color-foreground)">
					<Users class="h-5 w-5" style="color: var(--color-primary)" />
					Members
				</h2>
				<button
					onclick={() => { showMemberForm = !showMemberForm; if (showMemberForm && members.length === 0) loadMembers(); }}
					class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
					style="background-color: var(--color-background); color: var(--color-foreground); border: 1px solid var(--color-border)"
				>
					<UserPlus class="h-3.5 w-3.5" />
					Add Member
				</button>
			</div>

			{#if showMemberForm}
				<form onsubmit={addMember} class="mt-4 space-y-3 rounded-lg p-4" style="background-color: var(--color-background);">
					<div>
						<label for="member_user" class="mb-1 block text-xs font-medium" style="color: var(--color-foreground)">User ID or Username *</label>
						<input
							id="member_user"
							type="text"
							bind:value={memberUsername}
							placeholder="Enter user ID or username"
							required
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
							style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
						/>
					</div>
					<div>
						<label for="member_role" class="mb-1 block text-xs font-medium" style="color: var(--color-foreground)">Role</label>
						<select
							id="member_role"
							bind:value={memberRole}
							class="w-full rounded-lg border px-3 py-2 text-sm"
							style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
						>
							<option value="member">Member</option>
							<option value="admin">Admin</option>
							<option value="editor">Editor</option>
						</select>
					</div>
					<div class="flex justify-end gap-2">
						<button type="button" onclick={() => showMemberForm = false} class="rounded-lg px-3 py-1.5 text-sm font-medium" style="color: var(--color-muted)">Cancel</button>
						<button type="submit" disabled={formLoading || !memberUsername.trim()} class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50" style="background-color: var(--color-primary)">
							{#if formLoading}<Loader2 class="h-3.5 w-3.5 animate-spin" />{:else}<Plus class="h-3.5 w-3.5" />{/if}
							Add
						</button>
					</div>
				</form>
			{/if}

			{#if members.length === 0}
				<p class="mt-4 text-sm" style="color: var(--color-muted)">No members added yet.</p>
			{:else}
				<div class="mt-4 space-y-2">
					{#each members as m (m.user_id)}
						<div class="flex items-center gap-3 rounded-lg px-4 py-3" style="background-color: var(--color-background);">
							<div class="flex h-8 w-8 items-center justify-center rounded-full text-xs font-bold text-white" style="background-color: var(--color-primary);">
								{m.display_name?.charAt(0)?.toUpperCase() ?? m.username.charAt(0).toUpperCase()}
							</div>
							<div class="flex-1 min-w-0">
								<span class="text-sm font-medium" style="color: var(--color-foreground)">{m.display_name || m.username}</span>
								<div class="text-xs" style="color: var(--color-muted)">{m.role}</div>
							</div>
							<button onclick={() => removeMember(m.user_id)} class="text-xs" style="color: #ef4444">Remove</button>
						</div>
					{/each}
				</div>
			{/if}
		</section>
	{/if}

	<Reviews entityType="professional_profile" entityId={profile.id} />
	</main>
{/if}
