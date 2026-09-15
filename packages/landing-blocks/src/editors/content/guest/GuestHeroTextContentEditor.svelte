<script lang="ts">
  import type { GuestHeroTextProps, CTA } from '../../../types';
  import CTAEditor from '../../CTAEditor.svelte';

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
</script>

<div class="space-y-4">
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Eyebrow</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.eyebrow ?? ''} oninput={(e) => update('eyebrow', e.currentTarget.value || undefined)} placeholder="Your Bridge to Global Opportunities" /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Headline prefix</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.headline_prefix ?? ''} oninput={(e) => update('headline_prefix', e.currentTarget.value || undefined)} placeholder="Work. Study. Live." /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Headline accent</span><input class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' value={props.headline_accent ?? ''} oninput={(e) => update('headline_accent', e.currentTarget.value || undefined)} placeholder="Anywhere in the World." /></label>
  <label class="block"><span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Subhead</span><textarea class='w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]' rows={3} value={props.subhead ?? ''} oninput={(e) => update('subhead', e.currentTarget.value || undefined)} placeholder="JapaConnect is your all-in-one platform..."></textarea></label>

  <div class="space-y-2">
    <div class="text-xs font-medium text-[var(--color-muted)]">CTAs</div>
    {#each props.ctas ?? [] as cta, i (i)}
      <div class="space-y-1">
        <CTAEditor value={cta} onchange={(v) => updateCta(i, v)} />
        <button type="button" onclick={() => removeCta(i)} class="text-xs text-[var(--color-danger)]">Remove CTA</button>
      </div>
    {/each}
    <button type="button" onclick={addCta} class="rounded border border-dashed border-[var(--color-border)] px-2 py-1 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface)]">Add CTA</button>
  </div>
</div>
