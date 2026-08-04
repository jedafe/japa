<script lang="ts">
  import type { AdvertisementProps } from '../types';

  interface Props {
    props: AdvertisementProps;
  }
  let { props }: Props = $props();
</script>

<section class="px-6 py-4">
  <div data-ad-slot={props.slot_key} class="ad-container flex min-h-[90px] items-center justify-center rounded-[var(--radius)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface-subtle)] text-sm text-[var(--color-muted)]">
    Advertisement
  </div>
</section>
