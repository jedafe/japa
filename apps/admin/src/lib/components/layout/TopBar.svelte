<script lang="ts">
	import { PUBLIC_JC_ENV } from '$env/static/public';
	import { user, clearUser } from '$lib/stores/auth.svelte';
	import { apiFetch } from '$lib/api/client';
	import { theme } from '$lib/stores/theme.svelte';
	import { getNotifications, getUnreadCount, markRead, markAllRead } from '$lib/stores/notifications.svelte';
	import Avatar from '$lib/components/ui/Avatar.svelte';
	import { Bell, Sun, Moon, Check } from 'lucide-svelte';

	let showDropdown = $state(false);
	let showNotifications = $state(false);

	function toggleTheme() {
		theme.set(theme.current === 'dark' ? 'light' : 'dark');
	}

	function closeDropdowns(e: MouseEvent) {
		const target = e.target as HTMLElement;
		if (!target.closest('[data-notification-menu]') && !target.closest('[data-user-menu]')) {
			showDropdown = false;
			showNotifications = false;
		}
	}

	async function logout() {
		// NOTE (AUDIT-FE-ADM-003, partial fix only): jc_token is httpOnly, so
		// it can never be cleared from client JS, and this app ships as a
		// static SPA (adapter-static + fallback: 'index.html', see
		// svelte.config.js) with no live server to run a cookie-clearing
		// action in production. This best-effort revokes the session
		// server-side (Go backend drops its jc_refresh cookie + JTI
		// allowlist entry) and clears in-memory client state, but the
		// jc_token cookie itself keeps being sent until it naturally expires.
		// Do not consider AUDIT-FE-ADM-003 closed — a full fix needs
		// AUDIT-FE-ADM-002 resolved first (move off adapter-static, or add a
		// real server endpoint capable of clearing the cookie).
		await apiFetch('/auth/logout', { method: 'POST' }).catch(() => {});
		clearUser();
		window.location.href = '/login';
	}
</script>

<svelte:window onclick={closeDropdowns} />

<header class="flex h-14 items-center justify-between border-b px-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
	<div class="flex items-center gap-3">
		<a href="/" class="text-sm font-semibold" style="color: var(--color-foreground)">JapaConnect Admin</a>
		{#if PUBLIC_JC_ENV === 'production'}
			<span class="rounded bg-red-600 px-2 py-0.5 text-xs font-bold text-white">
				PRODUCTION
			</span>
		{/if}
	</div>

	<div class="flex items-center gap-4">
		<div class="relative" data-notification-menu>
			<button
				type="button"
				class="relative rounded p-1 hover:bg-black/5 dark:hover:bg-white/10"
				style="color: var(--color-muted)"
				onclick={() => { showNotifications = !showNotifications; showDropdown = false; }}
			>
				<Bell class="h-4 w-4" />
				{#if getUnreadCount() > 0}
					<span class="absolute -right-1 -top-1 flex h-4 w-4 items-center justify-center rounded-full bg-red-500 text-[10px] text-white">
						{getUnreadCount()}
					</span>
				{/if}
			</button>

			{#if showNotifications}
				<div class="absolute right-0 top-full z-50 mt-1 w-80 rounded-md border shadow-lg" style="border-color: var(--color-border); background-color: var(--color-surface)">
					<div class="flex items-center justify-between border-b px-3 py-2" style="border-color: var(--color-border)">
						<span class="text-sm font-medium" style="color: var(--color-foreground)">Notifications</span>
						<button
							type="button"
							class="text-xs hover:underline"
							style="color: var(--color-primary)"
							onclick={() => markAllRead()}
						>
							Mark all read
						</button>
					</div>
					<div class="max-h-64 overflow-y-auto">
						{#if getNotifications().length === 0}
							<p class="px-3 py-4 text-center text-sm" style="color: var(--color-muted)">No notifications</p>
						{:else}
							{#each getNotifications() as notif}
								<button
									type="button"
									class="w-full px-3 py-2 text-left hover:bg-black/[0.02] dark:hover:bg-white/[0.05] {notif.read ? 'opacity-60' : ''}"
									onclick={() => {
										markRead(notif.id);
										if (notif.link) window.location.href = notif.link;
									}}
								>
									<p class="text-sm font-medium" style="color: var(--color-foreground)">{notif.title}</p>
									<p class="mt-0.5 text-xs" style="color: var(--color-muted)">{notif.body}</p>
								</button>
							{/each}
						{/if}
					</div>
				</div>
			{/if}
		</div>

		<button
			type="button"
			class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10"
			style="color: var(--color-muted)"
			onclick={toggleTheme}
		>
			{#if theme.current === 'dark'}
				<Sun class="h-4 w-4" />
			{:else}
				<Moon class="h-4 w-4" />
			{/if}
		</button>

		{#if user.current}
			<div class="relative" data-user-menu>
				<button
					type="button"
					class="flex items-center gap-2 rounded p-1 hover:bg-black/5 dark:hover:bg-white/10"
					onclick={() => (showDropdown = !showDropdown)}
				>
					<Avatar src={user.current.avatar_url} alt={user.current.display_name} size="sm" />
					<span class="text-sm" style="color: var(--color-foreground)">{user.current.display_name}</span>
				</button>

				{#if showDropdown}
					<div class="absolute right-0 top-full z-50 mt-1 w-48 rounded-md border shadow-lg" style="border-color: var(--color-border); background-color: var(--color-surface)">
						<button
							type="button"
							class="w-full px-4 py-2 text-left text-sm hover:bg-black/[0.02] dark:hover:bg-white/[0.05]"
							style="color: var(--color-foreground)"
							onclick={logout}
						>
							Logout
						</button>
					</div>
				{/if}
			</div>
		{/if}
	</div>
</header>
