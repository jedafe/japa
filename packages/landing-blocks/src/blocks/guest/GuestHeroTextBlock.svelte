<script lang="ts">
  import type { GuestHeroTextProps } from '../../../types';
  import type { FetchContext } from '../../../data/types';

  interface Props {
    props: GuestHeroTextProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id }: Props = $props();
</script>

<div data-block="guestherotext" class="rounded border border-dashed border-[var(--color-border)] p-4 text-sm text-[var(--color-muted)]">
  GuestHeroTextBlock placeholder
</div>
