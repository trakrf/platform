import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import LoginScreen from '@/components/LoginScreen';
import { useAuthStore } from '@/stores';

describe('LoginScreen', () => {
  const mockLogin = vi.fn();

  beforeEach(() => {
    mockLogin.mockClear();
    useAuthStore.setState({
      user: null,
      token: null,
      isAuthenticated: false,
      isLoading: false,
      error: null,
    });
    useAuthStore.getState().login = mockLogin;

    // Clear sessionStorage
    sessionStorage.clear();
  });

  afterEach(() => {
    cleanup();
  });

  describe('Rendering', () => {
    it('should render login form with all fields', () => {
      render(<LoginScreen />);

      expect(screen.getByRole('heading', { name: 'Log In' })).toBeInTheDocument();
      expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /log in/i })).toBeInTheDocument();
      expect(screen.getByText(/don't have an account/i)).toBeInTheDocument();
    });

    it('should render password visibility toggle', () => {
      render(<LoginScreen />);

      const toggleButtons = screen.getAllByRole('button');
      // Should have submit button and toggle button
      expect(toggleButtons.length).toBeGreaterThanOrEqual(2);
    });
  });

  describe('Validation', () => {
    it('should show email error on blur with invalid email', async () => {
      render(<LoginScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      fireEvent.change(emailInput, { target: { value: 'invalid-email' } });
      fireEvent.blur(emailInput);

      await waitFor(() => {
        expect(screen.getByText(/invalid email format/i)).toBeInTheDocument();
      });
    });

    it('should show password error on blur with empty password', async () => {
      render(<LoginScreen />);

      const passwordInput = screen.getByLabelText(/password/i);
      fireEvent.blur(passwordInput);

      await waitFor(() => {
        expect(screen.getByText(/password is required/i)).toBeInTheDocument();
      });
    });

    it('should not submit with validation errors', async () => {
      render(<LoginScreen />);

      const submitButton = screen.getByRole('button', { name: /log in/i });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(mockLogin).not.toHaveBeenCalled();
      });
    });
  });

  describe('Password Visibility Toggle', () => {
    it('should toggle password visibility on icon click', () => {
      render(<LoginScreen />);

      const passwordInput = screen.getByLabelText(/password/i) as HTMLInputElement;
      const toggleButtons = screen.getAllByRole('button');
      const toggleButton = toggleButtons.find(btn => btn !== screen.getByRole('button', { name: /log in/i }));

      expect(passwordInput.type).toBe('password');

      if (toggleButton) {
        fireEvent.click(toggleButton);
        expect(passwordInput.type).toBe('text');

        fireEvent.click(toggleButton);
        expect(passwordInput.type).toBe('password');
      }
    });
  });

  describe('Form Submission', () => {
    it('should call login with correct credentials on valid submit', async () => {
      mockLogin.mockResolvedValue(undefined);
      render(<LoginScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const submitButton = screen.getByRole('button', { name: /log in/i });

      fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(mockLogin).toHaveBeenCalledWith('test@example.com', 'password123');
      });
    });

    it('should redirect to home after successful login without redirect param', async () => {
      mockLogin.mockResolvedValue(undefined);
      render(<LoginScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const submitButton = screen.getByRole('button', { name: /log in/i });

      fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(window.location.hash).toBe('#home');
      });
    });

    it('should redirect to intended route after successful login', async () => {
      sessionStorage.setItem('redirectAfterLogin', 'devices');
      mockLogin.mockResolvedValue(undefined);
      render(<LoginScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const submitButton = screen.getByRole('button', { name: /log in/i });

      fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(window.location.hash).toBe('#devices');
        expect(sessionStorage.getItem('redirectAfterLogin')).toBeNull();
      });
    });

    it('should display backend error message on failed login', async () => {
      const errorMessage = 'Invalid credentials';
      mockLogin.mockRejectedValue({
        response: { data: { error: errorMessage } }
      });

      render(<LoginScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const submitButton = screen.getByRole('button', { name: /log in/i });

      fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
      fireEvent.change(passwordInput, { target: { value: 'wrongpassword' } });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText(errorMessage)).toBeInTheDocument();
      });
    });
  });

  describe('Loading State', () => {
    it('should disable form during loading', async () => {
      mockLogin.mockImplementation(() => new Promise(() => {})); // Never resolves
      useAuthStore.setState({ isLoading: true });

      render(<LoginScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const submitButton = screen.getByRole('button', { name: /logging in/i });

      expect(emailInput).toBeDisabled();
      expect(passwordInput).toBeDisabled();
      expect(submitButton).toBeDisabled();
    });

    it('should show loading text during submission', async () => {
      useAuthStore.setState({ isLoading: true });
      render(<LoginScreen />);

      expect(screen.getByText(/logging in/i)).toBeInTheDocument();
    });
  });

  describe('Navigation', () => {
    it('should have link to signup page', () => {
      render(<LoginScreen />);

      const signupLink = screen.getByText(/sign up/i);
      expect(signupLink).toHaveAttribute('href', '#signup');
    });
  });
});
