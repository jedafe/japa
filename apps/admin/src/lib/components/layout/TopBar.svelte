<script lang="ts">
	import { PUBLIC_JC_ENV } from '$env/static/public';
	import { user, clearUser } from '$lib/stores/auth';
	import { permissions } from '$lib/stores/permissions';
	import { apiFetch } from '$lib/api/client';
	import { theme } from '$lib/stores/theme';
	import Avatar from '$lib/components/ui/Avatar.svelte';
	import { Bell, Sun, Moon } from 'lucide-svelte';

	let showDropdown = $state(false);
	let unreadCount = $state(0);

	function toggleTheme() {
		theme.update((v) => (v === 'dark' ? 'light' : 'dark'));
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
		permissions.set([]);
		window.location.href = '/login';
	}
</script>

<header class="flex h-14 items-center justify-between border-b border-gray-200 bg-white px-4 dark:border-gray-700 dark:bg-gray-900">
	<div class="flex items-center gap-3">
		<a href="/" class="text-sm font-semibold text-gray-900 dark:text-gray-100">JapaConnect Admin</a>
		{#if PUBLIC_JC_ENV === 'production'}
			<span class="rounded bg-red-600 px-2 py-0.5 text-xs font-bold text-white">
				PRODUCTION
			</span>
		{/if}
	</div>

	<div class="flex items-center gap-4">
		<button
			type="button"
			class="relative rounded p-1 text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
		>
			<Bell class="h-4 w-4" />
			{#if unreadCount > 0}
				<span class="absolute -right-1 -top-1 flex h-4 w-4 items-center justify-center rounded-full bg-red-500 text-[10px] text-white">
					{unreadCount}
				</span>
			{/if}
		</button>

		<button
			type="button"
			class="rounded p-1 text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
			onclick={toggleTheme}
		>
			{#if $theme === 'dark'}
				<Sun class="h-4 w-4" />
			{:else}
				<Moon class="h-4 w-4" />
			{/if}
		</button>

		{#if $user}
			<div class="relative">
				<button
					type="button"
					class="flex items-center gap-2 rounded p-1 hover:bg-gray-100 dark:hover:bg-gray-800"
					onclick={() => (showDropdown = !showDropdown)}
				>
					<Avatar src={$user.avatar_url} alt={$user.display_name} size="sm" />
					<span class="text-sm text-gray-700 dark:text-gray-300">{$user.display_name}</span>
				</button>

				{#if showDropdown}
					<div class="absolute right-0 top-full z-50 mt-1 w-48 rounded-md border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800">
						<button
							type="button"
							class="w-full px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-700"
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
