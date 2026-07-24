<script lang="ts">
	import { createSortable } from '@dnd-kit/svelte/sortable';
	import { GripVertical, Trash2, Copy, ChevronUp, ChevronDown, Settings } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';

	let { block, index, totalBlocks, selected, onSelect, onDelete, onDuplicate, onMoveUp, onMoveDown }: {
		block: { id: string; block_type: string; order: number; props: Record<string, unknown> };
		index: number; totalBlocks: number; selected: boolean;
		onSelect: () => void; onDelete: () => void; onDuplicate: () => void;
		onMoveUp: () => void; onMoveDown: () => void;
	} = $props();

	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const sortable = createSortable({ id: block.id, data: { source: 'canvas', blockType: block.block_type, index } } as any);
	const label = $derived(block.block_type.replace(/_/g, ' ').replace(/\b\w/g, (c: string) => c.toUpperCase()));
</script>

<div
	{@attach sortable.attach}
	class="group relative rounded-lg border transition-all"
	class:ring-2={selected} class:ring-primary={selected}
	class:opacity-40={sortable.isDragging} class:border-dashed={sortable.isDragging}
	style="border-color: {selected ? 'var(--color-primary)' : 'var(--color-border)'}; background-color: var(--color-background)"
	role="button" tabindex="0" onclick={onSelect} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') onSelect(); }}
