<script lang="ts">
	import { ArrowLeft, Loader2, Monitor, Smartphone, Trash2 } from 'lucide-svelte';
	import { onMount } from 'svelte';

	type Session = {
		id: string;
		ip_address: string;
		user_agent: string;
		created_at: string;
		last_active_at: string;
		is_current: boolean;
	};

	let sessions = $state<Session[]>([]);
	let loading = $state(true);
	let error = $state('');
	let revoking = $state<string | null>(null);

	onMount(loadSessions);

	async function loadSessions() {
		loading = true;
		error = '';
		try {
			const res = await fetch('/api/v1/me/sessions', { credentials: 'include' });
			if (res.ok) {
				const { data } = await res.json();
				sessions = data ?? [];
			} else {
				error = 'Failed to load sessions.';
			}
		} catch {
			error = 'Failed to load sessions.';
		} finally {
			loading = false;
		}
	}

	async function revokeSession(id: string) {
		revoking = id;
		try {
			await fetch(`/api/v1/me/sessions/${id}`, { method: 'DELETE', credentials: 'include' });
			sessions = sessions.filter((s) => s.id !== id);
		} catch {
			error = 'Failed to revoke session.';
		} finally {
			revoking = null;
		}
	}

	async function revokeAllSessions() {
		if (!confirm('Are you sure you want to revoke all other sessions?')) return;
		try {
			await fetch('/api/v1/me/sessions', { method: 'DELETE', credentials: 'include' });
			sessions = sessions.filter((s) => s.is_current);
		} catch {
			error = 'Failed to revoke sessions.';
		}
	}

	function isMobile(ua: string): boolean {
		return /Mobile|Android|iPhone|iPad/i.test(ua);
	}

	function parseBrowser(ua: string): string {
		if (ua.includes('Chrome')) return 'Chrome';
		if (ua.includes('Firefox')) return 'Firefox';
		if (ua.includes('Safari')) return 'Safari';
		if (ua.includes('Edge')) return 'Edge';
		return 'Unknown Browser';
	}

	function timeAgo(date: string): string {
		const seconds = Math.floor((Date.now() - new Date(date).getTime()) / 1000);
		if (seconds < 60) return 'just now';
		if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
		if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
		return `${Math.floor(seconds / 86400)}d ago`;
	}
</script>

<svelte:head>
	<title>Sessions | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-2xl px-4 py-8">
	<a
		href="/settings"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to Settings
	</a>

	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">Active Sessions</h1>
		{#if sessions.length > 1}
			<button
				onclick={revokeAllSessions}
				class="rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
				style="border-color: var(--color-danger); color: var(--color-danger)"
			>
				Revoke all others
			</button>
		{/if}
	</div>

	{#if error}
		<div class="mb-6 rounded-lg border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{error}
		</div>
	{/if}

	{#if loading}
		<div class="space-y-3">
			{#each [1, 2, 3] as _}
				<div class="h-20 animate-pulse rounded-lg" style="background-color: var(--color-background)"></div>
			{/each}
		</div>
	{:else}
		<div class="space-y-3">
			{#each sessions as session (session.id)}
				<div
					class="flex items-center justify-between rounded-xl border p-4"
					style="border-color: {session.is_current ? 'var(--color-primary)' : 'var(--color-border)'}; background: var(--color-surface);"
				>
					<div class="flex items-center gap-3">
						{#if isMobile(session.user_agent)}
							<Smartphone class="h-8 w-8" style="color: var(--color-muted)" />
						{:else}
							<Monitor class="h-8 w-8" style="color: var(--color-muted)" />
						{/if}
						<div>
							<div class="flex items-center gap-2">
								<span class="text-sm font-medium" style="color: var(--color-foreground)">
									{parseBrowser(session.user_agent)}
								</span>
								{#if session.is_current}
									<span
										class="rounded-full px-2 py-0.5 text-xs font-medium"
										style="background-color: color-mix(in srgb, var(--color-primary) 15%, transparent); color: var(--color-primary)"
									>
										Current
									</span>
								{/if}
							</div>
							<p class="text-xs" style="color: var(--color-muted)">
								{session.ip_address} &middot; Active {timeAgo(session.last_active_at)}
							</p>
						</div>
					</div>

					{#if !session.is_current}
						<button
							onclick={() => revokeSession(session.id)}
							disabled={revoking === session.id}
							class="rounded-lg p-2 text-sm transition-colors hover:bg-black/5 dark:hover:bg-white/10"
							style="color: var(--color-danger)"
							title="Revoke session"
						>
							{#if revoking === session.id}
								<Loader2 class="h-4 w-4 animate-spin" />
							{:else}
								<Trash2 class="h-4 w-4" />
							{/if}
						</button>
					{/if}
				</div>
			{:else}
				<p class="text-center text-sm" style="color: var(--color-muted)">No active sessions.</p>
			{/each}
		</div>
	{/if}
</main>
