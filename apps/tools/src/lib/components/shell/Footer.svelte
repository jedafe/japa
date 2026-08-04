<script lang="ts">
	import { mainAppUrl } from '$lib/seo';
	import type { ToolMeta } from '$lib/tools/registry';

	/**
	 * Footer — two variants based on shell mode.
	 * Hub shell: full footer with links to all tools + main app.
	 * Single-tool shell: minimal "Powered by JapaConnect" footer (ADR-086 / §78).
	 */
	let {
		mode = 'hub',
		tool = undefined
	}: {
		mode?: 'hub' | 'single';
		tool?: ToolMeta | undefined;
	} = $props();
</script>

{#if mode === 'single'}
	<footer class="py-4 px-4 text-center border-t border-border bg-surface">
		<a
			href={mainAppUrl('/')}
			class="text-sm text-muted hover:text-text transition-colors"
		>
			Powered by JapaConnect
		</a>
	</footer>
{:else}
	<footer class="py-10 px-4 border-t border-border bg-surface mt-12">
		<div class="mx-auto max-w-[var(--max-width)] flex flex-col sm:flex-row items-center justify-between gap-4">
			<div class="flex items-center gap-2 text-sm text-muted">
				<span
					class="flex items-center justify-center w-6 h-6 rounded bg-primary text-primary-contrast text-xs font-bold"
				>
					J
				</span>
				<span>JapaConnect Tools</span>
			</div>
			<div class="flex items-center gap-4 text-sm text-muted">
				<a href={mainAppUrl('/')} class="hover:text-text transition-colors">JapaConnect</a>
				<a href={mainAppUrl('/docs')} class="hover:text-text transition-colors">Docs</a>
				<a href={mainAppUrl('/learn')} class="hover:text-text transition-colors">Learn</a>
			</div>
		</div>
	</footer>
{/if}
