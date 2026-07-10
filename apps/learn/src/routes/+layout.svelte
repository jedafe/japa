<script lang="ts">
	import '../app.css';
	import { t, locale } from '$lib/stores/i18n.svelte';
	import { setToken } from '$lib/api';
	import { GraduationCap, BookOpen, Award, Menu, X, User, LogOut } from 'lucide-svelte';
	import { onNavigate } from '$app/navigation';

	let { children, data } = $props();

	let mobileMenuOpen = $state(false);

	$effect(() => {
		setToken(data.token ?? null);
	});

	$effect(() => {
		locale.init(data.locale ?? 'en', data.translations ?? {});
	});

	onNavigate((nav) => {
		if (!document.startViewTransition) return;
		return new Promise<void>((resolve) => {
			document.startViewTransition(async () => {
				resolve();
				await nav.complete;
			});
		});
	});

	const navLinks = [
		{ href: '/', label: 'Tracks', icon: BookOpen },
		{ href: '/certificates', label: 'My Certificates', icon: Award }
	];

	function logout() {
		document.cookie = 'learn_token=; path=/; max-age=0';
		document.cookie = 'jc_token=; path=/; max-age=0';
		window.location.href = '/login';
	}
</script>

<svelte:head>
	<link rel="icon" href="/favicon.svg" type="image/svg+xml" />
	<meta name="description" content="JapaConnect Learn — structured learning tracks for your journey abroad" />
</svelte:head>

<div class="flex min-h-screen flex-col">
	<header class="sticky top-0 z-40 border-b backdrop-blur-sm" style="border-color: var(--color-border); background-color: color-mix(in srgb, var(--color-background) 85%, transparent)">
		<div class="mx-auto flex max-w-6xl items-center justify-between px-4 py-3 sm:px-6">
			<a href="/" class="flex items-center gap-2 font-bold tracking-tight">
				<GraduationCap class="h-6 w-6" style="color: var(--color-primary)" />
				<span class="text-lg">{t('learn.brand')}</span>
			</a>

			<!-- Desktop nav -->
			<nav class="hidden items-center gap-6 md:flex">
				{#each navLinks as link}
					<a
						href={link.href}
						class="flex items-center gap-1.5 text-sm font-medium transition-colors hover:opacity-80"
						style="color: var(--color-muted)"
					>
						<link.icon class="h-4 w-4" />
						{link.label}
					</a>
				{/each}

				{#if data.user}
					<div class="flex items-center gap-3">
						<div class="flex items-center gap-2">
							{#if data.user.avatar_url}
								<img src={data.user.avatar_url} alt="" class="h-8 w-8 rounded-full object-cover" />
							{:else}
								<div class="flex h-8 w-8 items-center justify-center rounded-full text-xs font-medium text-white" style="background-color: var(--color-primary)">
									{(data.user.display_name ?? data.user.username ?? 'U').charAt(0).toUpperCase()}
								</div>
							{/if}
							<span class="text-sm font-medium" style="color: var(--color-foreground)">
								{data.user.display_name ?? data.user.username}
							</span>
						</div>
						<button
							onclick={logout}
							class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors hover:opacity-80"
							style="color: var(--color-muted)"
						>
							<LogOut class="h-4 w-4" />
							Sign out
						</button>
					</div>
				{:else}
					<a
						href="/login"
						class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
						style="background-color: var(--color-primary)"
					>
						<User class="h-4 w-4" />
						Sign in
					</a>
				{/if}
			</nav>

			<!-- Mobile menu button -->
			<button
				class="rounded-md p-2 md:hidden"
				style="color: var(--color-muted)"
				onclick={() => (mobileMenuOpen = !mobileMenuOpen)}
				aria-label="Toggle menu"
			>
				{#if mobileMenuOpen}
					<X class="h-5 w-5" />
				{:else}
					<Menu class="h-5 w-5" />
				{/if}
			</button>
		</div>

		<!-- Mobile nav -->
		{#if mobileMenuOpen}
			<div class="border-t px-4 py-3 md:hidden" style="border-color: var(--color-border)">
				<nav class="flex flex-col gap-2">
					{#each navLinks as link}
						<a
							href={link.href}
							class="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors"
							style="color: var(--color-muted)"
							onclick={() => (mobileMenuOpen = false)}
						>
							<link.icon class="h-4 w-4" />
							{link.label}
						</a>
					{/each}
					{#if data.user}
						<div class="mt-2 flex items-center gap-2 border-t px-3 py-2" style="border-color: var(--color-border)">
							{#if data.user.avatar_url}
								<img src={data.user.avatar_url} alt="" class="h-8 w-8 rounded-full object-cover" />
							{:else}
								<div class="flex h-8 w-8 items-center justify-center rounded-full text-xs font-medium text-white" style="background-color: var(--color-primary)">
									{(data.user.display_name ?? data.user.username ?? 'U').charAt(0).toUpperCase()}
								</div>
							{/if}
							<span class="text-sm font-medium" style="color: var(--color-foreground)">
								{data.user.display_name ?? data.user.username}
							</span>
						</div>
						<button
							onclick={logout}
							class="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium"
							style="color: var(--color-muted)"
						>
							<LogOut class="h-4 w-4" />
							Sign out
						</button>
					{:else}
						<a
							href="/login"
							class="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium"
							style="color: var(--color-primary)"
							onclick={() => (mobileMenuOpen = false)}
						>
							<User class="h-4 w-4" />
							Sign in
						</a>
					{/if}
				</nav>
			</div>
		{/if}
	</header>

	<div class="flex-1">
		{@render children()}
	</div>

	<footer class="border-t py-6 text-center text-sm" style="border-color: var(--color-border); color: var(--color-muted)">
		{t('footer.copyright')}
	</footer>
</div>
