<script lang="ts">
	import { setUser, setPermissions, clearUser } from '$lib/stores/auth';
	import Sidebar from '$lib/components/Sidebar.svelte';
	import TopBar from '$lib/components/layout/TopBar.svelte';
	import '../app.css';
	import { page } from '$app/state';

	let { children, data } = $props();

	$effect(() => {
		if (data.user) {
			setUser(data.user);
			setPermissions(data.user.permissions ?? []);
		} else {
			clearUser();
		}
	});
</script>

<svelte:head>
	<title>JapaConnect Admin</title>
</svelte:head>

<div class="flex h-screen overflow-hidden bg-gray-50 dark:bg-gray-900">
	<Sidebar currentPath={page.url.pathname} />

	<div class="flex flex-1 flex-col overflow-hidden">
		<TopBar />
		<main class="flex-1 overflow-y-auto">
			<div class="mx-auto max-w-7xl px-4 py-6 sm:px-6 md:px-8">
				{@render children()}
			</div>
		</main>
	</div>
</div>
