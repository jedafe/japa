<script lang="ts">
  import type { GuestHeroImageProps } from '../../../types';
  import type { FetchContext } from '../../../data/types';

  interface Props {
    props: GuestHeroImageProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id }: Props = $props();
</script>

<div data-block="guestheroimage" class="rounded border border-dashed border-[var(--color-border)] p-4 text-sm text-[var(--color-muted)]">
  GuestHeroImageBlock placeholder
</div>
