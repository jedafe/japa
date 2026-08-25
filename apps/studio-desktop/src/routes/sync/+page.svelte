<script lang="ts">
	import { RefreshCw } from 'lucide-svelte';
</script>

<div class="flex h-full flex-col p-6">
	<header class="mb-6 flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold">Sync Log</h1>
			<p class="text-sm text-muted-foreground">History of publish, update, and failed sync attempts.</p>
		</div>
		<button class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent">
			<RefreshCw class="h-4 w-4" />
			<span>Refresh</span>
		</button>
	</header>

	<div class="rounded-xl border border-border bg-card p-8 text-center text-muted-foreground">
		<p>Sync log will appear here after the first publish or update.</p>
	</div>
</div>
