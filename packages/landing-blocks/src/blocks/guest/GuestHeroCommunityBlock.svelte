<script lang="ts">
  import type { GuestHeroCommunityProps, GuestHomeData } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { Users, ArrowRight } from 'lucide-svelte';
  import { getGuestHomeData } from '../../data/guestHomeContext';

  interface Props {
    props: GuestHeroCommunityProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props }: Props = $props();

  const guestHomeStore = getGuestHomeData();
  let guestHome = $state<GuestHomeData | undefined>(undefined);

  $effect(() => {
    if (!guestHomeStore) return;
    const unsub = guestHomeStore.subscribe((v) => (guestHome = v));
    return unsub;
  });

  const communityStat = $derived(guestHome?.stats.find((s) => s.label.toLowerCase().includes('community')));

  const value = $derived(props.value ?? communityStat?.value ?? '500K+');
  const label = $derived(props.label ?? communityStat?.label ?? 'members worldwide');
  const title = $derived(props.title ?? 'Join {value} {label}');
  const subtitle = $derived(props.subtitle ?? 'Your future starts with the right information.');
  const cta = $derived(props.cta);

  function fmt(template: string, vars: Record<string, string>): string {
    return template.replace(/\{(\w+)\}/g, (_, key) => vars[key] ?? '');
  }
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<div class="floating-card floating-card-community">
  <div class="community-avatars">
    {#each [1, 2, 3] as i (i)}
      <div class="community-avatar" style="z-index: {4 - i}; margin-left: {i === 1 ? 0 : -10}px">
        <Users size={16} />
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
