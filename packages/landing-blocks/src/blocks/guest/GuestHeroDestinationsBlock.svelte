<script lang="ts">
  import type { GuestHeroDestinationsProps } from '../../../types';
  import type { FetchContext } from '../../../data/types';

  interface Props {
    props: GuestHeroDestinationsProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id }: Props = $props();
</script>

<div data-block="guestherodestinations" class="rounded border border-dashed border-[var(--color-border)] p-4 text-sm text-[var(--color-muted)]">
  GuestHeroDestinationsBlock placeholder
</div>
