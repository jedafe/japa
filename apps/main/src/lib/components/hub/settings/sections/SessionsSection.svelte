<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { revokeSession, revokeOtherSessions } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';

	// Matches the JEOS §50 contract: id, device_name, ip_address,
	// last_used_at, created_at, expires_at, current.
	interface Session {
		id: string;
		device_name?: string;
		ip_address?: string;
		last_used_at?: string;
		created_at?: string;
		current?: boolean;
	}

	interface SessionsData {
		sessions?: Session[];
	}

	let { data }: { data: unknown } = $props();
	const payload = $derived(data as SessionsData | null);
	let sessions = $state<Session[]>(untrack(() => payload?.sessions ?? []));

	// Resync when the loader data is replaced (e.g. invalidateAll) — the
	// untracked init alone leaves the list permanently stale (BUG-P014).
	// Reads only the payload so optimistic removals don't retrigger it.
	$effect(() => {
		sessions = payload?.sessions ?? [];
	});

	async function doRevoke(id: string) {
		const idx = sessions.findIndex((s) => s.id === id);
		if (idx < 0) return;
		const removed = sessions[idx];
		await optimisticAction({
			apply: () => (sessions = sessions.filter((s) => s.id !== id)),
			rollback: () => (sessions = [...sessions.slice(0, idx), removed, ...sessions.slice(idx + 1)]),
			call: () => revokeSession(id)
		});
	}

	async function doRevokeOthers() {
		// Snapshot the pre-revoke list so rollback restores the exact prior
		// state (a currents-then-others rebuild would scramble the order).
		const prev = sessions;
		await optimisticAction({
			apply: () => (sessions = sessions.filter((s) => s.current)),
			rollback: () => (sessions = prev),
			call: () => revokeOtherSessions()
		});
	}
</script>

<section class="max-w-2xl space-y-6">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.sessions')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.security.sessions_desc')}</p>
	</header>

	<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center justify-between">
			<h2 class="text-sm font-medium text-[var(--color-text)]">{t('settings.security.sessions_title')}</h2>
			{#if sessions.filter((s) => !s.current).length > 0}
				<button
					type="button"
					class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
					onclick={doRevokeOthers}
				>
					{t('settings.security.revoke_others')}
				</button>
			{/if}
		</div>
		<ul class="space-y-2">
			{#each sessions as s (s.id)}
				<li class="flex items-center justify-between rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2">
					<div>
						<p class="text-sm text-[var(--color-text)]">
							{s.device_name ?? s.ip_address ?? t('settings.security.session_unknown')}
							{#if s.current}<span class="ml-2 text-xs text-[var(--color-primary)]">{t('settings.security.session_current')}</span>{/if}
						</p>
						{#if s.last_used_at}
							<p class="text-xs text-[var(--color-muted)]">{t('settings.security.session_last_active')}: {s.last_used_at}</p>
						{/if}
					</div>
					{#if !s.current}
						<button
							type="button"
							class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger,#dc2626)]"
							onclick={() => doRevoke(s.id)}
						>
							{t('settings.security.revoke')}
						</button>
					{/if}
				</li>
			{/each}
		</ul>
	</div>
</section>
