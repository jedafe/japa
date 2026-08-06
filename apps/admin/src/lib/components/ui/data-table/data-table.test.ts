import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import { DataTable } from './index';
import { Eye, Pencil, Trash2 } from 'lucide-svelte';

vi.mock('$app/state', () => ({ page: { params: {}, url: new URL('http://localhost/') } }));
vi.mock('$app/navigation', () => ({ goto: vi.fn(), replaceState: vi.fn(), pushState: vi.fn() }));

const rows = [{ id: '1', name: 'Alpha' }, { id: '2', name: 'Beta' }];

describe('DataTable server-side mode', () => {
  it('renders all rows without client slicing when serverSide=true', () => {
    render(DataTable, {
      columns: [{ key: 'name', label: 'Name' }],
      data: rows,
      serverSide: true,
      totalRows: 100,
      totalPages: 10,
      page: 0,
      showPagination: true
    });
    // Both rows present (no client-side slice)
    expect(screen.getByText('Alpha')).toBeTruthy();
    expect(screen.getByText('Beta')).toBeTruthy();
  });

  it('emits onpagechange when pagination advances', async () => {
    const onpage = vi.fn();
    const { container } = render(DataTable, {
      columns: [{ key: 'name', label: 'Name' }],
      data: rows,
      serverSide: true,
      totalRows: 100,
      totalPages: 10,
      page: 0,
      onpagechange: onpage,
      showPagination: true
    });
    const next = container.querySelector('[aria-label="Next page"]');
    await fireEvent.click(next!);
    expect(onpage).toHaveBeenCalledWith(1);
  });

  it('emits onpagesizechange when page size changes', async () => {
    const onpagesize = vi.fn();
    const { rerender } = render(DataTable, {
      columns: [{ key: 'name', label: 'Name' }],
      data: rows,
      serverSide: true,
      totalRows: 100,
      totalPages: 10,
      page: 0,
      pageSize: 10,
      onpagesizechange: onpagesize,
      showPagination: true
    });
    // Re-render with a new pageSize to trigger the server-side page-size effect.
    await rerender({ pageSize: 25 });
    expect(onpagesize).toHaveBeenCalledWith(25);
  });
});

describe('DataTable action column', () => {
  it('renders an ActionMenu trigger per row when actions provided', () => {
    const { container } = render(DataTable, {
      columns: [{ key: 'name', label: 'Name' }],
      data: rows,
      actions: [{ label: 'View', icon: Eye, onclick: () => {} }]
    });
    const triggers = container.querySelectorAll('[data-action-menu-trigger]');
    expect(triggers.length).toBe(2);
  });

  it('does not render action column when actions omitted', () => {
    const { container } = render(DataTable, {
      columns: [{ key: 'name', label: 'Name' }],
      data: rows
    });
    const triggers = container.querySelectorAll('[data-action-menu-trigger]');
    expect(triggers.length).toBe(0);
  });
});

describe('DataTable bulk bar', () => {
  it('renders the bulk bar with count when ids selected', () => {
    render(DataTable, {
      columns: [{ key: 'name', label: 'Name' }],
      data: rows,
      selectable: true,
      selectedIds: new Set(['1']),
      bulkActionsList: [{ label: 'Delete', icon: Trash2, onclick: () => {}, variant: 'destructive' }]
    });
    expect(screen.getByText(/1 selected/)).toBeTruthy();
  });

  it('does not render bulk bar when no ids selected', () => {
    render(DataTable, {
      columns: [{ key: 'name', label: 'Name' }],
      data: rows,
      selectable: true,
      selectedIds: new Set<string>(),
      bulkActionsList: [{ label: 'Delete', icon: Trash2, onclick: () => {}, variant: 'destructive' }]
    });
    expect(screen.queryByText(/1 selected/)).toBeNull();
  });
});

describe('DataTable create button', () => {
  it('renders a Create button when createLabel + onCreate provided', () => {
    const onCreate = vi.fn();
    render(DataTable, {
      columns: [{ key: 'name', label: 'Name' }],
      data: rows,
      createLabel: 'New Item',
      onCreate
    });
    expect(screen.getByText('New Item')).toBeTruthy();
  });

  it('fires onCreate when the Create button is clicked', async () => {
    const onCreate = vi.fn();
    render(DataTable, {
      columns: [{ key: 'name', label: 'Name' }],
      data: rows,
      createLabel: 'New Item',
      onCreate
    });
    await fireEvent.click(screen.getByText('New Item'));
    expect(onCreate).toHaveBeenCalled();
  });
});

describe('DataTable backward compatibility', () => {
  it('keeps client-side slicing when serverSide omitted (default)', () => {
    const many = Array.from({ length: 25 }, (_, i) => ({ id: String(i), name: `Row ${i}` }));
    render(DataTable, {
      columns: [{ key: 'name', label: 'Name' }],
      data: many,
      pageSize: 10,
      showPagination: true
    });
    // Only first 10 rows should be visible (client-side slice)
    expect(screen.getByText('Row 0')).toBeTruthy();
    expect(screen.getByText('Row 9')).toBeTruthy();
    expect(screen.queryByText('Row 10')).toBeNull();
  });
});
