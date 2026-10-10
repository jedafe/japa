<script lang="ts">
	// Highlights / audience checklist — a heading plus a one- or
	// two-column list of check-marked lines (§114 2026-10-10: package
	// "Package Highlights" and mentorship "Who This Is For").
	import { CheckCircle2 } from 'lucide-svelte';

	interface Props {
		heading: string;
		lines: string[];
		columns?: 1 | 2;
	}
	let { heading, lines, columns = 1 }: Props = $props();
</script>

{#if lines.length > 0}
	<section aria-labelledby={`checklist-${heading.replace(/\W+/g, '-').toLowerCase()}`}>
		{#if heading}
			<h2
				id={`checklist-${heading.replace(/\W+/g, '-').toLowerCase()}`}
				class="text-xl font-bold text-[var(--color-text)]"
			>
				{heading}
			</h2>
		{/if}
		<ul
			class="mt-4 grid gap-x-10 gap-y-3 {columns === 2 ? 'sm:grid-cols-2' : ''}"
		>
			{#each lines as line, i (i)}
				<li class="flex items-start gap-2.5 text-sm text-[var(--color-text-subtle)]">
					<CheckCircle2
						size={16}
						class="mt-0.5 shrink-0 text-[var(--color-primary)]"
						aria-hidden="true"
					/>
					<span>{line}</span>
				</li>
			{/each}
		</ul>
	</section>
{/if}
