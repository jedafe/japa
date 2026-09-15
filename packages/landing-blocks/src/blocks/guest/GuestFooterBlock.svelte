<script lang="ts">
  import type { GuestFooterProps } from '../../../types';
  import type { FetchContext } from '../../../data/types';

  interface Props {
    props: GuestFooterProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id }: Props = $props();
</script>

<div data-block="guestfooter" class="rounded border border-dashed border-[var(--color-border)] p-4 text-sm text-[var(--color-muted)]">
  GuestFooterBlock placeholder
</div>
