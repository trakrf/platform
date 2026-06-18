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
      expect(screen.getByLabelText(/organization name/i)).toBeInTheDocument();
      expect(screen.getByLabelText(/password/i)).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /sign up/i })).toBeInTheDocument();
      expect(screen.getByText(/already have an account/i)).toBeInTheDocument();
    });

    it('should show helper text for existing company users', () => {
      render(<SignupScreen />);

      expect(screen.getByText(/ask your admin for an invite/i)).toBeInTheDocument();
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

    it('should not show required error on blur with empty org name', async () => {
      // Required errors on blur of untouched fields cause layout shifts that
      // break click targets on nearby elements (TRA-490). Required is only
      // enforced on submit.
      render(<SignupScreen />);

      const orgNameInput = screen.getByLabelText(/organization name/i);
      fireEvent.change(orgNameInput, { target: { value: '' } });
      fireEvent.blur(orgNameInput);

      await new Promise((r) => setTimeout(r, 50));
      expect(screen.queryByText(/organization name is required/i)).not.toBeInTheDocument();
    });

    it('should show org name error on blur with name too short', async () => {
      render(<SignupScreen />);

      const orgNameInput = screen.getByLabelText(/organization name/i);
      fireEvent.change(orgNameInput, { target: { value: 'A' } });
      fireEvent.blur(orgNameInput);

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
    // TRA-971: self-service signup now also requires name, company website, and
    // phone. fillContactFields populates them so submit reaches the signup call.
    const fillContactFields = () => {
      fireEvent.change(screen.getByLabelText(/your name/i), { target: { value: 'Jane Doe' } });
      fireEvent.change(screen.getByLabelText(/company website/i), { target: { value: 'acme.com' } });
      fireEvent.change(screen.getByLabelText(/phone/i), { target: { value: '555-1234' } });
    };
    const expectedContact = { name: 'Jane Doe', phone: '555-1234', website: 'acme.com' };

    it('should call signup with email, password, org name, and contact details', async () => {
      mockSignup.mockResolvedValue(undefined);
      render(<SignupScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const orgNameInput = screen.getByLabelText(/organization name/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const submitButton = screen.getByRole('button', { name: /sign up/i });

      fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
      fireEvent.change(orgNameInput, { target: { value: 'Test Company' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fillContactFields();
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(mockSignup).toHaveBeenCalledWith(
          'test@example.com',
          'password123',
          'Test Company',
          undefined,
          expectedContact,
          undefined // acknowledgeNonProd — undefined on prod/local (jsdom host is localhost)
        );
        expect(mockSignup).toHaveBeenCalledTimes(1);
      });
    });

    it('should trim org name before submitting', async () => {
      mockSignup.mockResolvedValue(undefined);
      render(<SignupScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const orgNameInput = screen.getByLabelText(/organization name/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const submitButton = screen.getByRole('button', { name: /sign up/i });

      fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
      fireEvent.change(orgNameInput, { target: { value: '  Trimmed Company  ' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fillContactFields();
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(mockSignup).toHaveBeenCalledWith(
          'test@example.com',
          'password123',
          'Trimmed Company',
          undefined,
          expectedContact,
          undefined
        );
      });
    });

    it('should redirect to home after successful signup', async () => {
      mockSignup.mockResolvedValue(undefined);
      render(<SignupScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const orgNameInput = screen.getByLabelText(/organization name/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const submitButton = screen.getByRole('button', { name: /sign up/i });

      fireEvent.change(emailInput, { target: { value: 'test@example.com' } });
      fireEvent.change(orgNameInput, { target: { value: 'Test Company' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fillContactFields();
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
      const orgNameInput = screen.getByLabelText(/organization name/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const submitButton = screen.getByRole('button', { name: /sign up/i });

      fireEvent.change(emailInput, { target: { value: 'existing@example.com' } });
      fireEvent.change(orgNameInput, { target: { value: 'Test Company' } });
      fireEvent.change(passwordInput, { target: { value: 'password123' } });
      fillContactFields();
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText(errorMessage)).toBeInTheDocument();
      });
    });

    it('should show the go-to-production panel on a 403 (non-prod env block)', async () => {
      // TRA-970: backend returns 403 on non-prod self-service signup.
      mockSignup.mockRejectedValue({ response: { status: 403 } });
      render(<SignupScreen />);

      fireEvent.change(screen.getByLabelText(/email/i), { target: { value: 'test@example.com' } });
      fireEvent.change(screen.getByLabelText(/organization name/i), { target: { value: 'Test Company' } });
      fireEvent.change(screen.getByLabelText(/password/i), { target: { value: 'password123' } });
      fillContactFields();
      fireEvent.click(screen.getByRole('button', { name: /sign up/i }));

      await waitFor(() => {
        expect(screen.getByText(/app\.trakrf\.id/i)).toBeInTheDocument();
      });
    });
  });

  // TRA-970: on a non-prod host (anything but app.trakrf.id / localhost) the form
  // warns and requires a deliberate sandbox acknowledgment before submitting.
  describe('Non-prod sandbox', () => {
    let originalLocation: Location;

    beforeEach(() => {
      originalLocation = window.location;
      Object.defineProperty(window, 'location', {
        configurable: true,
        writable: true,
        value: { hostname: 'app.preview.trakrf.id', hash: '' },
      });
    });

    afterEach(() => {
      Object.defineProperty(window, 'location', {
        configurable: true,
        writable: true,
        value: originalLocation,
      });
    });

    const fillAll = () => {
      fireEvent.change(screen.getByLabelText(/email/i), { target: { value: 'test@example.com' } });
      fireEvent.change(screen.getByLabelText(/organization name/i), { target: { value: 'Test Company' } });
      fireEvent.change(screen.getByLabelText(/password/i), { target: { value: 'password123' } });
      fireEvent.change(screen.getByLabelText(/your name/i), { target: { value: 'Jane Doe' } });
      fireEvent.change(screen.getByLabelText(/company website/i), { target: { value: 'acme.com' } });
      fireEvent.change(screen.getByLabelText(/phone/i), { target: { value: '555-1234' } });
    };

    it('shows the non-production warning banner', () => {
      render(<SignupScreen />);
      expect(screen.getByText(/non-production environment/i)).toBeInTheDocument();
    });

    it('blocks submit until the sandbox acknowledgment is checked', async () => {
      mockSignup.mockResolvedValue(undefined);
      render(<SignupScreen />);
      fillAll();
      fireEvent.click(screen.getByRole('button', { name: /sign up/i }));

      await waitFor(() => {
        expect(screen.getByText(/acknowledge this is a non-production/i)).toBeInTheDocument();
      });
      expect(mockSignup).not.toHaveBeenCalled();
    });

    it('passes acknowledgeNonProd=true once acknowledged', async () => {
      mockSignup.mockResolvedValue(undefined);
      render(<SignupScreen />);
      fillAll();
      fireEvent.click(screen.getByLabelText(/non-production test environment/i));
      fireEvent.click(screen.getByRole('button', { name: /sign up/i }));

      await waitFor(() => {
        expect(mockSignup).toHaveBeenCalledWith(
          'test@example.com',
          'password123',
          'Test Company',
          undefined,
          { name: 'Jane Doe', phone: '555-1234', website: 'acme.com' },
          true
        );
      });
    });
  });

  describe('Loading State', () => {
    it('should disable form during loading', async () => {
      mockSignup.mockImplementation(() => new Promise(() => {}));
      useAuthStore.setState({ isLoading: true });

      render(<SignupScreen />);

      const emailInput = screen.getByLabelText(/email/i);
      const orgNameInput = screen.getByLabelText(/organization name/i);
      const passwordInput = screen.getByLabelText(/password/i);
      const submitButton = screen.getByRole('button', { name: /creating account/i });

      expect(emailInput).toBeDisabled();
      expect(orgNameInput).toBeDisabled();
      expect(passwordInput).toBeDisabled();
      expect(submitButton).toBeDisabled();
    });

    it('should show loading text during submission', async () => {
      useAuthStore.setState({ isLoading: true });
      render(<SignupScreen />);

      expect(screen.getByText(/creating account/i)).toBeInTheDocument();
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
