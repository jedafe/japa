<script lang="ts">
	import Icon from '$lib/components/ui/Icon.svelte';
	import { t } from '$lib/i18n';
	import type { ToolMeta } from '$lib/tools/registry';

	/**
	 * ToolCard — a card on the tools hub grid. Displays the tool icon, name,
	 * tagline, and a CTA link to the tool's route.
	 * Ref: §78, ROADMAP-TOOLS 02.01.
	 */
	let { tool }: { tool: ToolMeta } = $props();
</script>

<a
	href={tool.route}
	class="group flex flex-col gap-3 p-6 bg-surface border border-border rounded-[var(--radius-card)] hover:border-[var(--tool-accent)] transition-all duration-200 hover:shadow-md"
	style="--tool-accent: {tool.accent};"
>
	<div
		class="flex items-center justify-center w-11 h-11 rounded-lg text-primary-contrast"
		style="background-color: {tool.accent};"
	>
		<Icon name={tool.icon} size={22} />
	</div>
	<div class="flex-1">
		<h3 class="text-base font-semibold text-text group-hover:text-[var(--tool-accent)] transition-colors">
			{t(tool.nameKey, tool.name)}
		</h3>
		<p class="mt-1 text-sm text-text-subtle">{t(tool.taglineKey, tool.tagline)}</p>
	</div>
	<div class="flex items-center gap-1 text-sm font-medium text-[var(--tool-accent)]">
		{t('hub.tool.open', 'Open tool')}
		<Icon name="arrow-right" size={16} />
	</div>
</a>
