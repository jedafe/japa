import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
  getToken: vi.fn().mockReturnValue(null)
}));

const mockProfile = {
  id: '123',
  name: 'Test User',
  headline: 'Software Engineer',
  slug: 'test-user',
  skills: ['JavaScript', 'TypeScript'],
  is_public: true,
  author_user_id: 'user-1',
  relocations: [],
  documents: [],
  verifiers: []
};

describe('Professional Profile +page.svelte', () => {
  it('renders the profile name as heading', () => {
    render(Page, { props: { data: { profile: mockProfile } } });
    expect(screen.getByRole('heading', { name: /Test User/i })).toBeTruthy();
  });

  it('shows the headline text', () => {
    render(Page, { props: { data: { profile: mockProfile } } });
    expect(screen.getByText('Software Engineer')).toBeTruthy();
  });

  it('shows empty state for relocations', () => {
    render(Page, { props: { data: { profile: mockProfile } } });
    expect(screen.getByText('No relocations added yet.')).toBeTruthy();
  });

  it('shows empty state for documents', () => {
    render(Page, { props: { data: { profile: mockProfile } } });
    expect(screen.getByText('No documents added yet.')).toBeTruthy();
  });

  it('renders the back to profiles link', () => {
    render(Page, { props: { data: { profile: mockProfile } } });
    expect(screen.getByText('Back to profiles')).toBeTruthy();
  });
});