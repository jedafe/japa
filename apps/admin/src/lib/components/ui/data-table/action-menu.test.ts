import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import ActionMenu from './action-menu.svelte';
import { Eye, Pencil, Trash2 } from 'lucide-svelte';

describe('ActionMenu', () => {
  it('renders a ⋮ trigger button', () => {
    const { container } = render(ActionMenu, { actions: [], row: { id: '1' } });
    expect(container.querySelector('[data-action-menu-trigger]')).toBeTruthy();
  });

  it('shows only actions whose show() returns true', async () => {
    const onView = vi.fn();
    const onDelete = vi.fn();
    const { container } = render(ActionMenu, {
      row: { id: '1', status: 'active' },
      actions: [
        { label: 'View', icon: Eye, onclick: onView },
        { label: 'Delete', icon: Trash2, onclick: onDelete, show: (r) => r.status === 'inactive' }
      ]
    });
    await fireEvent.click(container.querySelector('[data-action-menu-trigger]')!);
    await waitFor(() => expect(screen.getByText('View')).toBeTruthy());
    expect(screen.queryByText('Delete')).toBeNull();
  });

  it('fires onclick with the row when an item is selected', async () => {
    const onEdit = vi.fn();
    const { container } = render(ActionMenu, {
      row: { id: '42' },
      actions: [{ label: 'Edit', icon: Pencil, onclick: onEdit }]
    });
    await fireEvent.click(container.querySelector('[data-action-menu-trigger]')!);
    await waitFor(() => expect(screen.getByText('Edit')).toBeTruthy());
    await fireEvent.click(screen.getByText('Edit'));
    expect(onEdit).toHaveBeenCalledWith({ id: '42' });
  });
});
