<script lang="ts">
  import type { GuestFooterProps } from '../../../types';
  import GuestCardListEditor from './GuestCardListEditor.svelte';

  interface Props {
    props: GuestFooterProps;
    onchange: (v: GuestFooterProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass =
    'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestFooterProps>(key: K, value: GuestFooterProps[K]) {
    onchange({ ...props, [key]: value });
  }

  function defaultSocial() {
    return { icon: 'Globe', url: '', label: '' };
  }

  function defaultColumn() {
    return { id: crypto.randomUUID(), title: '', links: [{ label: '', url: '' }] };
  }
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<div class="space-y-4">
  <div class="grid grid-cols-2 gap-2">
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Light logo URL</span>
      <input
        class={inputClass}
        value={props.logo?.light ?? ''}
        oninput={(e) => update('logo', { ...props.logo, light: e.currentTarget.value || undefined })}
      />
    </label>
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Dark logo URL</span>
      <input
        class={inputClass}
        value={props.logo?.dark ?? ''}
        oninput={(e) => update('logo', { ...props.logo, dark: e.currentTarget.value || undefined })}
      />
    </label>
  </div>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Tagline</span>
    <input
      class={inputClass}
      value={props.tagline ?? ''}
      oninput={(e) => update('tagline', e.currentTarget.value || undefined)}
    />
  </label>

  <div class="space-y-2">
    <div class="text-xs font-medium text-[var(--color-muted)]">Social links</div>
    <GuestCardListEditor
      items={props.socials ?? []}
      onchange={(items) => update('socials', items)}
      add={defaultSocial}
      id="footer-socials"
    >
      {#snippet card(social: { icon: string; url: string; label: string }, updateSocial)}
        <div class="space-y-2">
          <input
            class={inputClass}
            value={social.icon}
            oninput={(e) => updateSocial({ ...social, icon: e.currentTarget.value })}
            placeholder="Icon"
          />
          <input
            class={inputClass}
            value={social.url}
            oninput={(e) => updateSocial({ ...social, url: e.currentTarget.value })}
            placeholder="URL"
          />
          <input
            class={inputClass}
            value={social.label}
            oninput={(e) => updateSocial({ ...social, label: e.currentTarget.value })}
            placeholder="Label"
          />
        </div>
      {/snippet}
    </GuestCardListEditor>
  </div>

  <div class="space-y-2">
    <div class="text-xs font-medium text-[var(--color-muted)]">Columns</div>
    <GuestCardListEditor
      items={props.columns ?? []}
      getId={(c) => c.id ?? c.title}
      onchange={(items) => update('columns', items)}
      add={defaultColumn}
      id="footer-columns"
    >
      {#snippet card(col, updateCol)}
        {@const links = col.links ?? []}
        <div class="space-y-2">
          <input
            class={inputClass}
            value={col.title}
            oninput={(e) => updateCol({ ...col, title: e.currentTarget.value })}
            placeholder="Column title"
          />
          <div class="space-y-1 border-l border-[var(--color-border)] pl-2">
            {#each links as link, li (li)}
              <div class="flex items-center gap-2">
                <input
                  class={inputClass}
                  value={link.label}
                  oninput={(e) => {
                    const next = [...links];
                    next[li] = { ...link, label: e.currentTarget.value };
                    updateCol({ ...col, links: next });
                  }}
                  placeholder="Label"
                />
                <input
                  class={inputClass}
                  value={link.url}
                  oninput={(e) => {
                    const next = [...links];
                    next[li] = { ...link, url: e.currentTarget.value };
                    updateCol({ ...col, links: next });
                  }}
                  placeholder="URL"
                />
                <button
                  type="button"
                  onclick={() => {
                    const next = links.filter((_, i) => i !== li);
                    updateCol({ ...col, links: next });
                  }}
                  class="text-[var(--color-danger)]"
                >
                  ×
                </button>
              </div>
            {/each}
            <button
              type="button"
              onclick={() => updateCol({ ...col, links: [...links, { label: '', url: '' }] })}
              class="text-xs text-[var(--color-muted)] hover:text-[var(--color-primary)]"
            >
              + Add link
            </button>
          </div>
        </div>
      {/snippet}
    </GuestCardListEditor>
  </div>
</div>
