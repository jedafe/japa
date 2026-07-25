<script lang="ts">
	import { X } from 'lucide-svelte';
	import { Button } from '$lib/components/ui/button';

	let { block, onUpdate, onClose }: {
		block: { id: string; block_type: string; props: Record<string, unknown> } | null;
		onUpdate: (id: string, props: Record<string, unknown>) => void;
		onClose: () => void;
	} = $props();

	function setProp(key: string, value: unknown) {
		if (!block) return;
		onUpdate(block.id, { ...block.props, [key]: value });
	}
	function gp(key: string): unknown { return block?.props?.[key] ?? ''; }
</script>

{#if block}
<div class="flex h-full flex-col">
	<div class="flex items-center justify-between border-b px-4 py-3" style="border-color: var(--color-border)">
		<h3 class="text-sm font-semibold" style="color: var(--color-foreground)">{block.block_type.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())}</h3>
		<Button variant="ghost" size="sm" onclick={onClose}><X class="h-4 w-4" /></Button>
	</div>
	<div class="flex-1 space-y-4 overflow-y-auto p-4">
		{#if block.block_type === 'hero'}
			{@render text('title','Title',gp('title'))}{@render text('subtitle','Subtitle',gp('subtitle'))}{@render text('cta_text','CTA Text',gp('cta_text'))}{@render text('cta_url','CTA URL',gp('cta_url'))}{@render num('overlay_opacity','Overlay Opacity',gp('overlay_opacity'),0,1,0.1)}
		{:else if block.block_type === 'rich_text'}
			{@render area('content','HTML Content',gp('content'),8)}
		{:else if block.block_type === 'image'}
			{@render pick('layout','Layout',gp('layout'),['single','grid','masonry','carousel'])}{@render text('caption','Caption',gp('caption'))}
		{:else if block.block_type === 'cta'}
			{@render text('text','Button Text',gp('text'))}{@render text('url','URL',gp('url'))}{@render pick('style','Style',gp('style'),['primary','secondary','outline'])}
		{:else if block.block_type === 'stats'}
			{@render pick('layout','Layout',gp('layout'),['horizontal','vertical','grid'])}
		{:else if block.block_type === 'faq'}
			{@render text('title','Section Title',gp('title'))}
		{:else if block.block_type === 'testimonial'}
			{@render area('quote','Quote',gp('quote'),4)}{@render text('author','Author',gp('author'))}{@render text('role','Role',gp('role'))}
		{:else if block.block_type === 'spacer'}
			{@render num('height','Height (px)',gp('height'),0,500,1)}{@render pick('style','Style',gp('style'),['space','line','dots'])}
		{:else if block.block_type === 'table'}
			{@render text('caption','Caption',gp('caption'))}{@render bool('responsive','Responsive',gp('responsive'))}
		{:else if block.block_type === 'countdown'}
			{@render text('title','Title',gp('title'))}{@render text('target_datetime','Target (ISO)',gp('target_datetime'))}{@render bool('show_days','Show Days',gp('show_days'))}{@render bool('show_hours','Show Hours',gp('show_hours'))}{@render bool('show_minutes','Show Minutes',gp('show_minutes'))}{@render bool('show_seconds','Show Seconds',gp('show_seconds'))}
		{:else if block.block_type === 'slider'}
			{@render num('autoplay_ms','Autoplay (ms)',gp('autoplay_ms'),0,30000,1000)}{@render bool('show_dots','Show Dots',gp('show_dots'))}{@render bool('show_arrows','Show Arrows',gp('show_arrows'))}
		{:else if block.block_type === 'comparison'}
			{@render text('comparison_id','Comparison ID',gp('comparison_id'))}
		{:else if block.block_type === 'form'}
			{@render text('form_id','Form ID',gp('form_id'))}{@render bool('show_title','Show Title',gp('show_title'))}
		{:else if block.block_type === 'table_of_contents'}
			{@render text('title','Title',gp('title'))}{@render bool('sticky','Sticky',gp('sticky'))}
		{:else if block.block_type === 'dynamic_listing'}
			{@render text('content_type','Content Type',gp('content_type'))}{@render num('limit','Limit',gp('limit'),1,100,1)}{@render pick('sort_order','Sort',gp('sort_order'),['asc','desc'])}
		{:else if block.block_type === 'grid_row'}
			{@render num('columns','Columns',gp('columns'),1,6,1)}
			{@render num('gap','Gap (px)',gp('gap'),0,64,4)}
			{@render pick('align','Vertical Align',gp('align'),['start','center','end','stretch'])}
			{@render bool('stack','Stack on Mobile',gp('stack'))}
		{:else}
			<p class="text-sm" style="color: var(--color-muted)">No config for {block.block_type}</p>
		{/if}
	</div>
</div>
{/if}

{#snippet text(key:string,label:string,value:unknown)}
<div><label for={`pb-${key}`} class="mb-1 block text-xs font-medium" style="color:var(--color-muted)">{label}</label><input id={`pb-${key}`} type="text" value={String(value??'')} oninput={(e)=>setProp(key,e.currentTarget.value)} class="w-full rounded-md border px-2.5 py-1.5 text-sm" style="border-color:var(--color-border);background:var(--color-background);color:var(--color-foreground)"/></div>{/snippet}
{#snippet area(key:string,label:string,value:unknown,rows:number)}
<div><label for={`pb-${key}`} class="mb-1 block text-xs font-medium" style="color:var(--color-muted)">{label}</label><textarea id={`pb-${key}`} {rows} oninput={(e)=>setProp(key,e.currentTarget.value)} class="w-full rounded-md border px-2.5 py-1.5 text-sm font-mono" style="border-color:var(--color-border);background:var(--color-background);color:var(--color-foreground)">{String(value??'')}</textarea></div>{/snippet}
{#snippet num(key:string,label:string,value:unknown,min:number,max:number,step:number)}
<div><label for={`pb-${key}`} class="mb-1 block text-xs font-medium" style="color:var(--color-muted)">{label}</label><input id={`pb-${key}`} type="number" value={Number(value??0)} {min} {max} {step} oninput={(e)=>setProp(key,Number(e.currentTarget.value))} class="w-full rounded-md border px-2.5 py-1.5 text-sm" style="border-color:var(--color-border);background:var(--color-background);color:var(--color-foreground)"/></div>{/snippet}
{#snippet pick(key:string,label:string,value:unknown,opts:string[])}
<div><label for={`pb-${key}`} class="mb-1 block text-xs font-medium" style="color:var(--color-muted)">{label}</label><select id={`pb-${key}`} onchange={(e)=>setProp(key,e.currentTarget.value)} class="w-full rounded-md border px-2.5 py-1.5 text-sm" style="border-color:var(--color-border);background:var(--color-background);color:var(--color-foreground)">{#each opts as o}<option value={o} selected={String(value)===o}>{o}</option>{/each}</select></div>{/snippet}
{#snippet bool(key:string,label:string,value:unknown)}
<label class="flex items-center gap-2 text-sm" style="color:var(--color-foreground)"><input type="checkbox" checked={Boolean(value)} onchange={(e)=>setProp(key,e.currentTarget.checked)} class="h-4 w-4 rounded"/>{label}</label>{/snippet}