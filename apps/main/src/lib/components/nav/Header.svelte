<script lang="ts">
	import {
		Menu,
		Search,
		Bell,
		Sun,
		Moon,
		Monitor,
		LogOut,
		User,
		Settings,
		ChevronDown
	} from 'lucide-svelte';
	import { DropdownMenu } from 'bits-ui';
	import { theme } from '$lib/stores/theme.svelte';
	import { t, locale } from '$lib/stores/i18n.svelte';
	import { page } from '$app/state';
	import LanguageToggle from '$lib/components/LanguageToggle.svelte';

	type HeaderUser = {
		id: string;
		email: string;
		username: string;
		displayName: string | null;
		avatarUrl: string | null;
		profileType: string;
		status: string;
	};

	let {
		user = null,
		notificationCount = 0,
		onsidenavToggle = () => {}
	}: {
		user: HeaderUser | null;
		notificationCount: number;
		onsidenavToggle: () => void;
	} = $props();

	let searchQuery = $state('');

	function handleSearch(e: Event) {
		e.preventDefault();
		if (searchQuery.trim()) {
			window.location.href = `/search?q=${encodeURIComponent(searchQuery.trim())}`;
		}
	}
</script>

<header
	class="sticky top-0 z-40 border-b backdrop-blur-xl"
	style="background-color: color-mix(in srgb, var(--color-background) 85%, transparent); border-color: var(--color-border)"
>
	<div class="flex h-14 items-center justify-between px-4">
		<!-- Left: hamburger (mobile) + logo -->
		<div class="flex items-center gap-3">
			<button class="flex h-11 w-11 items-center justify-center rounded-lg p-2.5 md:hidden" onclick={onsidenavToggle} aria-label="Toggle navigation">
				<Menu class="h-5 w-5" />
			</button>
			<a href="/" class="text-lg font-bold" style="color: var(--color-primary)">{t('lib.components.nav.Header.svelte.japaconnect')}</a>
		</div>

		<!-- Center: search (desktop) -->
		<form
			class="mx-4 hidden max-w-md flex-1 md:flex"
			onsubmit={handleSearch}
			role="search"
		>
			<div class="relative w-full">
				<Search
					class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
					style="color: var(--color-muted)"
				/>
				<input
					type="search"
					placeholder={t('search.placeholder')}
					bind:value={searchQuery}
					class="w-full rounded-xl border py-2 pl-9 pr-4 text-sm outline-none transition-all focus:border-[var(--color-primary)] focus:ring-2 focus:ring-offset-1"
					style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-foreground); --tw-ring-color: color-mix(in srgb, var(--color-primary) 25%, transparent)"
				aria-label={t('nav.search')}
				/>
			</div>
		</form>

		<!-- Right: search link (mobile), language toggle, notifications, user -->
		<div class="flex items-center gap-1">
		<a
			href="/search"
			class="flex h-11 w-11 items-center justify-center rounded-lg hover:bg-black/5 dark:hover:bg-white/10 md:hidden"
			aria-label="Search"
		>
				<Search class="h-5 w-5" style="color: var(--color-muted)" />
			</a>

			<LanguageToggle />

			{#if user}
			<a
				href="/notifications"
				class="relative flex h-11 w-11 items-center justify-center rounded-xl transition-colors hover:bg-black/5 dark:hover:bg-white/10"
				aria-label={t('nav.notifications')}
			>
					<Bell class="h-5 w-5" style="color: var(--color-muted)" />
					{#if notificationCount > 0}
						<span
							class="absolute -right-0.5 -top-0.5 flex h-4.5 w-4.5 items-center justify-center rounded-full text-[10px] font-bold text-white shadow-sm"
							style="background: var(--gradient-primary)"
						>
							{notificationCount > 99 ? '99+' : notificationCount}
						</span>
					{/if}
				</a>

				<!-- User dropdown -->
				<DropdownMenu.Root>
					<DropdownMenu.Trigger
						class="flex items-center gap-2 rounded-xl p-1 transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					>
						{#if user.avatarUrl}
							<img src={user.avatarUrl} alt="" class="h-8 w-8 rounded-full object-cover ring-2 ring-transparent transition-all hover:ring-[var(--color-primary)]/20" />
						{:else}
							<div
								class="flex h-8 w-8 items-center justify-center rounded-full text-xs font-bold text-white shadow-sm"
								style="background: var(--gradient-primary)"
							>
								{(user.displayName || user.username || 'U').charAt(0).toUpperCase()}
							</div>
						{/if}
						<ChevronDown
							class="hidden h-3 w-3 sm:block"
							style="color: var(--color-muted)"
						/>
					</DropdownMenu.Trigger>
					<DropdownMenu.Content
						align="end"
						class="w-52 rounded-xl border p-1.5 shadow-xl"
						style="background-color: var(--color-background); border-color: var(--color-border)"
					>
						<DropdownMenu.Item
							class="flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors hover:bg-black/5 dark:hover:bg-white/10"
						>
							<User class="h-4 w-4" />
							<a href="/profile/{user.username}">{t('nav.profile')}</a>
						</DropdownMenu.Item>
						<DropdownMenu.Item
							class="flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors hover:bg-black/5 dark:hover:bg-white/10"
						>
							<Settings class="h-4 w-4" />
							<a href="/settings">{t('nav.settings')}</a>
						</DropdownMenu.Item>
						<DropdownMenu.Separator
							class="my-1.5 h-px"
							style="background-color: var(--color-border)"
						/>
						<DropdownMenu.Item
							class="flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors hover:bg-black/5 dark:hover:bg-white/10"
						>
							{#if theme.current === 'dark'}
								<Sun class="h-4 w-4" />
							<button onclick={() => theme.set('light')}>{t('theme.light')}</button>
						{:else if theme.current === 'light'}
							<Monitor class="h-4 w-4" />
							<button onclick={() => theme.set('system')}>{t('theme.system')}</button>
						{:else}
							<Moon class="h-4 w-4" />
							<button onclick={() => theme.set('dark')}>{t('theme.dark')}</button>
							{/if}
						</DropdownMenu.Item>
						<DropdownMenu.Separator
							class="my-1.5 h-px"
							style="background-color: var(--color-border)"
						/>
						<DropdownMenu.Item
							class="flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-sm transition-colors hover:bg-black/5 dark:hover:bg-white/10"
						>
							<LogOut class="h-4 w-4" />
							<a href="/auth/logout">{t('nav.logout')}</a>
						</DropdownMenu.Item>
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			{:else}
				<a
					href="/auth/login"
					class="btn btn-primary px-4 py-1.5 text-sm"
				>
					{t('nav.login')}
				</a>
			{/if}
		</div>
	</div>
</header>
