import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import SignupScreen from '@/components/SignupScreen';
import { useAuthStore } from '@/stores';

describe('SignupScreen', () => {
  const mockSignup = vi.fn();

  beforeEach(() => {
    mockSignup.mockClear();
    useAuthStore.setState({
      user: null,
      token: null,
      isAuthenticated: false,
      isLoading: false,
      error: null,
    });
    useAuthStore.getState().signup = mockSignup;
  });

  afterEach(() => {
    cleanup();
  });

  describe('Rendering', () => {
    it('should render signup form with all fields', () => {
      render(<SignupScreen />);

      expect(screen.getByRole('heading', { name: 'Sign Up' })).toBeInTheDocument();
      expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/organization name/i)).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /sign up/i })).toBeInTheDocument();
      expect(screen.getByText(/already have an account/i)).toBeInTheDocument();
    });

    it('should render password visibility toggle', () => {
      render(<SignupScreen />);

      const toggleButtons = screen.getAllByRole('button');
      // Should have submit button and toggle button
      expect(toggleButtons.length).toBeGreaterThanOrEqual(2);
    });
  });

  describe('Validation', () => {
    it('should show email error on blur with invalid email', async () => {
      render(<SignupScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      fireEvent.change(emailInput, { target: { value: 'invalid-email' } });
      fireEvent.blur(emailInput);

      await waitFor(() => {
        expect(screen.getByText(/invalid email format/i)).toBeInTheDocument();
      });
    });

    it('should show password error on blur with short password', async () => {
      render(<SignupScreen />);

      const passwordInput = screen.getByLabelText(/password/i);
      fireEvent.change(passwordInput, { target: { value: 'short' } });
      fireEvent.blur(passwordInput);

      await waitFor(() => {
        expect(screen.getByText(/at least 8 characters/i)).toBeInTheDocument();
      });
    });

    it('should show organization error on blur with short name', async () => {
      render(<SignupScreen />);

      const orgInput = screen.getByLabelText(/organization name/i);
      fireEvent.change(orgInput, { target: { value: 'a' } });
      fireEvent.blur(orgInput);

      await waitFor(() => {
        expect(screen.getByText(/at least 2 characters/i)).toBeInTheDocument();
      });
    });

    it('should not submit with validation errors', async () => {
      render(<SignupScreen />);

      const submitButton = screen.getByRole('button', { name: /sign up/i });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(mockSignup).not.toHaveBeenCalled();
      });
    });
  });

  describe('Password Visibility Toggle', () => {
    it('should toggle password visibility on icon click', () => {
      render(<SignupScreen />);

      const passwordInput = screen.getByLabelText(/password/i) as HTMLInputElement;
      const toggleButtons = screen.getAllByRole('button');
      const toggleButton = toggleButtons.find(btn => btn !== screen.getByRole('button', { name: /sign up/i }));

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
    it('should call signup with correct data on valid submit', async () => {
      mockSignup.mockResolvedValue(undefined);
      render(<SignupScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const orgInput = screen.getByLabelText(/organization name/i);
      const submitButton = screen.getByRole('button', { name: /sign up/i });

      fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fireEvent.change(orgInput, { target: { value: 'Acme Corp' } });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(mockSignup).toHaveBeenCalledWith('test@example.com', 'password123', 'Acme Corp');
      });
    });

    it('should redirect to home after successful signup', async () => {
      mockSignup.mockResolvedValue(undefined);
      render(<SignupScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const orgInput = screen.getByLabelText(/organization name/i);
      const submitButton = screen.getByRole('button', { name: /sign up/i });

      fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fireEvent.change(orgInput, { target: { value: 'Acme Corp' } });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(window.location.hash).toBe('#home');
      });
    });

    it('should display backend error message on failed signup', async () => {
      const errorMessage = 'Email already exists';
      mockSignup.mockRejectedValue({
        response: { data: { error: errorMessage } }
      });

      render(<SignupScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const orgInput = screen.getByLabelText(/organization name/i);
      const submitButton = screen.getByRole('button', { name: /sign up/i });

      fireEvent.change(emailInput, { target: { value: 'existing@example.com' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fireEvent.change(orgInput, { target: { value: 'Acme Corp' } });
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText(errorMessage)).toBeInTheDocument();
      });
    });
  });

  describe('Loading State', () => {
    it('should disable form during loading', async () => {
      mockSignup.mockImplementation(() => new Promise(() => {}));
      useAuthStore.setState({ isLoading: true });

      render(<SignupScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const orgInput = screen.getByLabelText(/organization name/i);
      const submitButton = screen.getByRole('button', { name: /signing up/i });

      expect(emailInput).toBeDisabled();
      expect(passwordInput).toBeDisabled();
      expect(orgInput).toBeDisabled();
      expect(submitButton).toBeDisabled();
    });

    it('should show loading text during submission', async () => {
      useAuthStore.setState({ isLoading: true });
      render(<SignupScreen />);

      expect(screen.getByText(/signing up/i)).toBeInTheDocument();
    });
  });

  describe('Navigation', () => {
    it('should have link to login page', () => {
      render(<SignupScreen />);

      const loginLink = screen.getByText(/log in/i);
      expect(loginLink).toHaveAttribute('href', '#login');
    });
  });
});
