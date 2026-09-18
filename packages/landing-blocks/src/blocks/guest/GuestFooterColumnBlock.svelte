<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md
  // Single link column inside `guest_footer` — a titled <ul> whose items are
  // `guest_footer_link` child blocks, editable in the page builder tree.
  import type { GuestFooterColumnProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import GuestChildren from './GuestChildren.svelte';

  interface Props {
    props: GuestFooterColumnProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id }: Props = $props();
</script>

<div class="footer-column" {id}>
  <h3 class="footer-column-title">{props.title ?? ''}</h3>
  <ul class="footer-column-list">
    <GuestChildren blocks={props.children ?? []} {fetchCtx} {geoCountry} tag="li" />
  </ul>
</div>
