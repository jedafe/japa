<script lang="ts">
	import { PUBLIC_JC_ENV, PUBLIC_JC_API_URL } from '$env/static/public';
	import { user } from '$lib/stores/auth';
	import { theme } from '$lib/stores/theme';
	import { notifications, unreadCount, markRead, markAllRead } from '$lib/stores/notifications';
	import Avatar from '$lib/components/ui/Avatar.svelte';
	import { Bell, Sun, Moon, Check } from 'lucide-svelte';

	let showDropdown = $state(false);
	let showNotifications = $state(false);

	function toggleTheme() {
		theme.update((v) => (v === 'dark' ? 'light' : 'dark'));
	}

	function closeDropdowns(e: MouseEvent) {
		const target = e.target as HTMLElement;
		if (!target.closest('[data-notification-menu]') && !target.closest('[data-user-menu]')) {
			showDropdown = false;
			showNotifications = false;
		}
	}

	async function logout() {
		try {
			await fetch(`${PUBLIC_JC_API_URL}/api/v1/auth/logout`, {
				method: 'POST',
				credentials: 'include'
			});
		} catch {
			// Continue with client-side cleanup even if the request fails
		}
		user.set(null);
		window.location.href = '/login';
	}
</script>

<svelte:window onclick={closeDropdowns} />

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
		<div class="relative" data-notification-menu>
			<button
				type="button"
				class="relative rounded p-1 text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
				onclick={() => { showNotifications = !showNotifications; showDropdown = false; }}
			>
				<Bell class="h-4 w-4" />
				{#if $unreadCount > 0}
					<span class="absolute -right-1 -top-1 flex h-4 w-4 items-center justify-center rounded-full bg-red-500 text-[10px] text-white">
						{$unreadCount}
					</span>
				{/if}
			</button>

			{#if showNotifications}
				<div class="absolute right-0 top-full z-50 mt-1 w-80 rounded-md border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800">
					<div class="flex items-center justify-between border-b border-gray-100 px-3 py-2 dark:border-gray-700">
						<span class="text-sm font-medium text-gray-700 dark:text-gray-300">Notifications</span>
						<button
							type="button"
							class="text-xs text-blue-600 hover:underline dark:text-blue-400"
							onclick={() => markAllRead()}
						>
							Mark all read
						</button>
					</div>
					<div class="max-h-64 overflow-y-auto">
						{#if $notifications.length === 0}
							<p class="px-3 py-4 text-center text-sm text-gray-500 dark:text-gray-400">No notifications</p>
						{:else}
							{#each $notifications as notif}
								<button
									type="button"
									class="w-full px-3 py-2 text-left hover:bg-gray-50 dark:hover:bg-gray-700 {notif.read ? 'opacity-60' : ''}"
									onclick={() => {
										markRead(notif.id);
										if (notif.link) window.location.href = notif.link;
									}}
								>
									<p class="text-sm font-medium text-gray-800 dark:text-gray-200">{notif.title}</p>
									<p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{notif.body}</p>
								</button>
							{/each}
						{/if}
					</div>
				</div>
			{/if}
		</div>

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
