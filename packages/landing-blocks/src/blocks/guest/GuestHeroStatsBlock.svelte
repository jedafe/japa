<script lang="ts">
  import type { GuestHeroStatsProps } from '../../../types';
  import type { FetchContext } from '../../../data/types';

  interface Props {
    props: GuestHeroStatsProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id }: Props = $props();
</script>

<div data-block="guestherostats" class="rounded border border-dashed border-[var(--color-border)] p-4 text-sm text-[var(--color-muted)]">
  GuestHeroStatsBlock placeholder
</div>
