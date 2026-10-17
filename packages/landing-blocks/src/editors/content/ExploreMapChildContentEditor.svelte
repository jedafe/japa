<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
  // Shared content editor for all explore_map_* children — they carry no
  // config of their own; the parent World Explorer block owns everything.
  import type { ExploreMapChildProps } from '../../types';

  interface Props {
    props: ExploreMapChildProps;
    onchange: (v: ExploreMapChildProps) => void;
  }
  let { props }: Props = $props();
</script>

<p class="text-xs text-[var(--color-muted)]">
  This section is part of the World Explorer block. Its content and data are
  configured on the parent block — select World Explorer in the sidebar to
  edit them. Use the Style, Motion, and Visibility tabs to restyle or hide
  this section.
</p>
