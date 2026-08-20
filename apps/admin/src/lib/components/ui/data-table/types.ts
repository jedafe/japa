import type { Component, ComponentType, Snippet, SvelteComponent } from 'svelte';

export type SortDir = 'asc' | 'desc';

export interface Column {
  key: string;
  label: string;
  sortable?: boolean;
  class?: string;
  render?: (value: any, row: any) => string;
  component?: Component<any>;
  componentSnippet?: Snippet<[{ value: any; row: any; [k: string]: any }]>;
  componentProps?: Record<string, any>;
  /** Set to false to disable the resize handle on this column. Default: true. */
  resizable?: boolean;
  /** Set to false to exclude this column from the hide/unhide toggle. Default: true. */
  hideable?: boolean;
  /** Initial column width in pixels. */
  width?: number;
  /** Hide this column by default (user can toggle it on). */
  defaultHidden?: boolean;
}

export interface ActionContext {
  openView: (row: any) => void;
  openEdit: (row: any) => void;
  openDelete: (row: any) => void;
  reload: () => void;
}

export interface ActionItem {
  label: string;
  icon?: ComponentType<SvelteComponent>;
  onclick: (row: any, ctx?: ActionContext) => void;
  variant?: 'default' | 'destructive';
  show?: (row: any) => boolean;
}

export interface FilterDef {
  key: string;
  label: string;
  options?: Array<{ value: string; label: string }>;
  /** Use 'autocomplete' to render the country/space live-debounced Autocomplete instead of a Select. */
  type?: 'select' | 'autocomplete';
  placeholder?: string;
  emptyMessage?: string;
  /** Required when type is 'autocomplete'. */
  fetcher?: (q: string) => Promise<{ id: string; label: string; sublabel?: string }[]>;
  /** Optional initial suggestions for autocomplete. */
  initialFetcher?: () => Promise<{ id: string; label: string; sublabel?: string }[]>;
  /** Resolves the selected id to a display label for autocomplete. */
  getSelectedLabel?: (id: string) => string;
}

export interface BulkAction {
  label: string;
  icon?: ComponentType<SvelteComponent>;
  onclick: (ids: Set<string>) => void | Promise<void>;
  variant?: 'default' | 'destructive';
  disabled?: (ids: Set<string>, selectedRows?: any[]) => boolean;
}
