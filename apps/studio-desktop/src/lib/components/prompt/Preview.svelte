<script lang="ts">
	import { Info } from 'lucide-svelte';

	interface Props {
		value: string;
	}

	let { value }: Props = $props();
</script>

<div class="rounded-xl border border-border bg-card p-6 space-y-4">
	<h2 class="font-medium">Generated Prompt</h2>
	<textarea
		readonly
		class="h-[28rem] w-full resize-none rounded-lg border border-border bg-surface p-4 text-sm text-foreground focus:outline-none"
		{value}
		placeholder="Generated prompt will appear here."
	></textarea>
	<div class="flex items-start gap-2 rounded-lg bg-surface p-3 text-sm text-muted-foreground">
		<Info class="mt-0.5 h-4 w-4 shrink-0" />
		<p>
			Copy the prompt above and paste it into Claude Code, or save it to
			<code class="rounded bg-muted px-1">.claude/CLAUDE.md</code>
			to make it the current project instruction.
		</p>
	</div>
</div>
