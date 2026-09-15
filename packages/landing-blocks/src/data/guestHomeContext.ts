// packages/landing-blocks/src/data/guestHomeContext.ts
// Shared GuestHomeData store for all guest_* blocks. Stored in Svelte context
// so it can be read by deeply nested components without prop drilling.
import { getContext, setContext } from 'svelte';
import { writable, type Writable } from 'svelte/store';
import type { GuestHomeData } from '../types';

const GUEST_HOME_CTX = Symbol('guest-home-data');

// Fallback store for tests and non-component environments (e.g. SSR entry points).
let moduleStore: Writable<GuestHomeData | undefined> | undefined = undefined;

function getStoreFromContext(): Writable<GuestHomeData | undefined> | undefined {
  try {
    return getContext<Writable<GuestHomeData | undefined> | undefined>(GUEST_HOME_CTX);
  } catch {
    return undefined;
  }
}

export function setGuestHomeData(data: GuestHomeData | undefined): Writable<GuestHomeData | undefined> {
  const existing = getStoreFromContext();
  if (existing) {
    existing.set(data);
    return existing;
  }
  if (moduleStore) {
    moduleStore.set(data);
    return moduleStore;
  }
  const store = writable<GuestHomeData | undefined>(data);
  try {
    setContext(GUEST_HOME_CTX, store);
  } catch {
    moduleStore = store;
  }
  return store;
}

export function getGuestHomeData(): Writable<GuestHomeData | undefined> | undefined {
  return getStoreFromContext() ?? moduleStore;
}
