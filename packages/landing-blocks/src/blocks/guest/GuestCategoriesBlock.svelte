<script lang="ts">
  import type { GuestCategoriesProps } from '../../../types';
  import type { FetchContext } from '../../../data/types';

  interface Props {
    props: GuestCategoriesProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id }: Props = $props();
</script>

<div data-block="guestcategories" class="rounded border border-dashed border-[var(--color-border)] p-4 text-sm text-[var(--color-muted)]">
  GuestCategoriesBlock placeholder
</div>