>
	<div class="flex items-center gap-2 border-b px-3 py-2" style="border-color: var(--color-border)">
		<div {@attach sortable.attachHandle} class="cursor-grab touch-none p-0.5" style="color: var(--color-muted)"><GripVertical class="h-4 w-4" /></div>
		<span class="flex-1 text-xs font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{label}</span>
		<span class="rounded px-1.5 py-0.5 font-mono text-[10px]" style="background: var(--color-surface); color: var(--color-muted)">#{index + 1}</span>
		<div class="flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
			{#if index > 0}<Button variant="ghost" size="sm" onclick={(e: MouseEvent) => { e.stopPropagation(); onMoveUp(); }}><ChevronUp class="h-3 w-3" /></Button>{/if}
			{#if index < totalBlocks - 1}<Button variant="ghost" size="sm" onclick={(e: MouseEvent) => { e.stopPropagation(); onMoveDown(); }}><ChevronDown class="h-3 w-3" /></Button>{/if}
			<Button variant="ghost" size="sm" onclick={(e: MouseEvent) => { e.stopPropagation(); onSelect(); }}><Settings class="h-3 w-3" style="color: var(--color-primary)" /></Button>
			<Button variant="ghost" size="sm" onclick={(e: MouseEvent) => { e.stopPropagation(); onDuplicate(); }}><Copy class="h-3 w-3" /></Button>
			<Button variant="ghost" size="sm" onclick={(e: MouseEvent) => { e.stopPropagation(); onDelete(); }}><Trash2 class="h-3 w-3" style="color: var(--color-danger)" /></Button>
		</div>
	</div>
	<div class="p-4">{@render blockPreview()}</div>
</div>

{#snippet blockPreview()}
	{#if block.block_type === 'hero'}
		<div class="rounded-lg p-6 text-center text-white" style="background: linear-gradient(135deg, var(--color-primary), oklch(0.5 0.2 280))">
			<h3 class="text-lg font-bold">{block.props.title || 'Hero Title'}</h3>
			<p class="mt-1 text-sm opacity-80">{block.props.subtitle || 'Subtitle'}</p>
			{#if block.props.cta_text}<button class="mt-3 rounded-md bg-white px-4 py-1.5 text-sm font-medium" style="color: var(--color-primary)">{block.props.cta_text}</button>{/if}
		</div>
	{:else if block.block_type === 'rich_text'}
		<div class="prose prose-sm max-w-none rounded border p-3 text-sm" style="border-color: var(--color-border); color: var(--color-foreground)">{@html String(block.props.content || '<em style="color:var(--color-muted)">Rich text — click to edit</em>')}</div>
	{:else if block.block_type === 'image'}
		<div class="flex h-24 items-center justify-center rounded-lg border-2 border-dashed" style="border-color: var(--color-border); color: var(--color-muted)"><span class="text-sm">🖼 {Array.isArray(block.props.images) ? block.props.images.length : 0} image(s)</span></div>
	{:else if block.block_type === 'cta'}
		<div class="py-4 text-center"><button class="rounded-lg px-6 py-2 text-sm font-medium text-white" style="background-color: var(--color-primary)">{block.props.text || 'Call to Action'}</button></div>
	{:else if block.block_type === 'stats'}
		<div class="grid grid-cols-3 gap-3">
			{#each (Array.isArray(block.props.items) ? block.props.items : []).slice(0, 3) as item, i (i)}
				<div class="rounded-lg p-3 text-center" style="background: var(--color-surface)"><p class="text-lg font-bold" style="color: var(--color-primary)">{item?.value || '0'}</p><p class="text-xs" style="color: var(--color-muted)">{item?.label || 'Metric'}</p></div>
			{:else}<div class="col-span-3 py-3 text-center text-xs" style="color: var(--color-muted)">No stats</div>{/each}
		</div>
	{:else if block.block_type === 'faq'}
		<div class="space-y-2">
			{#if block.props.title}<p class="text-sm font-semibold">{block.props.title}</p>{/if}
			{#each (Array.isArray(block.props.questions) ? block.props.questions : []).slice(0, 2) as q, i (i)}<div class="rounded border px-3 py-2" style="border-color: var(--color-border)"><p class="text-xs font-medium">{q?.question || 'Question?'}</p></div>{:else}<p class="text-xs" style="color: var(--color-muted)">No FAQ items</p>{/each}
		</div>
	{:else if block.block_type === 'testimonial'}
		<div class="rounded-lg border-l-4 p-4 italic" style="border-color: var(--color-primary); background: var(--color-surface)"><p class="text-sm">"{block.props.quote || 'Quote'}"</p><p class="mt-2 text-xs font-medium not-italic" style="color: var(--color-muted)">— {block.props.author || 'Author'}</p></div>
	{:else if block.block_type === 'spacer'}
		<div class="flex items-center justify-center rounded border-2 border-dashed" style="height: {typeof block.props.height === 'number' ? block.props.height : 32}px; border-color: var(--color-border)"><span class="text-[10px]" style="color: var(--color-muted)">{block.props.height || 32}px</span></div>
	{:else if block.block_type === 'table'}
		<div class="overflow-x-auto rounded border text-xs" style="border-color: var(--color-border)"><table class="w-full">{#if Array.isArray(block.props.headers)}<thead><tr style="background: var(--color-surface)">{#each block.props.headers as h}<th class="px-2 py-1.5 text-left font-medium" style="color: var(--color-muted)">{h}</th>{/each}</tr></thead>{/if}<tbody>{#each (Array.isArray(block.props.rows) ? block.props.rows : []).slice(0, 3) as row, i (i)}<tr>{#each row as cell}<td class="px-2 py-1.5" style="border-color: var(--color-border)">{cell}</td>{/each}</tr>{:else}<tr><td class="px-2 py-3 text-center" style="color: var(--color-muted)">No rows</td></tr>{/each}</tbody></table></div>
	{:else if block.block_type === 'countdown'}
		<div class="rounded-lg p-4 text-center" style="background: var(--color-surface)">{#if block.props.title}<p class="text-sm font-semibold">{block.props.title}</p>{/if}<p class="mt-1 font-mono text-xs" style="color: var(--color-muted)">⏱ {block.props.target_datetime || 'not set'}</p></div>
	{:else if block.block_type === 'slider'}
		<div class="flex h-24 items-center justify-center rounded-lg border-2 border-dashed" style="border-color: var(--color-border); color: var(--color-muted)"><span class="text-sm">🎠 {Array.isArray(block.props.slides) ? block.props.slides.length : 0} slides</span></div>
	{:else if block.block_type === 'grid_row'}
		<div class="rounded-lg border-2 border-dashed p-3" style="border-color: var(--color-border)">
			<p class="mb-2 text-xs font-semibold" style="color: var(--color-muted)">Grid / Row — {typeof block.props.columns === 'number' ? block.props.columns : 2} columns</p>
			<div class="flex gap-2">
				{#if block.props.children && Array.isArray(block.props.children)}
					{#each block.props.children as col, ci}
						<div class="min-h-[40px] flex-1 rounded border p-2 text-center" style="border-color: var(--color-border); background: var(--color-surface)">
							<p class="text-[10px] font-medium" style="color: var(--color-muted)">Col {ci + 1}</p>
							{#if Array.isArray(col)}
								{#each col as child}
									<div class="mt-1 rounded px-2 py-1 text-[10px]" style="background: var(--color-background); color: var(--color-foreground); border: 1px solid var(--color-border)">
										{child.block_type?.replace(/_/g, ' ') ?? 'Block'}
									</div>
								{/each}
							{/if}
						</div>
					{/each}
				{:else}
					<div class="flex-1 rounded border p-2 text-center" style="border-color: var(--color-border); background: var(--color-surface)"><p class="text-[10px]" style="color: var(--color-muted)">Col 1</p></div>
					<div class="flex-1 rounded border p-2 text-center" style="border-color: var(--color-border); background: var(--color-surface)"><p class="text-[10px]" style="color: var(--color-muted)">Col 2</p></div>
				{/if}
			</div>
			{#if block.props.stack}
				<p class="mt-2 text-[10px]" style="color: var(--color-muted)">↕ Stacks on mobile</p>
			{/if}
		</div>
	{:else}
		<div class="rounded-lg p-3 text-center" style="background: var(--color-surface)"><p class="text-xs" style="color: var(--color-muted)">{block.block_type}</p></div>
	{/if}
{/snippet}