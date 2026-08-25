<script lang="ts">
	import { Sparkles, Copy, Save } from 'lucide-svelte';
</script>

<div class="flex h-full flex-col p-6">
	<header class="mb-6">
		<h1 class="text-2xl font-bold">Prompt Builder</h1>
		<p class="text-sm text-muted-foreground">Build a prompt for Claude Code to generate JapaConnect content.</p>
	</header>

	<div class="grid gap-6 lg:grid-cols-2">
		<div class="space-y-4 rounded-xl border border-border bg-card p-6">
			<label class="block text-sm font-medium">
				Topic / Title
				<input type="text" class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm" placeholder="e.g. Cost of Living in Toronto vs Lagos" />
			</label>
			<label class="block text-sm font-medium">
				Content Type
				<select class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm">
					<option value="blog_post">Blog Post</option>
					<option value="guide">Guide</option>
					<option value="comparison">Comparison</option>
					<option value="page">Page</option>
				</select>
			</label>
			<label class="block text-sm font-medium">
				Persona
				<select class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm">
					<option>Ada the Relocation Coach</option>
				</select>
			</label>
			<div class="grid grid-cols-2 gap-4">
				<label class="block text-sm font-medium">
					Tone
					<select class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm">
						<option>friendly</option>
						<option>professional</option>
						<option>travel blogger</option>
					</select>
				</label>
				<label class="block text-sm font-medium">
					Framework
					<select class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm">
						<option value="">none</option>
						<option value="aida">AIDA</option>
						<option value="pas">PAS</option>
					</select>
				</label>
			</div>
			<div class="grid grid-cols-3 gap-4">
				<label class="block text-sm font-medium">
					Min words
					<input type="number" value="800" class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm" />
				</label>
				<label class="block text-sm font-medium">
					Headings
					<input type="number" value="5" class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm" />
				</label>
				<label class="block text-sm font-medium">
					External links
					<input type="number" value="2" class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm" />
				</label>
			</div>
			<div class="flex gap-2 pt-2">
				<button class="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-contrast hover:bg-primary-hover">
					<Sparkles class="h-4 w-4" />
					<span>Generate Prompt</span>
				</button>
				<button class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent">
					<Copy class="h-4 w-4" />
					<span>Copy</span>
				</button>
				<button class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent">
					<Save class="h-4 w-4" />
					<span>Save to CLAUDE.md</span>
				</button>
			</div>
		</div>

		<div class="rounded-xl border border-border bg-card p-6">
			<h2 class="mb-2 font-medium">Generated Prompt</h2>
			<div class="h-96 overflow-auto rounded-lg bg-surface p-4 text-sm text-muted-foreground">
				<p>Prompt preview will appear here after Phase 02.02.</p>
			</div>
		</div>
	</div>
</div>
