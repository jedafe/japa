<script lang="ts">
	import { Save, Eye, Send, RotateCcw } from 'lucide-svelte';
	import { page } from '$app/state';

	const rawPath = $derived(page.params.path);
</script>

<div class="flex h-full flex-col p-6">
	<header class="mb-6 flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold">Editor</h1>
			<p class="text-sm text-muted-foreground">{rawPath || 'New file'}</p>
		</div>
		<div class="flex gap-2">
			<button class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent">
				<Save class="h-4 w-4" />
				<span>Save locally</span>
			</button>
			<button class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent">
				<Eye class="h-4 w-4" />
				<span>Preview</span>
			</button>
			<button class="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-contrast hover:bg-primary-hover">
				<Send class="h-4 w-4" />
				<span>Publish</span>
			</button>
			<button class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent">
				<RotateCcw class="h-4 w-4" />
				<span>Revert</span>
			</button>
		</div>
	</header>

	<div class="grid h-full gap-6 lg:grid-cols-2">
		<div class="rounded-xl border border-border bg-card p-4">
			<h2 class="mb-2 text-sm font-medium">Markdown Source</h2>
			<textarea class="h-[60vh] w-full resize-none rounded-lg border border-border bg-surface p-4 font-mono text-sm" placeholder="# Title&#10;&#10;Start writing..."></textarea>
		</div>
		<div class="rounded-xl border border-border bg-card p-4">
			<h2 class="mb-2 text-sm font-medium">Preview</h2>
			<div class="h-[60vh] w-full overflow-auto rounded-lg bg-surface p-4 text-sm text-muted-foreground">
				<p>Rendered preview will appear here after Phase 03.07.</p>
			</div>
		</div>
	</div>
</div>
