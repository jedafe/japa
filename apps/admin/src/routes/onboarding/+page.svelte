<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Save, RefreshCw } from 'lucide-svelte';
	import { onboardingAdminApi, spacesAdminApi, tagsAdminApi, userManagementApi } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { TagInput } from '$lib/components/ui/tag-input';

	const canManage = $derived(hasPermission('admin.settings'));

	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let success = $state('');

	// Settings state
	let minTagsFollow = $state(3);
	let minUsersFollow = $state(2);
	let minSpacesJoin = $state(2);

	// Available options for dropdowns
	let availableSpaces = $state<Array<{ value: string; label: string }>>([]);
	let availableUsers = $state<Array<{ value: string; label: string }>>([]);
	let availableTags = $state<Array<{ value: string; label: string }>>([]);

	// Selected IDs (arrays of UUIDs)
	let selectedSpaces = $state<string[]>([]);
	let selectedUsers = $state<string[]>([]);
	let selectedTags = $state<string[]>([]);

	async function loadOptions() {
		// Load spaces
		const spacesResult = await spacesAdminApi.list('page_size=100');
		if (spacesResult.ok && spacesResult.data) {
			const raw = spacesResult.data as any;
			const items = raw?.items ?? (Array.isArray(raw) ? raw : []);
			availableSpaces = items.map((s: any) => ({ value: s.id, label: s.name }));
		}
		// Load tags
		const tagsResult = await tagsAdminApi.list('page_size=100');
		if (tagsResult.ok && tagsResult.data) {
			const raw = tagsResult.data as any;
			const items = raw?.items ?? (Array.isArray(raw) ? raw : []);
			availableTags = items.map((t: any) => ({ value: t.id, label: t.name }));
		}
		// Load users
		const usersResult = await userManagementApi.list('page_size=100');
		if (usersResult.ok && usersResult.data) {
			const raw = usersResult.data as any;
			const items = raw?.items ?? (Array.isArray(raw) ? raw : []);
			availableUsers = items.map((u: any) => ({ value: u.id, label: u.username ?? u.name ?? u.email }));
		}
	}

	async function loadSettings() {
		loading = true;
		error = '';
		try {
			await loadOptions();
			const result = await onboardingAdminApi.listSettings();
			if (result.ok && result.data) {
				const d = result.data as Record<string, unknown>;
				minTagsFollow = (d.min_tags_follow as number) ?? 3;
				minUsersFollow = (d.min_users_follow as number) ?? 2;
				minSpacesJoin = (d.min_spaces_join as number) ?? 2;
				selectedSpaces = ((d.promoted_spaces as string[]) ?? []);
				selectedUsers = ((d.promoted_users as string[]) ?? []);
				selectedTags = ((d.promoted_tags as string[]) ?? []);
			}
		} catch {
			error = 'Failed to load settings';
		} finally {
			loading = false;
		}
	}

	async function saveMinimums() {
		saving = true;
		error = '';
		success = '';
		try {
			const result = await onboardingAdminApi.updateSettings({
				min_tags_follow: minTagsFollow,
				min_users_follow: minUsersFollow,
				min_spaces_join: minSpacesJoin,
			});
			if (result.ok) {
				success = 'Minimum requirements saved';
			} else {
				error = 'Failed to save';
			}
		} catch {
			error = 'Failed to save settings';
		} finally {
			saving = false;
		}
	}

	async function savePromoted() {
		saving = true;
		error = '';
		success = '';
		try {
			const result = await onboardingAdminApi.updateSettings({
				promoted_spaces: selectedSpaces,
				promoted_users: selectedUsers,
				promoted_tags: selectedTags,
			});
			if (result.ok) {
				success = 'Promoted entities saved';
			} else {
				error = 'Failed to save';
			}
		} catch {
			error = 'Failed to save settings';
		} finally {
			saving = false;
		}
	}

	$effect(() => {
		loadSettings();
	});
</script>

<svelte:head>
	<title>{t('onboarding.title', 'Onboarding Settings')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('onboarding.title', 'Onboarding Settings')}
	description={t('onboarding.description', 'Configure the onboarding wizard for new users')}
/>

