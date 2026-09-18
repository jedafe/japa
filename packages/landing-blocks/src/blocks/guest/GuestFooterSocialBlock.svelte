<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md
  // Single social icon inside `guest_footer`'s brand area.
  import type { GuestFooterSocialProps } from '../../types';
  import SocialIcon from './SocialIcon.svelte';

  interface Props {
    props: GuestFooterSocialProps;
    id?: string;
  }

  let { props, id }: Props = $props();
</script>

<a href={props.url ?? '#'} aria-label={props.label ?? props.icon ?? 'Social link'} target="_blank" rel="noopener noreferrer" {id}>
  <SocialIcon name={props.icon ?? ''} size={18} />
</a>
