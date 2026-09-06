<script lang="ts">
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		Camera, Loader2, Image as ImageIcon, Shield, ShieldCheck, UserMinus, UserPlus,
		Plus, Trash2, Pencil, ArrowUp, ArrowDown, AlertTriangle, Link2, Unlink
	} from 'lucide-svelte';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';
	import SpaceCombobox from '$lib/components/spaces/SpaceCombobox.svelte';
	import PostEditor from '$lib/components/create/editors/PostEditor.svelte';
	import { decodeBase64Text } from '$lib/tiptap/serialize';
	import type {
		Space, SpaceModerator, SpaceRule, SpacePermissionPreset, MediaAsset
	} from '$lib/types';

	function escapeHtml(s: string): string {
		return s
			.replace(/&/g, '&amp;')
			.replace(/</g, '&lt;')
			.replace(/>/g, '&gt;')
			.replace(/"/g, '&quot;');
	}

	interface Props {
		space: Space;
		keywords: string[];
		logoUrl?: string;
		headerUrl?: string;
		isOwner?: boolean;
		isModerator?: boolean;
		moderators?: SpaceModerator[];
		rules?: SpaceRule[];
		parentSpaces?: Space[];
		presets?: SpacePermissionPreset[];
	}

	let {
		space,
		keywords: initialKeywords,
		logoUrl: initialLogoUrl = '',
		headerUrl: initialHeaderUrl = '',
		isOwner = false,
		isModerator = false,
		moderators = $bindable([]),
		rules = $bindable([]),
		parentSpaces = $bindable([]),
		presets = []
	}: Props = $props();

	let name = $state(untrack(() => space.name));
	let description = $state(untrack(() => space.description ?? ''));
	// Wiki content: the API stores it as base64-encoded bytes. The
	// auto-populate task stores JSON {title, extract, ...}; manual edits
	// via this form store raw HTML. Handle both.
	let wikiHtml = $state(
		untrack(() => {
			if (!space.wiki_content) return '';
			try {
				const decoded = typeof space.wiki_content === 'string'
					? decodeBase64Text(space.wiki_content)
					: String(space.wiki_content);
				// Try parsing as JSON (auto-populate format).
				try {
					const json = JSON.parse(decoded);
					const parts: string[] = [];
					// Escape the Wikipedia-sourced strings — they are plain
					// text, and an extract containing HTML metacharacters would
					// otherwise be injected verbatim into the editor.
					if (json?.description) parts.push(`<p><em>${escapeHtml(String(json.description))}</em></p>`);
					if (json?.extract) parts.push(`<p>${escapeHtml(String(json.extract))}</p>`);
					return parts.join('');
				} catch {
					// Not JSON — treat as raw HTML (manual edit format).
					return decoded;
				}
			} catch {
				return '';
			}
		})
	);
	let logoUrl = $state(untrack(() => initialLogoUrl));
	let logoMediaId = $state(untrack(() => space.logo_media_id ?? ''));
	let logoPickerOpen = $state(false);
	let logoSaving = $state(false);
	let headerUrl = $state(untrack(() => initialHeaderUrl));
	let headerMediaId = $state(untrack(() => space.header_image_id ?? ''));
	let headerPickerOpen = $state(false);
	let headerSaving = $state(false);
	let visibility = $state(untrack(() => space.visibility));
	let postingPerm = $state(untrack(() => space.posting_perm));
	let replyingPerm = $state(untrack(() => space.replying_perm));
	let commentingPerm = $state(untrack(() => space.commenting_perm));
	let joiningPerm = $state(untrack(() => space.joining_perm));
	let keywords = $state<string>(untrack(() => initialKeywords.join(', ')));
	let saving = $state(false);
	let saved = $state(false);
	let error = $state('');

	// Rule 16: permission-axis options come from system_settings so admins
	// can restrict the available values without a code change.
	const permissionAxes: Record<string, string[]> = $derived(
		(page.data.siteSettings?.['spaces.permission_axes'] as Record<string, string[]> | undefined) ?? {
			visibility: ['public', 'members', 'private'],
			posting: ['open', 'members', 'verified', 'moderators', 'admins'],
			joining: ['open', 'request', 'invite']
		}
	);

	const allVisibilityOptions = [
		{ value: 'public', label: t('spaces.visibility_public', 'Public — visible to everyone') },
		{ value: 'members', label: t('spaces.visibility_members', 'Members — visible to space members only') },
		{ value: 'private', label: t('spaces.visibility_private', 'Private — visible to invited members only') }
	];
	const visibilityOptions = $derived(
		allVisibilityOptions.filter((o) => permissionAxes.visibility?.includes(o.value))
	);

	const allPermOptions = [
		{ value: 'open', label: t('spaces.perm_open', 'Open — anyone can participate') },
		{ value: 'members', label: t('spaces.perm_members', 'Members — only space members') },
		{ value: 'verified', label: t('spaces.perm_verified', 'Verified — only verified members') },
		{ value: 'moderators', label: t('spaces.perm_moderators', 'Moderators — only moderators') },
		{ value: 'admins', label: t('spaces.perm_admins', 'Admins — only space admins') }
	];
	const permOptions = $derived(allPermOptions.filter((o) => permissionAxes.posting?.includes(o.value)));

	const allJoiningOptions = [
		{ value: 'open', label: t('spaces.joining_open', 'Open — anyone can join immediately') },
		{ value: 'request', label: t('spaces.joining_request', 'Request — requires moderator approval') },
		{ value: 'invite', label: t('spaces.joining_invite', 'Invite — only invited users can join') }
	];
	const joiningOptions = $derived(allJoiningOptions.filter((o) => permissionAxes.joining?.includes(o.value)));

	// --- Logo / Header image handlers ---
	async function handleLogoSelect(asset: MediaAsset) {
		logoPickerOpen = false;
		logoSaving = true;
		error = '';
		const res = await api.patch<Space>(`/spaces/${space.id}`, { logo_media_id: asset.id });
		if (res.data) { logoMediaId = asset.id; logoUrl = asset.url; }
		else { error = res.error?.message ?? 'Failed to update logo'; }
		logoSaving = false;
	}

	async function handleHeaderSelect(asset: MediaAsset) {
		headerPickerOpen = false;
		headerSaving = true;
		error = '';
		const res = await api.patch<Space>(`/spaces/${space.id}`, { header_image_id: asset.id });
		if (res.data) { headerMediaId = asset.id; headerUrl = asset.url; }
		else { error = res.error?.message ?? 'Failed to update header image'; }
		headerSaving = false;
	}

	async function removeHeader() {
		headerSaving = true;
		error = '';
		const res = await api.patch<Space>(`/spaces/${space.id}`, { header_image_id: null });
		if (res.data) { headerMediaId = ''; headerUrl = ''; }
		else { error = res.error?.message ?? 'Failed to remove header'; }
		headerSaving = false;
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
			wiki_content: wikiHtml,
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
		try {
			const res = await api.post<{ space_id: string; user_id: string; assigned_by: string }>(
				`/spaces/${space.slug}/moderators`, { username: handle }
			);
			if (res.error) { modError = res.error.message; }
			else if (res.data) { await refreshModerators(); modUsername = ''; }
		} catch (err) {
			modError = err instanceof Error ? err.message : 'Failed to add moderator. The user must be a member of this space.';
		}
		modAdding = false;
	}

	async function removeModerator(userId: string) {
		if (removingId) return;
		removingId = userId;
		modError = '';
		const res = await api.delete<{ status: string }>(`/spaces/${space.slug}/moderators/${userId}`);
		if (res.error) { modError = res.error.message; }
		else { moderators = moderators.filter((m) => m.user_id !== userId); }
		removingId = null;
	}

	async function refreshModerators() {
		const res = await api.get<SpaceModerator[]>(`/spaces/${space.slug}/moderators`);
		if (res.data) moderators = res.data;
	}

	// --- Space Rules CRUD (Ref: §05 lines 82-89; POST/PATCH/DELETE /spaces/{slug}/rules) ---
	let ruleTitle = $state('');
	let ruleBody = $state('');
	let ruleAdding = $state(false);
	let ruleError = $state('');
	let editingRuleId = $state<string | null>(null);
	let editRuleTitle = $state('');
	let editRuleBody = $state('');
	let ruleSavingId = $state<string | null>(null);
	let ruleDeletingId = $state<string | null>(null);

	const canManageRules = $derived(isOwner || isModerator);

	async function addRule() {
		if (!ruleTitle.trim() || !ruleBody.trim() || ruleAdding) return;
		ruleAdding = true;
		ruleError = '';
		const maxPos = rules.length > 0 ? Math.max(...rules.map((r) => r.position)) : 0;
		const res = await api.post<SpaceRule>(`/spaces/${space.slug}/rules`, {
			title: ruleTitle.trim(),
			body: ruleBody.trim(),
			position: maxPos + 1
		});
		if (res.error) { ruleError = res.error.message; }
		else if (res.data) {
			rules = [...rules, res.data];
			ruleTitle = '';
			ruleBody = '';
		}
		ruleAdding = false;
	}

	async function saveRule(ruleId: string) {
		if (!editRuleTitle.trim() || !editRuleBody.trim()) return;
		ruleSavingId = ruleId;
		ruleError = '';
		const rule = rules.find((r) => r.id === ruleId);
		const res = await api.patch<SpaceRule>(`/spaces/${space.slug}/rules/${ruleId}`, {
			title: editRuleTitle.trim(),
			body: editRuleBody.trim(),
			position: rule?.position ?? 1
		});
		if (res.error) { ruleError = res.error.message; }
		else if (res.data) {
			rules = rules.map((r) => r.id === ruleId ? res.data! : r);
			editingRuleId = null;
		}
		ruleSavingId = null;
	}

	async function deleteRule(ruleId: string) {
		ruleDeletingId = ruleId;
		ruleError = '';
		const res = await api.delete<{ status: string }>(`/spaces/${space.slug}/rules/${ruleId}`);
		if (res.error) { ruleError = res.error.message; }
		else { rules = rules.filter((r) => r.id !== ruleId); }
		ruleDeletingId = null;
	}

	async function moveRule(ruleId: string, direction: 'up' | 'down') {
		const sorted = [...rules].sort((a, b) => a.position - b.position);
		const idx = sorted.findIndex((r) => r.id === ruleId);
		if (idx < 0) return;
		const swapIdx = direction === 'up' ? idx - 1 : idx + 1;
		if (swapIdx < 0 || swapIdx >= sorted.length) return;
		const rule = sorted[idx];
		const swapRule = sorted[swapIdx];
		// Save original for rollback
		const originalRules = [...rules];
		// Optimistically swap positions locally
		rules = rules.map((r) => {
			if (r.id === rule.id) return { ...r, position: swapRule.position };
			if (r.id === swapRule.id) return { ...r, position: rule.position };
			return r;
		});
		// Persist both updates
		ruleSavingId = ruleId;
		try {
			await api.patch<SpaceRule>(`/spaces/${space.slug}/rules/${rule.id}`, {
				title: rule.title, body: rule.body, position: swapRule.position
			});
			await api.patch<SpaceRule>(`/spaces/${space.slug}/rules/${swapRule.id}`, {
				title: swapRule.title, body: swapRule.body, position: rule.position
			});
		} catch {
			// Rollback on failure
			rules = originalRules;
			ruleError = 'Failed to reorder rules. Changes have been reverted.';
		}
		ruleSavingId = null;
	}

	function startEdit(rule: SpaceRule) {
		editingRuleId = rule.id;
		editRuleTitle = rule.title;
		editRuleBody = rule.body;
	}

	// --- Parent space management (Ref: §05 lines 46-54; POST/DELETE /spaces/{slug}/parents) ---
	let parentError = $state('');
	let parentAdding = $state(false);
	let removingParentId = $state<string | null>(null);

	const canManageParents = $derived(isOwner);
	const isCustomSpace = $derived(space.level === 'custom');

	async function addParent(spaceOption: { id: string; name: string; slug: string }) {
		if (parentAdding) return;
		parentAdding = true;
		parentError = '';
		const res = await api.post<{ space_id: string; parent_space_id: string }>(
			`/spaces/${space.slug}/parents`, { parent_space_id: spaceOption.id }
		);
		if (res.error) { parentError = res.error.message; }
		else { await refreshParents(); }
		parentAdding = false;
	}

	async function removeParent(parentId: string) {
		if (removingParentId) return;
		removingParentId = parentId;
		parentError = '';
		const res = await api.delete<{ status: string }>(`/spaces/${space.slug}/parents/${parentId}`);
		if (res.error) { parentError = res.error.message; }
		else { parentSpaces = parentSpaces.filter((p) => p.id !== parentId); }
		removingParentId = null;
	}

	async function refreshParents() {
		const res = await api.get<Space[]>(`/spaces/${space.slug}/parents`);
		if (res.data) parentSpaces = res.data;
	}

	// --- Permission presets (Ref: §05 lines 122-133; GET /space-permission-presets) ---
	let presetError = $state('');
	let applyingPreset = $state<string | null>(null);

	async function applyPreset(preset: SpacePermissionPreset) {
		if (!confirm(`Apply the "${preset.name}" preset? This will overwrite your current permission settings.`)) {
			return;
		}
		applyingPreset = preset.name;
		presetError = '';
		// Apply the preset's axes to the local form state and save
		visibility = preset.visibility;
		postingPerm = preset.posting_perm;
		replyingPerm = preset.replying_perm;
		commentingPerm = preset.commenting_perm;
		joiningPerm = preset.joining_perm;
		// Save immediately
		saving = true;
		const res = await api.patch<Space>(`/spaces/${space.id}`, {
			visibility, posting_perm: postingPerm, replying_perm: replyingPerm,
			commenting_perm: commentingPerm, joining_perm: joiningPerm
		});
		if (res.error) { presetError = res.error.message; }
		else { saved = true; setTimeout(() => (saved = false), 3000); }
		saving = false;
		applyingPreset = null;
	}

	// --- Delete space (Ref: §05; DELETE /spaces/{id}; creator-only, 0 members) ---
	let deleteConfirm = $state(false);
	let deleteConfirmName = $state('');
	let deleting = $state(false);
	let deleteError = $state('');

	const canDeleteSpace = $derived(isOwner && space.member_count === 0);

	async function deleteSpace() {
		if (deleting) return;
		deleting = true;
		deleteError = '';
		const res = await api.delete<{ status: string }>(`/spaces/${space.id}`);
		if (res.error) {
			deleteError = res.error.message;
			deleting = false;
		} else {
			await goto('/spaces');
		}
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
			{t('spaces.settings_saved', 'Settings saved successfully.')}
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

			<!-- Header image -->
			<div class="mb-5">
				<p class="mb-1 text-sm font-medium text-[var(--color-text)]">{t('spaces.settings_header', 'Header image')}</p>
				<p class="mb-2 text-xs text-[var(--color-muted)]">{t('spaces.settings_header_hint', 'A banner image shown at the top of the space page')}</p>
				<div class="relative overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-subtle)]">
					{#if headerUrl}
						<img src={headerUrl} alt="Header" class="h-32 w-full object-cover" />
						{#if headerSaving}
							<div class="absolute inset-0 grid place-items-center bg-[var(--color-surface)]/60">
								<Loader2 size={20} class="animate-spin text-[var(--color-primary)]" />
							</div>
						{/if}
						<div class="absolute bottom-2 right-2 flex gap-2">
							<button
								type="button"
								disabled={headerSaving}
								onclick={() => (headerPickerOpen = true)}
								class="rounded-lg bg-[var(--color-surface)] px-2.5 py-1 text-xs font-medium text-[var(--color-text)] shadow hover:bg-[var(--color-surface-subtle)]"
							>
								{t('common.change', 'Change')}
							</button>
							<button
								type="button"
								disabled={headerSaving}
								onclick={removeHeader}
								class="rounded-lg bg-[var(--color-surface)] px-2.5 py-1 text-xs font-medium text-red-600 shadow hover:bg-red-50 dark:hover:bg-red-950/30"
							>
								{t('common.remove', 'Remove')}
							</button>
						</div>
					{:else}
						<div class="flex h-32 items-center justify-center">
							{#if headerSaving}
								<Loader2 size={20} class="animate-spin text-[var(--color-muted)]" />
							{:else}
								<button
									type="button"
									onclick={() => (headerPickerOpen = true)}
									class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] hover:text-[var(--color-primary)]"
								>
									<ImageIcon size={16} />
									{t('spaces.settings_header_upload', 'Upload header image')}
								</button>
							{/if}
						</div>
					{/if}
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
				placeholder={t('spaces.settings_description_placeholder', 'A short description of this space')}
				class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
			></textarea>
			</div>
		<div>
			<span class="mb-1 block text-xs text-[var(--color-muted)]">
				{t('spaces.settings_wiki', 'About / Wiki')}
			</span>
			<PostEditor
				html={wikiHtml}
				onhtml={(html) => { wikiHtml = html; }}
				onedit={() => {}}
			/>
		</div>
		</fieldset>

		<!-- Permission presets (Ref: §05 lines 122-133) -->
		{#if presets.length > 0}
			<fieldset class="rounded-lg border border-[var(--color-border)] p-5">
				<legend class="px-2 text-sm font-semibold text-[var(--color-text)]">
					{t('spaces.settings_presets', 'Permission presets')}
				</legend>
				<p class="mb-3 text-xs text-[var(--color-muted)]">
					{t('spaces.settings_presets_hint', 'Apply a preset to quickly set all permission axes at once. You can still fine-tune individual permissions below.')}
				</p>
				{#if presetError}
					<p class="mb-3 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-950/30 dark:text-red-400">{presetError}</p>
				{/if}
				<div class="grid gap-2 sm:grid-cols-2">
					{#each presets as preset (preset.name)}
						<button
							type="button"
							disabled={applyingPreset !== null}
							onclick={() => applyPreset(preset)}
							class="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-left transition-colors hover:border-[var(--color-primary)]/40 disabled:opacity-50"
						>
							<div class="flex items-center justify-between">
								<span class="text-sm font-medium text-[var(--color-text)]">{preset.name}</span>
								{#if applyingPreset === preset.name}
									<Loader2 size={14} class="animate-spin text-[var(--color-primary)]" />
								{/if}
							</div>
							<p class="mt-1 text-xs text-[var(--color-muted)]">
								{preset.visibility} · {preset.posting_perm} · {preset.joining_perm}
							</p>
						</button>
					{/each}
				</div>
			</fieldset>
		{/if}

		<!-- Who can view this space -->
		<fieldset class="rounded-lg border border-[var(--color-border)] p-5">
			<legend class="px-2 text-sm font-semibold text-[var(--color-text)]">
				{t('spaces.settings_visibility', 'Who can view this space')}
			</legend>
			<p class="mb-3 text-xs text-[var(--color-muted)]">
				{t('spaces.settings_visibility_hint', 'Controls who can see this space and its content.')}
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
				{t('spaces.settings_posting_hint', 'Minimum role required to create content in this space.')}
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
				{t('spaces.settings_keywords_hint', 'When someone creates content without selecting a space, the system checks the title for these keywords. If a keyword matches, the content is automatically assigned to this space. Separate keywords with commas. Only applies to primary spaces (country, state, city) and public spaces.')}
			</p>
			<input
				type="text"
				bind:value={keywords}
				placeholder={t('spaces.keywords_placeholder', 'e.g. nigeria, lagos, naija')}
				class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
			/>
			<p class="mt-2 text-xs text-[var(--color-muted)]">
				Current keywords: {initialKeywords.length}
			</p>
		</fieldset>

		<!-- Moderators — owner/moderator (Ref: §05 lines 198-224, §03 spaces.moderator.assign) -->
		{#if canManageModerators}
			<fieldset class="rounded-lg border border-[var(--color-border)] p-5">
				<legend class="px-2 text-sm font-semibold text-[var(--color-text)]">
					{t('space.moderators.title', 'Moderators')}
				</legend>
				<p class="mb-3 text-xs text-[var(--color-muted)]">
					{t('space.moderators.hint', 'Assign moderators to help manage this space. The user must already be a member of this space.')}
				</p>

				{#if modError}
					<p class="mb-3 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-950/30 dark:text-red-400">{modError}</p>
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
						{#if modAdding}<Loader2 size={14} class="animate-spin" />{:else}<Shield size={14} />{/if}
						{t('space.moderators.add', 'Add')}
					</button>
				</div>
			</fieldset>
		{/if}

		<!-- Space Rules CRUD (Ref: §05 lines 82-89; spaces.moderate) -->
		{#if canManageRules}
			<fieldset class="rounded-lg border border-[var(--color-border)] p-5">
				<legend class="px-2 text-sm font-semibold text-[var(--color-text)]">
					{t('space.rules.title', 'Rules')}
				</legend>
				<p class="mb-3 text-xs text-[var(--color-muted)]">
					{t('space.rules.hint', 'Create rules that are displayed in the space sidebar. Rules can be reordered.')}
				</p>

				{#if ruleError}
					<p class="mb-3 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-950/30 dark:text-red-400">{ruleError}</p>
				{/if}

				<!-- Existing rules -->
				{#if rules.length > 0}
					<ul class="mb-4 space-y-2">
						{#each [...rules].sort((a, b) => a.position - b.position) as rule, i (rule.id)}
							<li class="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
								{#if editingRuleId === rule.id}
									<!-- Edit mode -->
									<div class="space-y-2">
										<input
											type="text"
											bind:value={editRuleTitle}
											placeholder={t('space.rules.title_placeholder', 'Rule title')}
											class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
										/>
										<textarea
											bind:value={editRuleBody}
											rows="2"
											placeholder={t('space.rules.body_placeholder', 'Rule description')}
											class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
										></textarea>
										<div class="flex items-center gap-2">
											<button
												type="button"
												disabled={ruleSavingId === rule.id}
												onclick={() => saveRule(rule.id)}
												class="rounded-lg bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
											>
												{#if ruleSavingId === rule.id}<Loader2 size={12} class="animate-spin" />{:else}
												{t('common.save', 'Save')}{/if}
											</button>
											<button
												type="button"
												onclick={() => (editingRuleId = null)}
												class="rounded-lg border border-[var(--color-border)] px-3 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
											>
												{t('common.cancel', 'Cancel')}
											</button>
										</div>
									</div>
								{:else}
									<!-- Display mode -->
									<div class="flex items-start gap-2">
										<span class="grid h-6 w-6 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">{i + 1}</span>
										<div class="min-w-0 flex-1">
											<p class="text-sm font-medium text-[var(--color-text)]">{rule.title}</p>
											{#if rule.body}
												<p class="mt-0.5 text-xs text-[var(--color-text-subtle)]">{rule.body}</p>
											{/if}
										</div>
										<div class="flex shrink-0 items-center gap-1">
											<button
												type="button"
												disabled={ruleSavingId === rule.id}
												onclick={() => moveRule(rule.id, 'up')}
												class="grid h-6 w-6 place-items-center rounded text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-30"
												aria-label="Move up"
											>
												<ArrowUp size={13} />
											</button>
											<button
												type="button"
												disabled={ruleSavingId === rule.id}
												onclick={() => moveRule(rule.id, 'down')}
												class="grid h-6 w-6 place-items-center rounded text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-30"
												aria-label="Move down"
											>
												<ArrowDown size={13} />
											</button>
											<button
												type="button"
												onclick={() => startEdit(rule)}
												class="grid h-6 w-6 place-items-center rounded text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
												aria-label="Edit rule"
											>
												<Pencil size={13} />
											</button>
											<button
												type="button"
												disabled={ruleDeletingId === rule.id}
												onclick={() => deleteRule(rule.id)}
												class="grid h-6 w-6 place-items-center rounded text-[var(--color-muted)] hover:bg-red-50 hover:text-red-600 disabled:opacity-30 dark:hover:bg-red-950/30"
												aria-label="Delete rule"
											>
												{#if ruleDeletingId === rule.id}<Loader2 size={13} class="animate-spin" />{:else}<Trash2 size={13} />{/if}
											</button>
										</div>
									</div>
								{/if}
							</li>
						{/each}
					</ul>
				{/if}

				<!-- Add new rule -->
				<div class="space-y-2">
					<input
						type="text"
						bind:value={ruleTitle}
						placeholder={t('space.rules.title_placeholder', 'Rule title')}
						disabled={ruleAdding}
						class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] disabled:opacity-50"
					/>
					<textarea
						bind:value={ruleBody}
						rows="2"
						placeholder={t('space.rules.body_placeholder', 'Rule description')}
						disabled={ruleAdding}
						class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] disabled:opacity-50"
					></textarea>
					<button
						type="button"
						disabled={ruleAdding || !ruleTitle.trim() || !ruleBody.trim()}
						onclick={addRule}
						class="inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{#if ruleAdding}<Loader2 size={14} class="animate-spin" />{:else}<Plus size={14} />{/if}
						{t('space.rules.add', 'Add rule')}
					</button>
				</div>
			</fieldset>
		{/if}

		<!-- Parent space management (Ref: §05 lines 46-54; spaces.edit; custom spaces only) -->
		{#if canManageParents && isCustomSpace}
			<fieldset class="rounded-lg border border-[var(--color-border)] p-5">
				<legend class="px-2 text-sm font-semibold text-[var(--color-text)]">
					{t('space.parents.title', 'Parent spaces')}
				</legend>
				<p class="mb-3 text-xs text-[var(--color-muted)]">
					{t('space.parents.hint', 'Link this custom space to one or more primary spaces (country, state, city). Posts bubble up to parent space feeds.')}
				</p>

				{#if parentError}
					<p class="mb-3 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-950/30 dark:text-red-400">{parentError}</p>
				{/if}

				{#if parentSpaces.length > 0}
					<ul class="mb-4 space-y-2">
						{#each parentSpaces as parent (parent.id)}
							<li class="flex items-center gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-2.5">
								<span class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-primary)]">
									<Link2 size={15} />
								</span>
								<a href="/spaces/{parent.slug}" class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-[var(--color-text)]">{parent.name}</span>
									<span class="block text-xs capitalize text-[var(--color-muted)]">{parent.level}</span>
								</a>
								<button
									type="button"
									disabled={removingParentId === parent.id}
									onclick={() => removeParent(parent.id)}
									class="grid h-7 w-7 shrink-0 place-items-center rounded-full text-[var(--color-muted)] transition-colors hover:bg-red-50 hover:text-red-600 disabled:opacity-50 dark:hover:bg-red-950/30"
									aria-label={t('space.parents.remove', 'Remove parent')}
								>
									{#if removingParentId === parent.id}<Loader2 size={14} class="animate-spin" />{:else}<Unlink size={14} />{/if}
								</button>
							</li>
						{/each}
					</ul>
				{/if}

				<SpaceCombobox
					multiple
					parentMode
					filter="level:country,state,city"
					placeholder={t('space.parents.search_placeholder', 'Search primary spaces to link…')}
					excludeIds={[space.id, ...parentSpaces.map((p) => p.id)]}
					onSelect={addParent}
				/>
			</fieldset>
		{/if}

		<div class="flex items-center gap-3">
			<button
				type="submit"
				disabled={saving}
				class="rounded-lg bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
			>
				{saving ? t('common.saving', 'Saving…') : t('spaces.settings_save', 'Save settings')}
			</button>
		</div>
	</form>

	<!-- Danger zone — delete space (Ref: §05; DELETE /spaces/{id}; creator-only, 0 members) -->
	{#if isOwner}
		<fieldset class="mt-6 rounded-lg border border-red-300 p-5 dark:border-red-900">
			<legend class="px-2 text-sm font-semibold text-red-700 dark:text-red-400">
				{t('space.danger_zone', 'Danger zone')}
			</legend>

			{#if deleteError}
				<p class="mb-3 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-950/30 dark:text-red-400">{deleteError}</p>
			{/if}

			{#if !canDeleteSpace}
				<p class="text-xs text-[var(--color-muted)]">
					{t('space.delete_hint', 'The space must have 0 members before it can be deleted. Current members: {n}').replace('{n}', String(space.member_count))}
				</p>
			{:else if !deleteConfirm}
				<div class="flex items-center gap-3">
					<p class="flex-1 text-xs text-[var(--color-muted)]">
						{t('space.delete_warning', 'Deleting a space is permanent and cannot be undone. All content, rules, and settings will be removed.')}
					</p>
					<button
						type="button"
						onclick={() => (deleteConfirm = true)}
						class="inline-flex shrink-0 items-center gap-1.5 rounded-lg border border-red-300 px-3 py-2 text-sm font-medium text-red-700 hover:bg-red-50 dark:border-red-900 dark:text-red-400 dark:hover:bg-red-950/30"
					>
						<AlertTriangle size={14} />
						{t('space.delete_button', 'Delete space')}
					</button>
				</div>
			{:else}
				<div class="space-y-3">
					<p class="text-sm font-medium text-red-700 dark:text-red-400">
						{t('space.delete_confirm', 'Are you absolutely sure? Type the space name to confirm.')}
					</p>
					<p class="text-xs text-[var(--color-muted)]">
						{@html t('spaces.delete_confirm_hint', 'Type <strong>{name}</strong> to confirm deletion:').replace('{name}', escapeHtml(space.name))}
					</p>
					<input
						type="text"
						bind:value={deleteConfirmName}
						placeholder={space.name}
						class="w-full rounded-lg border border-red-300 bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] dark:border-red-900"
					/>
					<div class="flex items-center gap-2">
						<button
							type="button"
							disabled={deleting || deleteConfirmName !== space.name}
							onclick={deleteSpace}
							class="inline-flex items-center gap-1.5 rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
						>
							{#if deleting}<Loader2 size={14} class="animate-spin" />{:else}<Trash2 size={14} />{/if}
							{t('space.delete_confirm_button', 'Yes, delete this space permanently')}
						</button>
						<button
							type="button"
							onclick={() => { deleteConfirm = false; deleteConfirmName = ''; }}
							class="rounded-lg border border-[var(--color-border)] px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
						>
							{t('common.cancel', 'Cancel')}
						</button>
					</div>
				</div>
			{/if}
		</fieldset>
	{/if}
</div>

{#if logoPickerOpen}
	<MediaLibraryModal onSelect={handleLogoSelect} onClose={() => (logoPickerOpen = false)} />
{/if}

{#if headerPickerOpen}
	<MediaLibraryModal onSelect={handleHeaderSelect} onClose={() => (headerPickerOpen = false)} />
{/if}
