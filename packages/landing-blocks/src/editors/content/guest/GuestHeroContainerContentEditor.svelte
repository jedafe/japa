<script lang="ts">
  import type { GuestHeroContainerProps } from '../../../types';

  interface Props {
    props: GuestHeroContainerProps;
    onchange: (v: GuestHeroContainerProps) => void;
  }

  let { props, onchange }: Props = $props();

  function update<K extends keyof GuestHeroContainerProps>(key: K, value: GuestHeroContainerProps[K]) {
    onchange({ ...props, [key]: value });
  }
</script>

<div class="space-y-3 text-sm text-[var(--color-muted)]">
  <p>This container renders the hero text, image, stats, destinations, and community blocks. Reorder and edit those blocks in the block tree.</p>
  <p>Children: {props.children?.length ?? 0}</p>
</div>
