<script lang="ts">
  import type { GuestHeroCommunityProps, GuestHomeData, Block, GuestHeroCommunityMemberProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { ArrowRight } from 'lucide-svelte';
  import { getGuestHomeData } from '../../data/guestHomeContext';
  import { writable } from 'svelte/store';
  import { blockRegistry } from '../../render/blockRegistry';
  import { isBlockVisible, isBlockEmpty } from '../../render/blockVisibility';
  import BlockShell from '../../shell/BlockShell.svelte';
  import ScopedCss from '../../shell/ScopedCss.svelte';
  import { responsiveVisibilityCss } from '../../shell/styleToCss';

  interface Props {
    props: GuestHeroCommunityProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }

  let { props, fetchCtx, geoCountry, id }: Props = $props();

  const guestHomeStore = $derived(getGuestHomeData() ?? writable<GuestHomeData | undefined>(undefined));
  const guestHome = $derived($guestHomeStore);

  const title = $derived(props.title ?? 'Join {value} {label}');
  const subtitle = $derived(props.subtitle ?? 'Your future starts with the right information.');
  const cta = $derived(props.cta);
  const communityStat = $derived(guestHome?.stats.find((s) => s.label.toLowerCase().includes('community')));
  const value = $derived(props.value ?? communityStat?.value ?? '500K+');
  const label = $derived(props.label ?? communityStat?.label ?? 'members worldwide');

  // If children exist without an explicit source, assume manual so the seeded
  // blocks actually render. When there are no children, fall back to context.
  const dataSource = $derived(
    props.data_source ?? (props.children && props.children.length > 0 ? 'manual' : 'context')
  );

  // Context data list, capped by max_items.
  const contextItems = $derived(
    (guestHome?.stories ?? [])
      .map((s) => s.author_avatar_url)
      .filter((url): url is string => Boolean(url))
      .slice(0, props.max_items ?? 4)
  );

  // Manual children (data_source === 'manual') are rendered as-is.
  const manualChildren = $derived<Block[] | undefined>(
    dataSource === 'manual' ? props.children : undefined
  );

  // Context template: the first block in children is cloned once per context item,
  // with its props merged from the story data. If there is no template, fall
  // back to legacy rendering.
  const template = $derived<Block | undefined>(
    dataSource === 'context' && props.children && props.children.length > 0
      ? props.children[0]
      : undefined
  );

  function cloneTemplate(url: string, index: number): Block {
    const base = template as Block;
    const itemProps: GuestHeroCommunityMemberProps = {
      ...base.props,
      image_url: url,
      name: `Member ${index + 1}`,
    };
    return { ...base, id: `${base.id}-${index}`, props: itemProps } as Block;
  }

  const visibleItems = $derived<Block[] | undefined>(
    dataSource === 'context' && template
      ? contextItems.map((url, i) => cloneTemplate(url, i))
      : undefined
  );

  // Legacy data for non-template, non-manual path.
  const legacyItems = $derived<string[] | undefined>(
    manualChildren || visibleItems
      ? undefined
      : contextItems
  );

  function fmt(template: string, vars: Record<string, string>): string {
    return template.replace(/\{(\w+)\}/g, (_, key) => vars[key] ?? '');
  }

  // Per-device visibility rules for whichever child list renders.
  const renderedChildren = $derived(manualChildren ?? visibleItems);
  const visibilityCss = $derived(responsiveVisibilityCss(renderedChildren ?? []));
</script>

<ScopedCss css={visibilityCss} />

<div class="floating-card floating-card-community">
  <div class="community-avatars">
    {#if manualChildren}
      {#each manualChildren as block (block.id)}
        {#if isBlockVisible(block, geoCountry)}
          {@const meta = blockRegistry[block.type]}
          {#if meta}
            <BlockShell style={block.style} motion={block.motion} background={block.style?.background} hideIfEmpty={block.visibility_rules?.hide_if_empty} initialHasContent={!isBlockEmpty(block)} scopeKey={block.id}>
              {@const Component = meta.component}
              {#if block.type === 'custom'}
                <Component props={block.props} blockDefId={block.block_def_id} {fetchCtx} {geoCountry} id={block.id} />
              {:else}
                <Component props={block.props} {fetchCtx} {geoCountry} id={block.id} />
              {/if}
            </BlockShell>
          {/if}
        {/if}
      {/each}
    {:else if visibleItems}
      {#each visibleItems as block (block.id)}
        {#if isBlockVisible(block, geoCountry)}
          {@const meta = blockRegistry[block.type]}
          {#if meta}
            <BlockShell style={block.style} motion={block.motion} background={block.style?.background} hideIfEmpty={block.visibility_rules?.hide_if_empty} initialHasContent={!isBlockEmpty(block)} scopeKey={block.id}>
              {@const Component = meta.component}
              {#if block.type === 'custom'}
                <Component props={block.props} blockDefId={block.block_def_id} {fetchCtx} {geoCountry} id={block.id} />
              {:else}
                <Component props={block.props} {fetchCtx} {geoCountry} id={block.id} />
              {/if}
            </BlockShell>
          {/if}
        {/if}
      {/each}
    {:else}
      {#each legacyItems ?? [] as url, i (url + i)}
        <div class="community-avatar" style="z-index: {4 - i}; margin-left: {i === 0 ? 0 : '-10px'}">
          <img src={url} alt="" class="community-avatar-img" />
        </div>
      {/each}
    {/if}
  </div>
  <div class="community-body">
    <h3 class="community-title">{fmt(title, { value, label })}</h3>
    <p class="community-subtitle">{subtitle}</p>
  </div>
  <a href={cta?.href ?? '/explore'} class="community-cta" aria-label={cta?.label ?? 'Join the community'}>
    <ArrowRight size={18} />
  </a>
</div>
