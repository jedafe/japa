import { describe, it, expect, beforeEach } from 'vitest';
import { toggleSidebar, setSidebarCollapsed, initSidebarFromStorage, sidebarCollapsed } from './sidebar.svelte';

describe('sidebar store', () => {
  beforeEach(() => {
    localStorage.clear();
    initSidebarFromStorage();
  });

  it('defaults to expanded when no stored value', () => {
    initSidebarFromStorage();
    expect(sidebarCollapsed()).toBe(false);
  });

  it('toggle flips the state and persists', () => {
    expect(sidebarCollapsed()).toBe(false);
    toggleSidebar();
    expect(sidebarCollapsed()).toBe(true);
    expect(localStorage.getItem('jc.admin.sidebar.collapsed')).toBe('1');
  });

  it('reads persisted value on init', () => {
    localStorage.setItem('jc.admin.sidebar.collapsed', '1');
    initSidebarFromStorage();
    expect(sidebarCollapsed()).toBe(true);
  });

  it('setSidebarCollapsed writes through', () => {
    setSidebarCollapsed(true);
    expect(sidebarCollapsed()).toBe(true);
    expect(localStorage.getItem('jc.admin.sidebar.collapsed')).toBe('1');
    setSidebarCollapsed(false);
    expect(localStorage.getItem('jc.admin.sidebar.collapsed')).toBe('0');
  });
});
