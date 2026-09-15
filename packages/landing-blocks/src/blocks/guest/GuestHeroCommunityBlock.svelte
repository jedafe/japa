<script lang="ts">
  import type { GuestHeroCommunityProps, GuestHomeData } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { Users, ArrowRight } from 'lucide-svelte';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import { writable } from 'svelte/store';

  interface Props {
    props: GuestHeroCommunityProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props }: Props = $props();

  const guestHomeStore = $derived(getGuestHomeData() ?? writable<GuestHomeData | undefined>(undefined));
  const guestHome = $derived($guestHomeStore);

  const communityStat = $derived(guestHome?.stats.find((s) => s.label.toLowerCase().includes('community')));

  const value = $derived(props.value ?? communityStat?.value ?? '500K+');
  const label = $derived(props.label ?? communityStat?.label ?? 'members worldwide');
  const title = $derived(props.title ?? 'Join {value} {label}');
  const subtitle = $derived(props.subtitle ?? 'Your future starts with the right information.');
  const cta = $derived(props.cta);

  const avatars = $derived(
    (guestHome?.stories ?? [])
      .map((s) => s.author_avatar_url)
      .filter(Boolean)
      .slice(0, 3) as string[]
  );

  function fmt(template: string, vars: Record<string, string>): string {
    return template.replace(/\{(\w+)\}/g, (_, key) => vars[key] ?? '');
  }
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<div class="floating-card floating-card-community">
  <div class="community-avatars">
    {#each [0, 1, 2] as i (i)}
      {@const url = avatars[i]}
      <div class="community-avatar" style="z-index: {3 - i}; margin-left: {i === 0 ? 0 : '-10px'}">
        {#if url}
          <img src={url} alt="" class="community-avatar-img" />
        {:else}
          <Users size={16} />
        {/if}
      </div>
    {/each}
  </div>
  <div class="community-body">
    <h3 class="community-title">{fmt(title, { value, label })}</h3>
    <p class="community-subtitle">{subtitle}</p>
  </div>
  <a href={cta?.href ?? '/explore'} class="community-cta" aria-label={cta?.label ?? 'Join the community'}>
    <ArrowRight size={18} />
  </a>
</div>
