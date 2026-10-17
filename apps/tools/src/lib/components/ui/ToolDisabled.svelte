<script lang="ts">
	import Icon from './Icon.svelte';
	import { mainAppUrl } from '$lib/seo';
	import { t } from '$lib/i18n';
	import type { ToolMeta } from '$lib/tools/registry';

	/**
	 * ToolDisabled — shown when a tool's `system_settings` enabled toggle
	 * (e.g. `checklist.enabled`) is false. The tool scaffold metadata (name,
	 * icon, accent) is still shown so the user recognises which tool is off,
	 * but no backend calls are made for the tool's domain data.
	 *
	 * Ref: §78 (Tools Web Surface), conventions/system-settings-catalog.md
	 *      ({tool}.enabled keys), Rule 3.
	 */
	let { tool }: { tool: ToolMeta } = $props();
</script>

<div class="mx-auto max-w-2xl px-4 py-16 text-center">
	<div class="flex justify-center mb-6">
		<div
			class="flex items-center justify-center w-14 h-14 rounded-xl text-primary-contrast opacity-40"
			style="background-color: {tool.accent};"
		>
			<Icon name={tool.icon} size={28} />
		</div>
	</div>
	<h1 class="text-2xl font-bold text-text">{t(tool.nameKey, tool.name)}</h1>
	<p class="mt-3 text-text-subtle max-w-md mx-auto">
		{t('tool.disabled_desc', 'This tool is currently disabled. Please check back later.')}
	</p>
	<a
		href={mainAppUrl('/')}
		class="mt-6 inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-primary text-primary-contrast text-sm font-medium hover:bg-primary-hover transition-colors"
	>
		<Icon name="arrow-left" size={16} />
		{t('shell.back_to_japaconnect', 'Back to {site}')}
	</a>
</div>
