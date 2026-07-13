<script lang="ts">
	import '../app.css';
	import { t } from '$lib/stores/i18n.svelte';
	import DocSidebar from '$lib/components/DocSidebar.svelte';
	import { page } from '$app/state';
	import { Menu, X, Key, Code } from 'lucide-svelte';

	let { children, data }: { children: any; data: { navTree?: any[]; user?: any; isAuth?: boolean; token?: string | null } } = $props();
	let sidebarOpen = $state(false);

	let audience = $derived.by(() => {
		const segs = page.url.pathname.split('/').filter(Boolean);
		return segs[0] || '';
	});

	type NavItem = { title: string; slug: string; audience: string; children?: NavItem[] };

	const AUDIENCE_LABELS: Record<string, string> = {
		users: 'Users',
		premium: 'Premium',
		organizations: 'Organizations',
		moderators: 'Moderators',
		api: 'API',
		admin: 'Admin',
		superadmin: 'Super Admin'
	};

	// The sidebar's top level is audience sections (/{audience}) with the
	// section's pages as children (/{audience}/{slug}).
	let flatNav: NavItem[] = $derived(
		(data.navTree ?? []).map((group: any) => ({
			title: AUDIENCE_LABELS[group.audience] ?? group.audience,
			slug: group.audience,
			audience: group.audience,
			children: (group.items ?? []).map((item: any) => ({
				title: item.title,
				slug: item.slug,
				audience: item.audience
			}))
		}))
	);

</script>

<div class="flex min-h-screen">
	<!-- Mobile menu button -->
	<button
		class="fixed left-4 top-4 z-50 rounded-md bg-white p-2 shadow-md md:hidden dark:bg-gray-800"
		onclick={() => (sidebarOpen = !sidebarOpen)}
	>
		{#if sidebarOpen}
			<X class="h-5 w-5" />
		{:else}
			<Menu class="h-5 w-5" />
		{/if}
	</button>

	<!-- Mobile overlay -->
	{#if sidebarOpen}
		<button
			class="fixed inset-0 z-30 bg-black/50 md:hidden"
			aria-label="Close sidebar"
			onclick={() => (sidebarOpen = false)}
		></button>
	{/if}

	<!-- Sidebar -->
	<div
		class="fixed inset-y-0 left-0 z-40 w-64 transition-transform md:static md:translate-x-0"
		class:translate-x-0={sidebarOpen}
		class:-translate-x-full={!sidebarOpen}
	>
		<DocSidebar audience={audience} navTree={flatNav} onNavigate={() => (sidebarOpen = false)} />
	</div>

	<div class="flex-1 overflow-auto">
		<!-- Header -->
		<header class="border-b px-4 py-4 md:px-8" style="border-color: var(--color-border)">
			<div class="flex items-center justify-between">
				<h1 class="text-sm font-semibold" style="color: var(--color-muted)">
					{t('docs.brand')}
				</h1>
				<div class="flex items-center gap-4 text-sm" style="color: var(--color-muted)">
					<a href="/api" class="flex items-center gap-1 hover:underline">
						<Code class="h-3 w-3" />
						API Reference
					</a>
					<a href="/" class="hover:underline">{t('nav.home')}</a>
					{#if data.isAuth}
						<a href="/api-keys" class="flex items-center gap-1 hover:underline">
							<Key class="h-3 w-3" />
							API Keys
						</a>
					{/if}
				</div>
			</div>
		</header>

		<!-- Main content -->
		<main class="px-4 py-8 md:px-8">
			{@render children()}
		</main>

		<!-- Footer -->
		<footer class="border-t px-4 py-4 text-center text-xs md:px-8" style="border-color: var(--color-border); color: var(--color-muted)">
			{t('footer.copyright')}
		</footer>
	</div>
</div>
