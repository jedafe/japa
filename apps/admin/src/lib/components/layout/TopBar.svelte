<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { PUBLIC_JC_ENV } from '$env/static/public';
	import { user, clearUser } from '$lib/stores/auth.svelte';
	import { apiFetch } from '$lib/api/client';
	import { clearAccessToken } from '$lib/api/token';
	import { theme } from '$lib/stores/theme.svelte';
	import { getNotifications, getUnreadCount, markRead, markAllRead } from '$lib/stores/notifications.svelte';
	import { goto, invalidateAll } from '$app/navigation';
	import Avatar from '$lib/components/ui/Avatar.svelte';
	import { Bell, Sun, Moon, Check, Menu, PanelLeft } from 'lucide-svelte';

	let { onMenuClick, onCollapseToggle }: { onMenuClick?: () => void; onCollapseToggle?: () => void } = $props();

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
		try {
			await apiFetch('/auth/logout', { method: 'POST' });
		} catch (err) {
			console.warn('[logout] backend logout failed', err);
		}
		clearUser();
		clearAccessToken();
		await goto('/login', { invalidateAll: true });
	}
</script>

<svelte:window onclick={closeDropdowns} />

<header class="flex h-14 items-center justify-between border-b px-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
	<div class="flex items-center gap-3">
		<button
			type="button"
			class="rounded-md p-1.5 transition-colors hover:bg-black/5 dark:hover:bg-white/10 md:hidden"
			style="color: var(--color-foreground)"
			onclick={onMenuClick}
			aria-label={t('lib.components.layout.TopBar.svelte.toggle_sidebar', 'Toggle sidebar')}
		>
			<Menu class="h-5 w-5" />
		</button>
		<button
			type="button"
			data-sidebar-collapse
			class="hidden rounded-md p-1.5 transition-colors hover:bg-black/5 dark:hover:bg-white/10 md:inline-flex"
			style="color: var(--color-foreground)"
			onclick={onCollapseToggle}
			aria-label={t('lib.components.layout.TopBar.svelte.toggle_sidebar', 'Toggle sidebar')}
		>
			<PanelLeft class="h-5 w-5 rtl:-scale-x-100" />
		</button>
		<a href="/" class="text-sm font-semibold" style="color: var(--color-foreground)">{t('lib.components.layout.TopBar.svelte.japaconnect_admin')}</a>
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
					<span class="absolute -end-1 -top-1 flex h-4 w-4 items-center justify-center rounded-full bg-red-500 text-[10px] text-white">
						{getUnreadCount()}
					</span>
				{/if}
			</button>

			{#if showNotifications}
				<div class="absolute end-0 top-full z-50 mt-1 w-80 rounded-md border shadow-lg" style="border-color: var(--color-border); background-color: var(--color-surface)">
					<div class="flex items-center justify-between border-b px-3 py-2" style="border-color: var(--color-border)">
						<span class="text-sm font-medium" style="color: var(--color-foreground)">{t('lib.components.layout.TopBar.svelte.notifications')}</span>
						<button
							type="button"
							class="text-xs hover:underline"
							style="color: var(--color-primary)"
							onclick={() => markAllRead()}
						>
							{t('lib.components.layout.TopBar.svelte.mark_all_read', 'Mark all read')}
						</button>
					</div>
					<div class="max-h-64 overflow-y-auto">
						{#if getNotifications().length === 0}
							<p class="px-3 py-4 text-center text-sm" style="color: var(--color-muted)">{t('lib.components.layout.TopBar.svelte.no_notifications')}</p>
						{:else}
							{#each getNotifications() as notif (notif.id)}
								<button
									type="button"
									class="w-full px-3 py-2 text-start hover:bg-black/[0.02] dark:hover:bg-white/[0.05] {notif.read ? 'opacity-60' : ''}"
									onclick={() => {
										markRead(notif.id);
										// action_url values are in-app paths; goto() throws on
										// absolute URLs, so only navigate for path values.
										if (notif.link?.startsWith('/')) goto(notif.link);
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
					<Avatar src={user.current.avatar_url} alt={user.current.username} size="sm" />
					<span class="text-sm" style="color: var(--color-foreground)">{user.current.username}</span>
				</button>

				{#if showDropdown}
					<div class="absolute end-0 top-full z-50 mt-1 w-48 rounded-md border shadow-lg" style="border-color: var(--color-border); background-color: var(--color-surface)">
						<button
							type="button"
							class="w-full px-4 py-2 text-start text-sm hover:bg-black/[0.02] dark:hover:bg-white/[0.05]"
							style="color: var(--color-foreground)"
							onclick={logout}
						>
							{t('lib.components.layout.TopBar.svelte.logout', 'Logout')}
						</button>
					</div>
				{/if}
			</div>
		{/if}
		
		<!-- Logo -->
		<a href="/" class="flex items-center gap-2 ms-2">
			<div class="flex h-8 w-8 items-center justify-center rounded-lg" style="background-color: var(--color-primary)">
				<span class="text-sm font-bold text-white">JC</span>
			</div>
		</a>
	</div>
</header>
