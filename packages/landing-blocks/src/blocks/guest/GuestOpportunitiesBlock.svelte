<script lang="ts">
  import type { GuestOpportunitiesProps } from '../../../types';
  import type { FetchContext } from '../../../data/types';

  interface Props {
    props: GuestOpportunitiesProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id }: Props = $props();
</script>

<div data-block="guestopportunities" class="rounded border border-dashed border-[var(--color-border)] p-4 text-sm text-[var(--color-muted)]">
  GuestOpportunitiesBlock placeholder
</div>
