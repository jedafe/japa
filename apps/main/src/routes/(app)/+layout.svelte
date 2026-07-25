<script lang="ts">
	import { untrack, type Snippet } from 'svelte';
	import { page } from '$app/state';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { isExceptionRoute } from '$lib/components/create/gate';
	import { api } from '$lib/api/client';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();

	// ADR-058 Exception #2: /create collapses the nav and hides the right sidebar.
	const exception = $derived(isExceptionRoute(page.route.id));

	// Seed from the server value once; thereafter this is user-controlled.
	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		const next = !collapsed;
		collapsed = next; // optimistic
		if (!data.user) return; // guests have nowhere to persist; keep it session-local
		const res = await api.patch('/me/account', { ui: { nav_collapsed: next } });
		if (res.error) collapsed = !next; // rollback on failure
	}
</script>

<AppShell
	menu={data.menu}
	user={data.user}
	collapsed={exception || collapsed}
	onToggleCollapse={toggleCollapse}
	trendingTags={data.trendingTags}
	suggestedSpaces={data.suggestedSpaces}
	hideRightSidebar={exception}
>
	{@render children()}
</AppShell>
