<script lang="ts">
  import type { SpacerDividerProps } from '../types';

  interface Props {
    props: SpacerDividerProps;
  }
  let { props }: Props = $props();
</script>

<section style="height: {props.height}px" class="flex items-center justify-center">
  {#if props.divider_style === 'line'}
    <hr class="border-t border-[var(--color-border)] w-full max-w-4xl mx-auto" style={props.divider_color ? `border-color: ${props.divider_color}` : ''} />
  {:else if props.divider_style === 'gradient'}
    <div class="w-full max-w-4xl mx-auto" style="height: 1px; background: linear-gradient(to right, transparent, {props.divider_color ?? 'var(--color-border)'}, transparent)"></div>
  {:else if props.divider_style === 'dots'}
    <div class="flex gap-2 text-[var(--color-muted)]" style={props.divider_color ? `color: ${props.divider_color}` : ''}>
      <span>&middot;</span>
      <span>&middot;</span>
      <span>&middot;</span>
    </div>
  {/if}
</section>
