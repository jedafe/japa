<script lang="ts">
	import Icon from '$lib/components/ui/Icon.svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { mainAppUrl } from '$lib/seo';
	import { t } from '$lib/i18n';
	import type { ToolMeta } from '$lib/tools/registry';

	/**
	 * SingleToolHeader — the minimal header for the single-tool shell mode
	 * (ADR-086 / §78). Shows the tool name + icon with tool branding, a theme
	 * toggle, and no cross-tool navigation. The footer carries the
	 * "Powered by JapaConnect" link.
	 */
	let { tool }: { tool: ToolMeta } = $props();
</script>

<header
	class="sticky top-0 z-50 h-[var(--header-height)] bg-surface/80 backdrop-blur-md border-b border-border"
	style="--tool-accent: {tool.accent}; --tool-accent-hover: {tool.accentHover};"
>
	<div class="mx-auto max-w-[var(--max-width)] h-full flex items-center justify-between px-4">
		<a href="/" class="flex items-center gap-2.5 font-semibold text-text">
			<span
				class="flex items-center justify-center w-8 h-8 rounded-lg text-primary-contrast"
				style="background-color: {tool.accent};"
			>
				<Icon name={tool.icon} size={18} />
			</span>
			<span class="text-base">{t(tool.nameKey, tool.name)}</span>
		</a>

		<div class="flex items-center gap-2">
			<button
				onclick={() => theme.toggle()}
				class="p-2 rounded-lg text-muted hover:text-text hover:bg-surface-subtle transition-colors"
				aria-label={t('shell.toggle_theme', 'Toggle theme')}
			>
				<Icon name={theme.resolved === 'dark' ? 'sun' : 'moon'} size={18} />
			</button>
		</div>
	</div>
</header>
