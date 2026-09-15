<script lang="ts">
  import type { GuestNewsletterProps } from '../../../types';
  import type { FetchContext } from '../../../data/types';

  interface Props {
    props: GuestNewsletterProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id }: Props = $props();
</script>

<div data-block="guestnewsletter" class="rounded border border-dashed border-[var(--color-border)] p-4 text-sm text-[var(--color-muted)]">
  GuestNewsletterBlock placeholder
</div>
