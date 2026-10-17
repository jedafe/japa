<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { afterNavigate } from '$app/navigation';
	import { LayoutDashboard, Sparkles, Check, Settings, RefreshCw, List } from 'lucide-svelte';
	import { initSettingsDb, loadSettings, type StudioSettings } from '$lib/services/settings';
	import { initSiteSettings, siteName } from '$lib/stores/site.svelte';
	import { startWatcher, stopWatcher } from '$lib/services/watcher';

	let { children } = $props();

	let settings = $state<StudioSettings>(loadSettings());
	let watcherError = $state<string | null>(null);

	function reloadSettings() {
		settings = loadSettings();
	}

	onMount(async () => {
		await initSettingsDb();
		reloadSettings();
		initSiteSettings(settings.baseUrl);
	});

	afterNavigate(() => {
		reloadSettings();
	});

	$effect(() => {
		const projectRoot = settings.projectRoot;
		const autoSync = settings.autoSync;

		if (!projectRoot || !autoSync) {
			void stopWatcher();
			return;
		}

		watcherError = null;
		let cancelled = false;
		startWatcher(projectRoot).catch((e) => {
			if (!cancelled) {
				watcherError = e instanceof Error ? e.message : 'Failed to start file watcher.';
			}
		});

		return () => {
			cancelled = true;
			void stopWatcher();
		};
	});
</script>

<div class="flex h-screen w-screen overflow-hidden bg-background text-foreground">
	<aside class="w-60 border-r border-border bg-surface flex flex-col">
		<div class="p-4 border-b border-border">
			<h1 class="font-semibold text-lg">Japa Studio</h1>
			<p class="text-xs text-muted-foreground">Content desktop client</p>
		</div>
		<nav class="flex-1 p-2 space-y-1">
			<a href="/" class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm hover:bg-accent">
				<LayoutDashboard class="h-4 w-4" />
				<span>Queue</span>
			</a>
			<a href="/prompt" class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm hover:bg-accent">
				<Sparkles class="h-4 w-4" />
				<span>Prompt</span>
			</a>
			<a href="/batch" class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm hover:bg-accent">
				<List class="h-4 w-4" />
				<span>Batch</span>
			</a>
			<a href="/sync" class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm hover:bg-accent">
				<RefreshCw class="h-4 w-4" />
				<span>Sync Log</span>
			</a>
			<a href="/settings" class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm hover:bg-accent">
				<Settings class="h-4 w-4" />
				<span>Settings</span>
			</a>
		</nav>
		{#if settings.autoSync}
			<div class="border-t border-border p-3">
				<div class="flex items-center gap-2 text-xs text-success">
					<Check class="h-3 w-3" />
					<span>Auto-sync enabled</span>
				</div>
				{#if watcherError}
					<p class="mt-1 text-xs text-danger">{watcherError}</p>
				{/if}
			</div>
		{/if}
	</aside>
	<main class="flex-1 overflow-auto">
		{@render children?.()}
	</main>
</div>
