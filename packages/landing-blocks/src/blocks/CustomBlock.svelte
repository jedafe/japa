<script lang="ts">
	// CustomBlock.svelte — schema-driven renderer for DB-defined custom blocks.
	// Renders the block's fields inside a BlockShell, using the config_schema
	// to determine how each field should be displayed (heading, paragraph,
	// image, link, etc.).
	import BlockShell from '../shell/BlockShell.svelte';
	import type { BlockStyle, MotionConfig, BackgroundConfig, CustomBlockProps, ConfigSchemaField } from '../types';
	import { getCustomBlockDef, resolveCustomBlockProps } from '../render/runtimeBlockRegistry';

	let {
		props,
		blockDefId,
		style,
		motion,
		background,
	}: {
		props: CustomBlockProps;
		blockDefId: string;
		style?: BlockStyle;
		motion?: MotionConfig;
		background?: BackgroundConfig;
	} = $props();

	const def = $derived(getCustomBlockDef(blockDefId));
	const schema = $derived(def?.config_schema?.fields ?? []);
	const resolvedProps = $derived(resolveCustomBlockProps(blockDefId, props));

	function value(field: ConfigSchemaField): unknown {
		return resolvedProps[field.key];
	}

	function asString(v: unknown): string {
		return typeof v === 'string' ? v : '';
	}

	function asUrl(v: unknown): string {
		return typeof v === 'string' ? v : '';
	}
</script>

<BlockShell {style} {motion} {background}>
	<div class="custom-block mx-auto max-w-4xl px-4 py-6">
		{#if !def}
			<!-- Block definition not found (possibly deleted or not loaded) -->
			<div class="rounded border border-dashed p-4 text-sm text-[var(--color-muted)]">
				Custom block definition not found (ID: {blockDefId}). It may have been deleted.
			</div>
		{:else}
			{#each schema as field}
				{@const v = value(field)}
				{#if field.type === 'text'}
					{#if asString(v)}
						<p class="text-base text-[var(--color-text)]">{asString(v)}</p>
					{/if}
				{:else if field.type === 'textarea'}
					{#if asString(v)}
						<p class="whitespace-pre-line text-sm text-[var(--color-text)]">{asString(v)}</p>
					{/if}
				{:else if field.type === 'richtext'}
					<!-- richtext rendered as raw HTML — admin sanitizes on save -->
					{#if asString(v)}
						<div class="prose prose-sm max-w-none text-[var(--color-text)]">{@html asString(v)}</div>
					{/if}
				{:else if field.type === 'number'}
					{#if typeof v === 'number'}
						<p class="text-2xl font-bold text-[var(--color-text)]">{v}</p>
					{/if}
				{:else if field.type === 'color'}
					{#if asString(v)}
						<div class="inline-flex items-center gap-2">
							<span class="inline-block h-4 w-4 rounded-full border" style="background-color: {asString(v)}; border-color: var(--color-border)"></span>
							<span class="text-sm text-[var(--color-text)]">{asString(v)}</span>
						</div>
					{/if}
				{:else if field.type === 'select'}
					{#if asString(v)}
						<p class="text-sm text-[var(--color-text)]">{field.label}: {asString(v)}</p>
					{/if}
				{:else if field.type === 'checkbox'}
					<p class="text-sm text-[var(--color-text)]">{field.label}: {v ? 'Yes' : 'No'}</p>
				{:else if field.type === 'url'}
					{#if asUrl(v)}
						<a href={asUrl(v)} class="text-sm text-[var(--color-primary)] underline" target="_blank" rel="noopener noreferrer">{field.label}</a>
					{/if}
				{:else if field.type === 'media'}
					{#if v && typeof v === 'object' && 'url' in (v as Record<string, unknown>)}
						{@const media = v as { url: string; alt?: string }}
						<img src={media.url} alt={media.alt ?? ''} class="max-h-96 w-full rounded-lg object-cover" loading="lazy" />
					{/if}
				{/if}
			{/each}
		{/if}
	</div>
</BlockShell>
