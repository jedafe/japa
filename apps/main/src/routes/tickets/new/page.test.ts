import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
  getToken: vi.fn().mockReturnValue(null),
  api: {
    createTicket: vi.fn().mockRejectedValue(new Error('Not connected'))
  }
}));

vi.mock('$app/navigation', () => ({
  goto: vi.fn()
}));

describe('New Ticket +page.svelte', () => {
  it('renders the New Support Ticket heading', () => {
    render(Page);
    expect(screen.getByRole('heading', { name: /New Support Ticket/i })).toBeTruthy();
  });

  it('shows the subtitle text', () => {
    render(Page);
    expect(screen.getByText('Describe your issue and we\'ll help you out.')).toBeTruthy();
  });

  it('renders the Back to Tickets link', () => {
    render(Page);
    expect(screen.getByText('Back to Tickets')).toBeTruthy();
  });

  it('renders the Create Ticket button', () => {
    render(Page);
    expect(screen.getByText('Create Ticket')).toBeTruthy();
  });

  it('renders the form fields', () => {
    render(Page);
    expect(screen.getByLabelText('Category')).toBeTruthy();
    expect(screen.getByLabelText('Subject')).toBeTruthy();
    expect(screen.getByLabelText('Description')).toBeTruthy();
  });
});