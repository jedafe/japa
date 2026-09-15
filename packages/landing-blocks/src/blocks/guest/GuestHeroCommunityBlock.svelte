<script lang="ts">
  import type { GuestHeroCommunityProps } from '../../../types';
  import type { FetchContext } from '../../../data/types';

  interface Props {
    props: GuestHeroCommunityProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id }: Props = $props();
</script>

<div data-block="guestherocommunity" class="rounded border border-dashed border-[var(--color-border)] p-4 text-sm text-[var(--color-muted)]">
  GuestHeroCommunityBlock placeholder
</div>
