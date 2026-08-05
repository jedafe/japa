import type { Component, Snippet } from 'svelte';

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
}

export interface ActionContext {
  openView: (row: any) => void;
  openEdit: (row: any) => void;
  openDelete: (row: any) => void;
  reload: () => void;
}

export interface ActionItem {
  label: string;
  icon?: Component;
  onclick: (row: any, ctx?: ActionContext) => void;
  variant?: 'default' | 'destructive';
  show?: (row: any) => boolean;
}

export interface FilterDef {
  key: string;
  label: string;
  options: Array<{ value: string; label: string }>;
}

export interface BulkAction {
  label: string;
  icon?: Component;
  onclick: (ids: Set<string>) => void | Promise<void>;
  variant?: 'default' | 'destructive';
  disabled?: (ids: Set<string>, selectedRows?: any[]) => boolean;
}
