import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { UserMenu } from './UserMenu';
import type { User } from '@/lib/api/auth';

afterEach(() => {
  cleanup();
});

const mockUser: User = {
  id: 1,
  email: 'test@example.com',
  name: 'Test User',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
};

describe('UserMenu', () => {
  it('should render user email', () => {
    const onLogout = vi.fn();
    render(<UserMenu user={mockUser} onLogout={onLogout} />);
    expect(screen.getByText('test@example.com')).toBeInTheDocument();
  });

  it('should render avatar with correct initials', () => {
    const onLogout = vi.fn();
    render(<UserMenu user={mockUser} onLogout={onLogout} />);
    expect(screen.getByText('TE')).toBeInTheDocument(); // "test" -> "TE"
  });

  it('should open dropdown on button click', async () => {
    const onLogout = vi.fn();
    render(<UserMenu user={mockUser} onLogout={onLogout} />);

    // Initially, logout button should not be visible
    expect(screen.queryByText('Logout')).not.toBeInTheDocument();

    // Click menu button to open dropdown
    const menuButton = screen.getByRole('button', { name: /test@example.com/i });
    fireEvent.click(menuButton);

    // Now logout button should be visible
    expect(screen.getByText('Logout')).toBeInTheDocument();
  });

  it('should call onLogout when logout button clicked', () => {
    const onLogout = vi.fn();
    render(<UserMenu user={mockUser} onLogout={onLogout} />);

    // Open dropdown
    const menuButton = screen.getByRole('button', { name: /test@example.com/i });
    fireEvent.click(menuButton);

    // Click logout
    const logoutButton = screen.getByText('Logout');
    fireEvent.click(logoutButton);

    expect(onLogout).toHaveBeenCalledTimes(1);
  });

  it('should hide email text on mobile (< 640px)', () => {
    const onLogout = vi.fn();
    const { container } = render(<UserMenu user={mockUser} onLogout={onLogout} />);

    const emailSpan = container.querySelector('.hidden.sm\\:inline-block');
    expect(emailSpan).toBeInTheDocument();
    expect(emailSpan).toHaveTextContent('test@example.com');
  });
});
