<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { revokeOAuthGrant } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Bot } from 'lucide-svelte';

	/**
	 * Connected apps settings — §120 OAuth agent grants. Lists the agent
	 * clients (claude.ai connectors, ChatGPT, …) the user has authorized via
	 * the /oauth/authorize consent flow, backed by GET /me/oauth/grants.
	 * Revoke calls DELETE /me/oauth/grants/{clientId} which drops the whole
	 * client/user token family.
	 */
	interface OAuthGrant {
		client_id: string;
		client_name: string;
		scopes: string[];
		first_granted_at: string;
		last_granted_at: string;
	}

	let { data }: { data: unknown } = $props();
	let grants = $state<OAuthGrant[]>(untrack(() => (data as OAuthGrant[] | null) ?? []));

	// Resync when the loader data is replaced (e.g. invalidateAll) — the
	// untracked init alone leaves the list permanently stale (BUG-P014).
	// Reads only `data` so optimistic removals don't retrigger it.
	$effect(() => {
		grants = (data as OAuthGrant[] | null) ?? [];
	});

	function fmtDate(iso: string): string {
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? iso : d.toLocaleDateString();
	}

	// Friendly labels for scopes whose raw value isn't self-explanatory.
	const scopeLabels: Record<string, string> = {
		openid: 'profile',
	};
	const scopeLabel = (s: string) => scopeLabels[s] ?? s;

	async function doRevoke(clientId: string) {
		const idx = grants.findIndex((g) => g.client_id === clientId);
		if (idx < 0) return;
		const removed = grants[idx];
		await optimisticAction({
			apply: () => (grants = grants.filter((g) => g.client_id !== clientId)),
			rollback: () => (grants = [...grants.slice(0, idx), removed, ...grants.slice(idx + 1)]),
			call: () => revokeOAuthGrant(clientId)
		});
	}
</script>

<section class="max-w-2xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.connected_apps')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.connected_apps.desc')}</p>
	</header>

	{#if grants.length === 0}
		<EmptyState icon={Bot} title={t('settings.connected_apps.empty_title')} body={t('settings.connected_apps.empty_body')} />
	{:else}
		<ul class="space-y-2">
			{#each grants as grant (grant.client_id)}
				<li class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
					<div class="flex items-center justify-between">
						<div class="min-w-0">
							<p class="truncate text-sm font-medium text-[var(--color-text)]">{grant.client_name || grant.client_id}</p>
							{#if grant.last_granted_at}
								<p class="text-xs text-[var(--color-muted)]">{t('settings.connected_apps.granted')} {fmtDate(grant.last_granted_at)}</p>
							{/if}
						</div>
						<button
							type="button"
							class="shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							onclick={() => doRevoke(grant.client_id)}
						>
							{t('settings.connected_apps.revoke')}
						</button>
					</div>
					{#if grant.scopes?.length}
						<ul class="mt-2 flex flex-wrap gap-1">
							{#each grant.scopes as scope (scope)}
								<li class="rounded-full border border-[var(--color-border)] px-2 py-0.5 text-xs text-[var(--color-muted)]">{scopeLabel(scope)}</li>
							{/each}
						</ul>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</section>
