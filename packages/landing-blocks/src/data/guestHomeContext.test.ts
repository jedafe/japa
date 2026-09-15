import { describe, it, expect } from 'vitest';
import { setGuestHomeData, getGuestHomeData } from './guestHomeContext';

describe('guestHomeContext', () => {
  it('round-trips data through a writable store', () => {
    const data = { topDestinations: [], opportunities: [], stories: [], tools: [], stats: [], categories: [] };
    const store = setGuestHomeData(data as any);
    expect(getGuestHomeData()).toBe(store);
    let seen: any;
    store.subscribe((v) => (seen = v));
    expect(seen).toBe(data);
  });
});
