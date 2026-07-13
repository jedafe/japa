<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { User, Mail, Calendar } from 'lucide-svelte';

	type Account = {
		id: string;
		email: string;
		name: string;
		username: string;
		created_at: string;
		avatar_url?: string;
	};

	let { data } = $props();
	let account = $derived(data.data ?? null);
</script>

<svelte:head>
	<title>{t('me.account.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.account.title')}</h1>

{#if !account}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="max-w-xl space-y-4 rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
		<div class="flex items-center gap-4">
			{#if account.avatar_url}
				<img src={account.avatar_url} alt={account.name} class="h-16 w-16 rounded-full object-cover" />
			{:else}
				<div class="flex h-16 w-16 items-center justify-center rounded-full" style="background: var(--color-primary); color: white">
					<User class="h-8 w-8" />
				</div>
			{/if}
			<div>
				<h2 class="text-lg font-semibold">{account.name}</h2>
				<p class="text-sm" style="color: var(--color-muted)">@{account.username}</p>
			</div>
		</div>
		<div class="space-y-2 text-sm">
			<div class="flex items-center gap-2" style="color: var(--color-muted)">
				<Mail class="h-4 w-4" />
				{account.email}
			</div>
			<div class="flex items-center gap-2" style="color: var(--color-muted)">
				<Calendar class="h-4 w-4" />
				{t('me.account.joined')}: {new Date(account.created_at).toLocaleDateString()}
			</div>
		</div>
	</div>
{/if}
