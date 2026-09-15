<script lang="ts">
  import type { GuestStoriesProps } from '../../../types';
  import type { FetchContext } from '../../../data/types';

  interface Props {
    props: GuestStoriesProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id }: Props = $props();
</script>

<div data-block="gueststories" class="rounded border border-dashed border-[var(--color-border)] p-4 text-sm text-[var(--color-muted)]">
  GuestStoriesBlock placeholder
</div>