{#if loading}
	<div class="flex items-center justify-center py-12">
		<RefreshCw class="h-6 w-6 animate-spin" style="color: var(--color-muted)" />
	</div>
{:else}
	{#if error}
		<div
			class="mb-4 rounded-xl border px-4 py-3 text-sm"
			style="border-color: var(--color-danger); background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
		>{error}</div>
	{/if}
	{#if success}
		<div
			class="mb-4 rounded-xl border px-4 py-3 text-sm"
			style="border-color: var(--color-success); background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)"
		>{success}</div>
	{/if}

	<div class="space-y-6">
		<!-- Minimum Engagement Requirements -->
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('onboarding.minimumsTitle', 'Minimum Engagement Requirements')}</h3>
				<p class="text-sm" style="color: var(--color-muted)">{t('onboarding.minimumsDesc', 'Set the minimum number of items users must engage with before completing onboarding.')}</p>
			</CardHeader>
			<CardContent>
				<div class="grid gap-4 sm:grid-cols-3">
					<div>
						<label for="minTags" class="block text-sm font-medium mb-1" style="color: var(--color-foreground)">{t('onboarding.minTags', 'Tags to Follow')}</label>
						<input
							id="minTags"
							type="number"
							min="0"
							max="50"
							bind:value={minTagsFollow}
							class="w-full rounded-lg border px-3 py-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
							disabled={!canManage}
						/>
						<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('onboarding.minTagsHelp', 'Minimum tags to follow in Step 3')}</p>
					</div>
					<div>
						<label for="minUsers" class="block text-sm font-medium mb-1" style="color: var(--color-foreground)">{t('onboarding.minUsers', 'Users to Follow')}</label>
						<input
							id="minUsers"
							type="number"
							min="0"
							max="50"
							bind:value={minUsersFollow}
							class="w-full rounded-lg border px-3 py-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
							disabled={!canManage}
						/>
						<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('onboarding.minUsersHelp', 'Minimum users to follow in Step 4')}</p>
					</div>
					<div>
						<label for="minSpaces" class="block text-sm font-medium mb-1" style="color: var(--color-foreground)">{t('onboarding.minSpaces', 'Spaces to Join')}</label>
						<input
							id="minSpaces"
							type="number"
							min="0"
							max="50"
							bind:value={minSpacesJoin}
							class="w-full rounded-lg border px-3 py-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
							disabled={!canManage}
						/>
						<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('onboarding.minSpacesHelp', 'Minimum spaces to join in Step 4')}</p>
					</div>
				</div>
				<div class="mt-4">
					<Button onclick={saveMinimums} disabled={saving || !canManage}>
						{#if saving}
							<RefreshCw class="mr-2 h-4 w-4 animate-spin" />
						{:else}
							<Save class="mr-2 h-4 w-4" />
						{/if}
						{t('common.save', 'Save Changes')}
					</Button>
				</div>
			</CardContent>
		</Card>

		<!-- Promoted Entities -->
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('onboarding.promotedTitle', 'Promoted Entities')}</h3>
				<p class="text-sm" style="color: var(--color-muted)">{t('onboarding.promotedDesc', 'Entities listed here appear first in the onboarding recommendations (Step 4).')}</p>
			</CardHeader>
			<CardContent>
				<div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
					<div>
						<label for="promoted-spaces" class="block text-sm font-medium mb-1" style="color: var(--color-foreground)">
							{t('onboarding.promotedSpaces', 'Promoted Spaces')}
						</label>
						<TagInput
							options={availableSpaces}
							bind:value={selectedSpaces}
							placeholder="Search spaces to promote..."
							disabled={!canManage}
							id="promoted-spaces"
						/>
						<p class="mt-1 text-xs" style="color: var(--color-muted)">
							{t('onboarding.promotedSpacesHelp', 'Spaces that appear first in onboarding recommendations')}
						</p>
					</div>
					<div>
						<label for="promoted-users" class="block text-sm font-medium mb-1" style="color: var(--color-foreground)">
							{t('onboarding.promotedUsers', 'Promoted Users')}
						</label>
						<TagInput
							options={availableUsers}
							bind:value={selectedUsers}
							placeholder="Search users to promote..."
							disabled={!canManage}
							id="promoted-users"
						/>
						<p class="mt-1 text-xs" style="color: var(--color-muted)">
							{t('onboarding.promotedUsersHelp', 'Users that appear first in onboarding recommendations')}
						</p>
					</div>
					<div>
						<label for="promoted-tags" class="block text-sm font-medium mb-1" style="color: var(--color-foreground)">
							{t('onboarding.promotedTags', 'Promoted Tags')}
						</label>
						<TagInput
							options={availableTags}
							bind:value={selectedTags}
							placeholder="Search tags to promote..."
							disabled={!canManage}
							id="promoted-tags"
						/>
						<p class="mt-1 text-xs" style="color: var(--color-muted)">
							{t('onboarding.promotedTagsHelp', 'Tags that appear first in onboarding recommendations')}
						</p>
					</div>
				</div>
				<div class="mt-4">
					<Button onclick={savePromoted} disabled={saving || !canManage}>
						{#if saving}
							<RefreshCw class="mr-2 h-4 w-4 animate-spin" />
						{:else}
							<Save class="mr-2 h-4 w-4" />
						{/if}
						{t('common.save', 'Save Changes')}
					</Button>
				</div>
			</CardContent>
		</Card>
	</div>
{/if}
