// packages/landing-blocks/src/data/featuresContext.ts
// Shared feature-flag map for all blocks. Stored in Svelte context so
// flag-gated blocks (e.g. feature.newsletter → guest_newsletter) can hide
// during SSR instead of flashing and unmounting on hydration — the map is
// plain layout data, no fetch needed, unlike FetchContext (browser-only).
import { getContext, setContext } from 'svelte';
import { writable, type Writable } from 'svelte/store';

const FEATURES_CTX = Symbol('landing-features');

// Fallback store for tests and non-component environments (e.g. SSR entry points).
let moduleStore: Writable<Record<string, boolean> | undefined> | undefined = undefined;

function getStoreFromContext(): Writable<Record<string, boolean> | undefined> | undefined {
  try {
    return getContext<Writable<Record<string, boolean> | undefined> | undefined>(FEATURES_CTX);
  } catch {
    return undefined;
  }
}

export function setFeaturesData(data: Record<string, boolean> | undefined): Writable<Record<string, boolean> | undefined> {
  const existing = getStoreFromContext();
  if (existing) {
    existing.set(data);
    return existing;
  }
  if (moduleStore) {
    moduleStore.set(data);
    return moduleStore;
  }
  const store = writable<Record<string, boolean> | undefined>(data);
  try {
    setContext(FEATURES_CTX, store);
  } catch {
    moduleStore = store;
  }
  return store;
}

export function getFeaturesData(): Writable<Record<string, boolean> | undefined> | undefined {
  return getStoreFromContext() ?? moduleStore;
}
