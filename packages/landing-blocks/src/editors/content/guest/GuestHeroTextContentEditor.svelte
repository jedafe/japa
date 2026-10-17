<script lang="ts">
  import { siteName } from '../../../site';
  import type { GuestHeroTextProps, CTA, Block } from '../../../types';
  import CTAEditor from '../../CTAEditor.svelte';
  import NestedBlocksEditor from '../../NestedBlocksEditor.svelte';

  interface Props {
    props: GuestHeroTextProps;
    onchange: (v: GuestHeroTextProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestHeroTextProps>(key: K, value: GuestHeroTextProps[K]) {
    onchange({ ...props, [key]: value });
  }

  function addCta() {
    update('ctas', [...(props.ctas ?? []), { label: '', href: '', variant: 'primary' as const }]);
  }
  function removeCta(i: number) {
    update('ctas', (props.ctas ?? []).filter((_, idx) => idx !== i));
  }
  function updateCta(i: number, v: CTA) {
    const arr = [...(props.ctas ?? [])];
    arr[i] = v;
    update('ctas', arr);
  }

  function ctaToBlock(cta: CTA): Block {
    return {
      id: crypto.randomUUID(),
      type: 'guest_hero_cta_item',
      visible: true,
      props: {
        label: cta.label,
        href: cta.href,
        variant: cta.variant,
        icon: cta.icon,
      },
    } as Block;
  }

  function convertToNested() {
    const children = (props.ctas ?? []).map(ctaToBlock);
    onchange({ ...props, ctas: undefined, children });
  }

  const hasChildren = $derived((props.children?.length ?? 0) > 0);
</script>

<div class="space-y-4">
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Eyebrow</span><input class={inputClass} value={props.eyebrow ?? ''} oninput={(e) => update('eyebrow', e.currentTarget.value || undefined)} placeholder="Your Bridge to Global Opportunities" /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Headline prefix</span><input class={inputClass} value={props.headline_prefix ?? ''} oninput={(e) => update('headline_prefix', e.currentTarget.value || undefined)} placeholder="Work. Study. Thrive." /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Headline accent</span><input class={inputClass} value={props.headline_accent ?? ''} oninput={(e) => update('headline_accent', e.currentTarget.value || undefined)} placeholder="Anywhere in the World." /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Subhead</span><textarea class={inputClass} rows={3} value={props.subhead ?? ''} oninput={(e) => update('subhead', e.currentTarget.value || undefined)} placeholder="{siteName()} is your all-in-one platform..."></textarea></label>

  {#if hasChildren}
    <div class="space-y-2">
      <p class="text-xs text-[var(--color-muted)]">Each CTA is a nested block. Select one in the sidebar to edit its content and style.</p>
      <NestedBlocksEditor blocks={props.children ?? []} onchange={(children) => update('children', children)} allowedTypes={['guest_hero_cta_item']} />
    </div>
  {:else}
    <div class="space-y-2">
      <div class="text-xs font-medium text-[var(--color-muted)]">CTAs</div>
      {#each props.ctas ?? [] as cta, i (i)}
        <div class="space-y-1">
          <CTAEditor value={cta} onchange={(v) => updateCta(i, v)} />
          <button type="button" onclick={() => removeCta(i)} class="text-xs text-[var(--color-danger)]">Remove CTA</button>
        </div>
      {/each}
      <button type="button" onclick={addCta} class="rounded border border-dashed border-[var(--color-border)] px-2 py-1 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface)]">Add CTA</button>
      {#if (props.ctas?.length ?? 0) > 0}
        <button
          type="button"
          onclick={convertToNested}
          class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface)]"
        >
          Convert to block items
        </button>
      {/if}
    </div>
  {/if}
</div>
