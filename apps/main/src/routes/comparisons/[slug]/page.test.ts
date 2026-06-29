import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
  getToken: vi.fn().mockReturnValue(null)
}));

const mockType = {
  id: 'type-1',
  name: 'Countries',
  slug: 'countries',
  description: 'Compare countries for relocation'
};

const mockComparisons = [];

describe('Comparison Type +page.svelte', () => {
  it('renders the type name as heading', () => {
    render(Page, { props: { data: { type: mockType, comparisons: mockComparisons } } });
    expect(screen.getByRole('heading', { name: /Countries/i })).toBeTruthy();
  });

  it('shows the type description', () => {
    render(Page, { props: { data: { type: mockType, comparisons: mockComparisons } } });
    expect(screen.getByText('Compare countries for relocation')).toBeTruthy();
  });

  it('renders the back to comparisons link', () => {
    render(Page, { props: { data: { type: mockType, comparisons: mockComparisons } } });
    expect(screen.getByText('Back to comparisons')).toBeTruthy();
  });

  it('renders the Create comparison button', () => {
    render(Page, { props: { data: { type: mockType, comparisons: mockComparisons } } });
    expect(screen.getByText('Create comparison')).toBeTruthy();
  });

  it('shows empty state when no comparisons', () => {
    render(Page, { props: { data: { type: mockType, comparisons: mockComparisons } } });
    expect(screen.getByText('No comparisons yet')).toBeTruthy();
  });
});