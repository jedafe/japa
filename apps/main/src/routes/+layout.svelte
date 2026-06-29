<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { Header, MobileNav, Footer, SideNav } from '$lib/components/nav';
	import { theme } from '$lib/stores/theme.svelte';
	import { setToken } from '$lib/api';
	import { onNavigate } from '$app/navigation';

	let { children, data } = $props();

	let sidenavOpen = $state(false);

	$effect(() => {
		theme.init('system');
	});

	$effect(() => {
		setToken(data.token ?? null);
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
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<meta name="description" content="JapaConnect — community platform for the Nigerian diaspora" />
</svelte:head>

<div class="flex min-h-screen flex-col">
	<Header
		user={data.user}
		notificationCount={data.notificationCount}
		onsidenavToggle={() => (sidenavOpen = !sidenavOpen)}
	/>

	<SideNav
		open={sidenavOpen}
		user={data.user}
		onclose={() => (sidenavOpen = false)}
	/>

	<main class="flex-1 pb-16 md:pb-0">
		{@render children()}
	</main>

	<Footer />
</div>

<MobileNav notificationCount={data.notificationCount} />
