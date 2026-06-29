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

	let {
		user = null,
		notificationCount = 0,
		onsidenavToggle = () => {}
	}: {
		user: any;
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
	class="sticky top-0 z-40 border-b"
	style="background-color: var(--color-background); border-color: var(--color-border)"
>
	<div class="flex h-14 items-center justify-between px-4">
		<!-- Left: hamburger (mobile) + logo -->
		<div class="flex items-center gap-3">
			<button class="p-1 md:hidden" onclick={onsidenavToggle} aria-label="Toggle navigation">
				<Menu class="h-5 w-5" />
			</button>
			<a href="/" class="text-lg font-bold" style="color: var(--color-primary)">JapaConnect</a>
		</div>

		<!-- Center: search (desktop) -->
		<form
			class="mx-4 hidden max-w-md flex-1 md:flex"
			onsubmit={handleSearch}
			role="search"
		>
			<div class="relative w-full">
				<Search
					class="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2"
					style="color: var(--color-muted)"
				/>
				<input
					type="search"
					placeholder="Search communities, posts, people..."
					bind:value={searchQuery}
					class="w-full rounded-lg border py-1.5 pl-9 pr-3 text-sm outline-none transition-colors focus:ring-2"
					style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-foreground)"
					aria-label="Search"
				/>
			</div>
		</form>

		<!-- Right: search link (mobile), notifications, user -->
		<div class="flex items-center gap-1">
			<a
				href="/search"
				class="rounded-md p-2 hover:bg-black/5 dark:hover:bg-white/10 md:hidden"
				aria-label="Search"
			>
				<Search class="h-5 w-5" style="color: var(--color-muted)" />
			</a>

			{#if user}
				<a
					href="/notifications"
					class="relative rounded-md p-2 hover:bg-black/5 dark:hover:bg-white/10"
					aria-label="Notifications"
				>
					<Bell class="h-5 w-5" style="color: var(--color-muted)" />
					{#if notificationCount > 0}
						<span
							class="absolute -right-0.5 -top-0.5 flex h-4 w-4 items-center justify-center rounded-full text-[10px] font-bold text-white"
							style="background-color: var(--color-primary)"
						>
							{notificationCount > 99 ? '99+' : notificationCount}
						</span>
					{/if}
				</a>

				<!-- User dropdown -->
				<DropdownMenu.Root>
					<DropdownMenu.Trigger
						class="flex items-center gap-2 rounded-md p-1 hover:bg-black/5 dark:hover:bg-white/10"
					>
						{#if user.avatarUrl}
							<img src={user.avatarUrl} alt="" class="h-7 w-7 rounded-full object-cover" />
						{:else}
							<div
								class="flex h-7 w-7 items-center justify-center rounded-full text-xs font-bold text-white"
								style="background-color: var(--color-primary)"
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
						class="w-48 rounded-lg border p-1 shadow-lg"
						style="background-color: var(--color-background); border-color: var(--color-border)"
					>
						<DropdownMenu.Item
							class="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-black/5 dark:hover:bg-white/10"
						>
							<User class="h-4 w-4" />
							<a href="/profile/{user.username}">Profile</a>
						</DropdownMenu.Item>
						<DropdownMenu.Item
							class="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-black/5 dark:hover:bg-white/10"
						>
							<Settings class="h-4 w-4" />
							<a href="/settings">Settings</a>
						</DropdownMenu.Item>
						<DropdownMenu.Separator
							class="my-1 h-px"
							style="background-color: var(--color-border)"
						/>
						<DropdownMenu.Item
							class="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-black/5 dark:hover:bg-white/10"
						>
							{#if theme.current === 'dark'}
								<Sun class="h-4 w-4" />
								<button onclick={() => theme.set('light')}>Light mode</button>
							{:else if theme.current === 'light'}
								<Monitor class="h-4 w-4" />
								<button onclick={() => theme.set('system')}>System mode</button>
							{:else}
								<Moon class="h-4 w-4" />
								<button onclick={() => theme.set('dark')}>Dark mode</button>
							{/if}
						</DropdownMenu.Item>
						<DropdownMenu.Separator
							class="my-1 h-px"
							style="background-color: var(--color-border)"
						/>
						<DropdownMenu.Item
							class="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-black/5 dark:hover:bg-white/10"
						>
							<LogOut class="h-4 w-4" />
							<a href="/auth/logout">Logout</a>
						</DropdownMenu.Item>
					</DropdownMenu.Content>
				</DropdownMenu.Root>
			{:else}
				<a
					href="/auth/login"
					class="rounded-md px-3 py-1.5 text-sm font-medium text-white"
					style="background-color: var(--color-primary)"
				>
					Sign in
				</a>
			{/if}
		</div>
	</div>
</header>
