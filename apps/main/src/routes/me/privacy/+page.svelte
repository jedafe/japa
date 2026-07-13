<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Shield, Cookie } from 'lucide-svelte';
	import { page as pageState } from '$app/state';
	import { invalidateAll } from '$app/navigation';

	type PrivacySettings = {
		profile_visibility: string;
		show_email: boolean;
		show_online_status: boolean;
		allow_search_indexing: boolean;
	};

	type ConsentItem = {
		category: string;
		granted: boolean;
		granted_at?: string;
	};

	let { data } = $props();
	let settings = $derived(data.data?.privacy ?? null);
	let consents = $derived<ConsentItem[]>(data.data?.consent ?? []);

	let activeTab = $derived.by(() => {
		const path = pageState.url.pathname;
		if (path.includes('#consent')) return 'consent';
		return 'privacy';
	});

	let saving = $state(false);
	let consentSaving = $state<string | null>(null);

	async function savePrivacy() {
		if (!settings) return;
		saving = true;
		try {
			await api.updateMePrivacy(settings);
		} catch {
		} finally {
			saving = false;
		}
	}

	async function toggleConsent(item: ConsentItem) {
		consentSaving = item.category;
		try {
			await api.updateConsent({ category: item.category, granted: !item.granted });
			await invalidateAll();
		} catch {
		} finally {
			consentSaving = null;
		}
	}
</script>

<svelte:head>
	<title>{t('me.privacy.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.privacy.title')}</h1>

<!-- Tab navigation -->
<div class="mb-6 flex gap-1 border-b" style="border-color: var(--color-border)">
	<a
		href="/me/privacy"
		class="flex items-center gap-1.5 px-4 py-2 text-sm font-medium border-b-2 transition-colors {activeTab === 'privacy' ? 'border-current' : 'border-transparent'}"
		style="color: {activeTab === 'privacy' ? 'var(--color-primary)' : 'var(--color-muted)'}"
	>
		<Shield class="h-4 w-4" />
		Privacy
	</a>
	<a
		href="/me/privacy#consent"
		class="flex items-center gap-1.5 px-4 py-2 text-sm font-medium border-b-2 transition-colors {activeTab === 'consent' ? 'border-current' : 'border-transparent'}"
		style="color: {activeTab === 'consent' ? 'var(--color-primary)' : 'var(--color-muted)'}"
	>
		<Cookie class="h-4 w-4" />
		Consent
	</a>
</div>

{#if activeTab === 'privacy'}
	{#if !settings}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
	{:else}
		<div class="max-w-xl space-y-4 rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
			<div class="flex items-center justify-between">
				<div>
					<p class="font-medium text-sm">{t('me.privacy.show_email')}</p>
					<p class="text-xs" style="color: var(--color-muted)">{t('me.privacy.show_email_desc')}</p>
				</div>
				<input type="checkbox" bind:checked={settings.show_email} class="rounded" />
			</div>
			<div class="flex items-center justify-between">
				<div>
					<p class="font-medium text-sm">{t('me.privacy.show_online')}</p>
					<p class="text-xs" style="color: var(--color-muted)">{t('me.privacy.show_online_desc')}</p>
				</div>
				<input type="checkbox" bind:checked={settings.show_online_status} class="rounded" />
			</div>
			<div class="flex items-center justify-between">
				<div>
					<p class="font-medium text-sm">{t('me.privacy.allow_indexing')}</p>
					<p class="text-xs" style="color: var(--color-muted)">{t('me.privacy.allow_indexing_desc')}</p>
				</div>
				<input type="checkbox" bind:checked={settings.allow_search_indexing} class="rounded" />
			</div>
			<button class="btn btn-primary text-xs" onclick={savePrivacy} disabled={saving}>
				{saving ? t('common.saving') : t('common.save')}
			</button>
		</div>
	{/if}
{:else if activeTab === 'consent'}
	{#if consents.length === 0}
		<div class="rounded-lg border p-6 text-center text-sm" style="border-color: var(--color-border); color: var(--color-muted)">
			No consent preferences found.
		</div>
	{:else}
		<div class="max-w-xl space-y-3 rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
			<p class="mb-4 text-sm" style="color: var(--color-muted)">
				Manage how your data is used across the platform.
			</p>
			{#each consents as item (item.category)}
				<div class="flex items-center justify-between py-2">
					<div>
						<p class="font-medium text-sm capitalize">{item.category.replace(/_/g, ' ')}</p>
						{#if item.granted_at}
							<p class="text-xs" style="color: var(--color-muted)">Last updated: {new Date(item.granted_at).toLocaleDateString()}</p>
						{/if}
					</div>
					<button
						onclick={() => toggleConsent(item)}
						disabled={consentSaving === item.category}
						class="rounded-full px-3 py-1 text-xs font-medium transition-colors {item.granted
							? 'bg-green-100 text-green-800 hover:bg-green-200 dark:bg-green-900/30 dark:text-green-400'
							: 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-400'}"
					>
						{consentSaving === item.category ? '...' : item.granted ? 'Granted' : 'Denied'}
					</button>
				</div>
			{/each}
		</div>
	{/if}
{/if}
