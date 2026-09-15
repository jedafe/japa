<script lang="ts">
  import type { GuestToolsProps } from '../../../types';
  import type { FetchContext } from '../../../data/types';

  interface Props {
    props: GuestToolsProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id }: Props = $props();
</script>

<div data-block="guesttools" class="rounded border border-dashed border-[var(--color-border)] p-4 text-sm text-[var(--color-muted)]">
  GuestToolsBlock placeholder
</div>
