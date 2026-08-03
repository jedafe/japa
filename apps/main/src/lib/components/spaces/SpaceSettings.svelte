<script lang="ts">
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { Camera, Loader2, Image as ImageIcon, Shield, ShieldCheck, UserMinus, UserPlus } from 'lucide-svelte';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';
	import type { Space, SpaceModerator, MediaAsset } from '$lib/types';

	interface Props {
		space: Space;
		keywords: string[];
		logoUrl?: string;
		isOwner?: boolean;
		isModerator?: boolean;
		moderators?: SpaceModerator[];
	}

	let {
		space,
		keywords: initialKeywords,
		logoUrl: initialLogoUrl = '',
		isOwner = false,
		isModerator = false,
		moderators = $bindable([])
	}: Props = $props();

	let name = $state(space.name);
	let description = $state(space.description ?? '');
	let wikiContent = $state(space.wiki_content ?? '');
	let logoUrl = $state(initialLogoUrl);
	let logoMediaId = $state(space.logo_media_id ?? '');
	let logoPickerOpen = $state(false);
	let logoSaving = $state(false);
	let visibility = $state(space.visibility);
	let postingPerm = $state(space.posting_perm);
	let replyingPerm = $state(space.replying_perm);
	let commentingPerm = $state(space.commenting_perm);
	let joiningPerm = $state(space.joining_perm);
	let keywords = $state<string>(initialKeywords.join(', '));
	let saving = $state(false);
	let saved = $state(false);
	let error = $state('');

	const visibilityOptions = [
		{ value: 'public', label: 'Public — visible to everyone' },
		{ value: 'members', label: 'Members — visible to space members only' },
		{ value: 'private', label: 'Private — visible to invited members only' }
	];

	const permOptions = [
		{ value: 'open', label: 'Open — anyone can participate' },
		{ value: 'members', label: 'Members — only space members' },
		{ value: 'verified', label: 'Verified — only verified members' },
		{ value: 'moderators', label: 'Moderators — only moderators' },
		{ value: 'admins', label: 'Admins — only space admins' }
	];

	const joiningOptions = [
		{ value: 'open', label: 'Open — anyone can join immediately' },
		{ value: 'request', label: 'Request — requires moderator approval' },
		{ value: 'invite', label: 'Invite — only invited users can join' }
	];

	async function handleLogoSelect(asset: MediaAsset) {
		logoPickerOpen = false;
		logoSaving = true;
		error = '';
		try {
			const res = await api.patch<Space>(`/spaces/${space.id}`, {
				logo_media_id: asset.id
			});
			if (res.data) {
				logoMediaId = asset.id;
				logoUrl = asset.url;
			} else {
				error = res.error?.message ?? 'Failed to update logo';
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to update logo';
		} finally {
			logoSaving = false;
		}
	}

	async function save() {
		saving = true;
		saved = false;
		error = '';
		try {
			const keywordArray = keywords
				.split(',')
				.map((k) => k.trim().toLowerCase())
				.filter((k) => k.length > 0);

			const res = await api.patch<Space>(`/spaces/${space.id}`, {
				name,
				description: description || null,
				wiki_content: wikiContent,
				visibility,
				posting_perm: postingPerm,
				replying_perm: replyingPerm,
				commenting_perm: commentingPerm,
				joining_perm: joiningPerm,
				keywords: keywordArray
			});
			if (res.data) {
				saved = true;
				setTimeout(() => (saved = false), 3000);
			} else {
				error = res.error?.message ?? 'Failed to save settings';
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save settings';
		} finally {
			saving = false;
		}
	}

	// --- Moderators management (Ref: §05 lines 198-224, §03 spaces.moderator.assign) ---
	// spaces.moderator.assign is dynamically resolved: the space owner (role=
	// 'owner'), any assigned moderator (the space_moderators row IS the grant),
	// and super-admins / holders of spaces.moderate can all assign moderators.
	// We surface the panel for the owner and assigned moderators; site admins
	// manage moderators through the Admin Panel. The space_owner handler path
	// additionally requires the target to already be a space member.
	let modUsername = $state('');
	let modAdding = $state(false);
	let modError = $state('');
	let removingId = $state<string | null>(null);

	const canManageModerators = $derived(isOwner || isModerator);

	async function addModerator() {
		const handle = modUsername.trim().replace(/^@/, '');
		if (!handle || modAdding) return;
		modAdding = true;
		modError = '';
		const res = await api.post<{ space_id: string; user_id: string; assigned_by: string }>(
			`/spaces/${space.slug}/moderators`,
			{ username: handle }
		);
		if (res.error) {
			modError = res.error.message;
		} else if (res.data) {
			// Refresh the moderator list from the server so the new entry has
			// the full join row (username, display_name, assigned_by).
			await refreshModerators();
			modUsername = '';
		}
		modAdding = false;
	}

	async function removeModerator(userId: string) {
		if (removingId) return;
		removingId = userId;
		modError = '';
		const res = await api.delete<{ status: string }>(
			`/spaces/${space.slug}/moderators/${userId}`
		);
		if (res.error) {
			modError = res.error.message;
		} else {
			moderators = moderators.filter((m) => m.user_id !== userId);
		}
		removingId = null;
	}

	async function refreshModerators() {
		const res = await api.get<SpaceModerator[]>(`/spaces/${space.slug}/moderators`);
		if (res.data) moderators = res.data;
	}
</script>

<div class="mx-auto max-w-2xl px-4 py-6">
	{#if error}
		<div class="mb-4 rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-400">
			{error}
		</div>
	{/if}

	{#if saved}
		<div class="mb-4 rounded-lg bg-green-50 px-4 py-3 text-sm text-green-700 dark:bg-green-950/30 dark:text-green-400">
			Settings saved successfully.
		</div>
	{/if}

	<form onsubmit={(e) => { e.preventDefault(); save(); }} class="space-y-6">
		<!-- General information -->
		<fieldset class="rounded-lg border border-[var(--color-border)] p-5">
			<legend class="px-2 text-sm font-semibold text-[var(--color-text)]">
				{t('spaces.settings_general', 'General information')}
			</legend>

			<!-- Space logo -->
			<div class="mb-5 flex items-center gap-4">
				<div class="relative">
					{#if logoUrl}
						<img src={logoUrl} alt={name} class="h-16 w-16 rounded-full object-cover" />
					{:else}
						<div class="grid h-16 w-16 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)]">
							<ImageIcon size={24} />
						</div>
					{/if}
					{#if logoSaving}
						<div class="absolute inset-0 grid h-16 w-16 place-items-center rounded-full bg-[var(--color-surface)]/60">
							<Loader2 size={16} class="animate-spin text-[var(--color-primary)]" />
						</div>
					{/if}
					<button
						type="button"
						disabled={logoSaving}
						class="absolute -bottom-1 -right-1 grid h-6 w-6 cursor-pointer place-items-center rounded-full bg-[var(--color-primary)] text-white hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
						onclick={() => (logoPickerOpen = true)}
						aria-label="Upload logo"
					>
						<Camera size={12} />
					</button>
				</div>
				<div>
					<p class="text-sm font-medium text-[var(--color-text)]">{t('spaces.settings_logo', 'Logo')}</p>
					<p class="text-xs text-[var(--color-muted)]">{t('spaces.settings_logo_hint', 'Click the camera to upload or pick from library')}</p>
				</div>
			</div>

			<div class="mb-4">
				<label for="space-name" class="mb-1 block text-xs text-[var(--color-muted)]">
					{t('spaces.settings_name', 'Name')}
				</label>
				<input
					id="space-name"
					type="text"
					bind:value={name}
					class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				/>
			</div>
			<div class="mb-4">
				<label for="space-description" class="mb-1 block text-xs text-[var(--color-muted)]">
					{t('spaces.settings_description', 'Description')}
				</label>
				<textarea
					id="space-description"
					bind:value={description}
					rows="3"
					placeholder="A short description of this space"
					class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				></textarea>
			</div>
			<div>
				<label for="space-wiki" class="mb-1 block text-xs text-[var(--color-muted)]">
					{t('spaces.settings_wiki', 'About / Wiki')}
				</label>
				<textarea
					id="space-wiki"
					bind:value={wikiContent}
					rows="6"
					placeholder="Detailed information about this space (HTML)"
					class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 font-mono text-xs text-[var(--color-text)]"
				></textarea>
			</div>
		</fieldset>

		<!-- Who can view this space -->
		<fieldset class="rounded-lg border border-[var(--color-border)] p-5">
			<legend class="px-2 text-sm font-semibold text-[var(--color-text)]">
				{t('spaces.settings_visibility', 'Who can view this space')}
			</legend>
			<p class="mb-3 text-xs text-[var(--color-muted)]">
				Controls who can see this space and its content.
			</p>
			{#each visibilityOptions as opt}
				<label class="mb-2 flex cursor-pointer items-start gap-2 text-sm text-[var(--color-text)]">
					<input type="radio" name="visibility" value={opt.value} bind:group={visibility} class="mt-0.5" />
					<span>{opt.label}</span>
				</label>
			{/each}
		</fieldset>

		<!-- Who can post -->
		<fieldset class="rounded-lg border border-[var(--color-border)] p-5">
			<legend class="px-2 text-sm font-semibold text-[var(--color-text)]">
				{t('spaces.settings_posting', 'Who can post')}
			</legend>
			<p class="mb-3 text-xs text-[var(--color-muted)]">
				Minimum role required to create content in this space.
			</p>
			<select bind:value={postingPerm} class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]">
				{#each permOptions as opt}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</fieldset>

		<!-- Who can reply / comment -->
		<fieldset class="rounded-lg border border-[var(--color-border)] p-5">
			<legend class="px-2 text-sm font-semibold text-[var(--color-text)]">
				{t('spaces.settings_replying', 'Who can reply and comment')}
			</legend>
			<div class="mb-3">
				<label for="replying-perm" class="mb-1 block text-xs text-[var(--color-muted)]">Replying</label>
				<select id="replying-perm" bind:value={replyingPerm} class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]">
					{#each permOptions as opt}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>
			<div>
				<label for="commenting-perm" class="mb-1 block text-xs text-[var(--color-muted)]">Commenting</label>
				<select id="commenting-perm" bind:value={commentingPerm} class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]">
					{#each permOptions as opt}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>
		</fieldset>

		<!-- Who can join -->
		<fieldset class="rounded-lg border border-[var(--color-border)] p-5">
			<legend class="px-2 text-sm font-semibold text-[var(--color-text)]">
				{t('spaces.settings_joining', 'Who can join')}
			</legend>
			<p class="mb-3 text-xs text-[var(--color-muted)]">
				Controls how users become members of this space.
			</p>
			<select bind:value={joiningPerm} class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]">
				{#each joiningOptions as opt}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
		</fieldset>

		<!-- Keywords for auto-assign -->
		<fieldset class="rounded-lg border border-[var(--color-border)] p-5">
			<legend class="px-2 text-sm font-semibold text-[var(--color-text)]">
				{t('spaces.settings_keywords', 'Auto-assign keywords')}
			</legend>
			<p class="mb-3 text-xs text-[var(--color-muted)]">
				When someone creates content without selecting a space, the system checks the title for these keywords.
				If a keyword matches, the content is automatically assigned to this space. Separate keywords with commas.
				Only applies to primary spaces (country, state, city) and public spaces.
			</p>
			<input
				type="text"
				bind:value={keywords}
				placeholder="e.g. nigeria, lagos, naija"
				class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
			/>
			<p class="mt-2 text-xs text-[var(--color-muted)]">
				Current keywords: {initialKeywords.length}
			</p>
		</fieldset>

		<!-- Moderators — owner-only (Ref: §05 lines 198-224, §03 spaces.moderator.assign) -->
		{#if canManageModerators}
			<fieldset class="rounded-lg border border-[var(--color-border)] p-5">
				<legend class="px-2 text-sm font-semibold text-[var(--color-text)]">
					{t('space.moderators.title', 'Moderators')}
				</legend>
				<p class="mb-3 text-xs text-[var(--color-muted)]">
					{t('space.moderators.hint', 'Assign moderators to help manage this space. The user must already be a member of this space.')}
				</p>

				{#if modError}
					<p class="mb-3 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-950/30 dark:text-red-400">
						{modError}
					</p>
				{/if}

				{#if moderators.length === 0}
					<p class="mb-3 text-xs text-[var(--color-muted)]">
						{t('space.moderators.empty', 'No moderators assigned yet.')}
					</p>
				{:else}
					<ul class="mb-4 space-y-2">
						{#each moderators as mod (mod.user_id)}
							<li class="flex items-center gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-2.5">
								<span class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-primary)]">
									<ShieldCheck size={15} />
								</span>
								<a href="/u/{mod.user_username}" class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-[var(--color-text)]">
										{mod.user_display_name ?? mod.user_username}
									</span>
									<span class="block text-xs text-[var(--color-muted)]">@{mod.user_username}</span>
								</a>
								<span class="shrink-0 rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[10px] font-medium text-[var(--color-text-subtle)]">
									{mod.assigned_by === 'space_owner'
										? t('space.moderators.assigned_by_owner', 'By owner')
										: t('space.moderators.assigned_by_admin', 'By admin')}
								</span>
								<button
									type="button"
									disabled={removingId === mod.user_id}
									aria-label={t('space.moderators.remove', 'Remove moderator')}
									title={t('space.moderators.remove', 'Remove moderator')}
									onclick={() => removeModerator(mod.user_id)}
									class="grid h-7 w-7 shrink-0 place-items-center rounded-full text-[var(--color-muted)] transition-colors hover:bg-red-50 hover:text-red-600 disabled:opacity-50 dark:hover:bg-red-950/30"
								>
									{#if removingId === mod.user_id}
										<Loader2 size={14} class="animate-spin" />
									{:else}
										<UserMinus size={14} />
									{/if}
								</button>
							</li>
						{/each}
					</ul>
				{/if}

				<div class="flex items-center gap-2">
					<div class="relative flex-1">
						<UserPlus size={15} class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
						<input
							type="text"
							bind:value={modUsername}
							placeholder={t('space.moderators.username_placeholder', 'username to add…')}
							disabled={modAdding}
							onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addModerator(); } }}
							class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] py-2 pl-8 pr-3 text-sm text-[var(--color-text)] disabled:opacity-50"
						/>
					</div>
					<button
						type="button"
						disabled={modAdding || !modUsername.trim()}
						onclick={addModerator}
						class="inline-flex shrink-0 items-center gap-1.5 rounded-lg bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{#if modAdding}
							<Loader2 size={14} class="animate-spin" />
						{:else}
							<Shield size={14} />
						{/if}
						{t('space.moderators.add', 'Add')}
					</button>
				</div>
			</fieldset>
		{/if}

		<div class="flex items-center gap-3">
			<button
				type="submit"
				disabled={saving}
				class="rounded-lg bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
			>
				{saving ? 'Saving…' : t('spaces.settings_save', 'Save settings')}
			</button>
		</div>
	</form>
</div>

{#if logoPickerOpen}
	<MediaLibraryModal onSelect={handleLogoSelect} onClose={() => (logoPickerOpen = false)} />
{/if}
