<script lang="ts">
	// Ref: japaconnect-engineering/sections/46-impersonation.md (UI Banner)
	// Yellow sticky banner shown at the top of every admin page during an
	// active impersonation session. Polls /admin/impersonate/status every
	// 60 seconds. Warns the admin at 5 minutes remaining. On "End Session"
	// click, calls DELETE /admin/impersonate and reloads.
	import { onMount, onDestroy } from 'svelte';
	import { impersonationApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { restoreSession } from '$lib/stores/session.svelte';
	import { goto } from '$app/navigation';
	import { LogOut, ShieldAlert } from 'lucide-svelte';

	type Status = {
		active: boolean;
		target_user_id: string | null;
		expires_at: string | null;
	};

	let status = $state<Status>({ active: false, target_user_id: null, expires_at: null });
	let ending = $state(false);
	let pollTimer: ReturnType<typeof setInterval> | undefined;

	const POLL_INTERVAL_MS = 60_000; // 60 seconds per Section 46
	const WARNING_THRESHOLD_MS = 5 * 60 * 1000; // 5 minutes

	async function loadStatus() {
		try {
			const result = await impersonationApi.status();
			if (result.ok) {
				status = result.data;
			}
		} catch {
			// Silent fail — banner is non-critical UI
		}
	}

	async function endSession() {
		ending = true;
		try {
			await impersonationApi.end();
			status = { active: false, target_user_id: null, expires_at: null };
			// Restore the admin's own session in-place (re-fetches /me +
			// /me/permissions) then navigate to admin root. invalidateAll
			// triggers the layout's reactive dependencies so the permission-
			// gated UI re-evaluates without a full page reload.
			await restoreSession();
			await goto('/', { invalidateAll: true });
		} catch {
			// If end fails, restore session and navigate anyway to reset state
			await restoreSession();
			await goto('/', { invalidateAll: true });
		} finally {
			ending = false;
		}
	}

	let remainingMs = $derived.by(() => {
		if (!status.expires_at) return null;
		const expiry = new Date(status.expires_at).getTime();
		return expiry - Date.now();
	});

	let remainingMinutes = $derived(
		remainingMs !== null ? Math.max(0, Math.ceil(remainingMs / 60_000)) : null
	);

	let isWarning = $derived(remainingMs !== null && remainingMs <= WARNING_THRESHOLD_MS);

	onMount(() => {
		loadStatus();
		pollTimer = setInterval(loadStatus, POLL_INTERVAL_MS);
	});

	onDestroy(() => {
		if (pollTimer) clearInterval(pollTimer);
	});
</script>

{#if status.active}
	<div
		class="flex items-center justify-between gap-3 px-4 py-2 text-sm"
		class:animate-pulse={isWarning}
		style="background-color: color-mix(in srgb, var(--color-warning) 15%, transparent); border-bottom: 1px solid var(--color-warning); color: var(--color-warning)"
		role="alert"
		aria-live="polite"
	>
		<div class="flex items-center gap-2">
			<ShieldAlert class="h-4 w-4 shrink-0" />
			<span>
				{t('impersonate.banner_text', 'Viewing as')}
				<strong>{status.target_user_id ?? '—'}</strong>
			</span>
			{#if remainingMinutes !== null}
				<span class="text-xs opacity-80">
					{t('impersonate.remaining', 'Session expires in {n}m').replace('{n}', String(remainingMinutes))}
				</span>
			{/if}
		</div>
		<button
			class="flex items-center gap-1.5 rounded-md px-3 py-1 text-xs font-medium transition-colors hover:opacity-90"
			style="background-color: var(--color-warning); color: white"
			onclick={endSession}
			disabled={ending}
		>
			<LogOut class="h-3.5 w-3.5" />
			{t('impersonate.end_session', 'End Session')}
		</button>
	</div>
{/if}
