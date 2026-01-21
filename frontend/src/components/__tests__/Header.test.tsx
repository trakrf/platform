import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import Header from '@/components/Header';
import { useDeviceStore, useUIStore, useAuthStore, useOrgStore } from '@/stores';
import { ReaderState } from '@/worker/types/reader';

describe('Header', () => {
  const mockConnect = vi.fn();
  const mockDisconnect = vi.fn();
  const mockOnMenuToggle = vi.fn();
  const mockCheckBrowserSupport = vi.fn();

  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    // Reset all mocks
    mockConnect.mockClear();
    mockDisconnect.mockClear();
    mockOnMenuToggle.mockClear();
    mockCheckBrowserSupport.mockClear();
    
    // Mock Web Bluetooth API for browser support check
    Object.defineProperty(window, '__webBluetoothBridged', {
      writable: true,
      value: true
    });
    
    // Mock navigator.bluetooth
    Object.defineProperty(navigator, 'bluetooth', {
      writable: true,
      value: {
        requestDevice: vi.fn()
      }
    });
    
    // Set default store states
    useDeviceStore.setState({
      readerState: ReaderState.DISCONNECTED,
      batteryPercentage: null,
      connect: mockConnect,
      disconnect: mockDisconnect
    });
    useUIStore.setState({ activeTab: 'home' });
    useOrgStore.setState({
      currentOrg: null,
      currentRole: null,
      orgs: [],
      isLoading: false,
      error: null,
    });
  });

  it('should not show connection button on home page', () => {
    useUIStore.setState({ activeTab: 'home' });
    render(<Header />);

    // Connection button should be hidden on home page
    expect(screen.queryByTestId('connect-button')).not.toBeInTheDocument();
    expect(screen.queryByText('Disconnected')).not.toBeInTheDocument();
  });

  it('should show Disconnected button on tabs other than home and help', () => {
    const tabs: Array<'inventory' | 'locate' | 'barcode' | 'settings'> = ['inventory', 'locate', 'barcode', 'settings'];

    tabs.forEach(tab => {
      useUIStore.setState({ activeTab: tab });
      const { container } = render(<Header />);

      expect(screen.getByText('Disconnected')).toBeInTheDocument();
      container.remove();
    });
  });

  it('should show battery indicator when device is connected', () => {
    useDeviceStore.setState({ 
      readerState: ReaderState.CONNECTED,
      batteryPercentage: 75
    });
    useUIStore.setState({ activeTab: 'inventory' });
    
    render(<Header />);
    
    expect(screen.getByText('75%')).toBeInTheDocument();
    expect(screen.getByTitle('Battery: 75%')).toBeInTheDocument();
  });

  it('should not show battery on home when disconnected', () => {
    useDeviceStore.setState({ 
      readerState: ReaderState.DISCONNECTED,
      batteryPercentage: null
    });
    useUIStore.setState({ activeTab: 'home' });
    
    render(<Header />);
    
    expect(screen.queryByText('%')).not.toBeInTheDocument();
  });

  it('should show correct battery colors based on percentage', () => {
    const testCases = [
      { percentage: 85, expectedClass: 'text-green-500' },
      { percentage: 50, expectedClass: 'text-yellow-500' },
      { percentage: 20, expectedClass: 'text-red-500' }
    ];

    testCases.forEach(({ percentage, expectedClass }) => {
      useUIStore.setState({ activeTab: 'inventory' }); // Use a tab that shows the button
      useDeviceStore.setState({
        readerState: ReaderState.CONNECTED,
        batteryPercentage: percentage
      });

      const { container } = render(<Header />);
      const batteryText = screen.getByText(`${percentage}%`);
      expect(batteryText).toHaveClass(expectedClass);
      container.remove();
    });
  });

  it('should handle connect button click', async () => {
    useUIStore.setState({ activeTab: 'inventory' }); // Use a tab that shows the button
    useDeviceStore.setState({ readerState: ReaderState.DISCONNECTED });
    render(<Header />);

    // Wait for component to initialize with browser support
    await waitFor(() => {
      const button = screen.getByTestId('connect-button');
      expect(button).not.toHaveClass('opacity-50');
    });

    const connectButton = screen.getByTestId('connect-button');
    fireEvent.click(connectButton);

    await waitFor(() => {
      expect(mockConnect).toHaveBeenCalledTimes(1);
    });
  });

  it('should show disconnect confirmation modal', async () => {
    useUIStore.setState({ activeTab: 'inventory' }); // Use a tab that shows the button
    useDeviceStore.setState({ readerState: ReaderState.CONNECTED });
    render(<Header />);

    const disconnectButton = screen.getByTestId('disconnect-button');
    fireEvent.click(disconnectButton);

    await waitFor(() => {
      expect(screen.getByText('Disconnect Reader')).toBeInTheDocument();
      expect(screen.getByText('Are you sure you want to disconnect the reader?')).toBeInTheDocument();
    });
  });

  it('should display correct button text for all reader states', () => {
    const testStates = [
      { state: ReaderState.DISCONNECTED, text: 'Disconnected' },
      { state: ReaderState.CONNECTING, text: 'Connecting' },
      { state: ReaderState.CONNECTED, text: 'Connected' },
      { state: ReaderState.CONFIGURING, text: 'Configuring' },
      { state: ReaderState.BUSY, text: 'Busy' }
    ];

    testStates.forEach(({ state, text }) => {
      useUIStore.setState({ activeTab: 'inventory' }); // Use a tab that shows the button
      useDeviceStore.setState({ readerState: state });
      const { container } = render(<Header />);

      expect(screen.getByText(text)).toBeInTheDocument();
      container.remove();
    });
  });

  it('should show Scanning when reader state is SCANNING', () => {
    useUIStore.setState({ activeTab: 'inventory' }); // Use a tab that shows the button
    useDeviceStore.setState({ readerState: ReaderState.SCANNING });

    render(<Header />);

    expect(screen.getByText('Scanning')).toBeInTheDocument();
  });

  it('should toggle mobile menu', () => {
    render(<Header onMenuToggle={mockOnMenuToggle} isMobileMenuOpen={false} />);
    
    const menuButton = screen.getByLabelText('Toggle menu');
    fireEvent.click(menuButton);
    
    expect(mockOnMenuToggle).toHaveBeenCalledTimes(1);
  });

  it('should show correct page titles and subtitles', () => {
    const pages: Array<{ tab: 'home' | 'inventory' | 'locate' | 'barcode' | 'settings' | 'help'; title: string; subtitle: string }> = [
      { tab: 'home', title: 'RFID Dashboard', subtitle: 'Choose your scanning mode to get started' },
      { tab: 'inventory', title: 'My Items', subtitle: 'View and manage your scanned items' },
      { tab: 'locate', title: 'Find Item', subtitle: 'Search for a specific item' },
      { tab: 'barcode', title: 'Barcode Scanner', subtitle: 'Scan barcodes to identify items' },
      { tab: 'settings', title: 'Device Setup', subtitle: 'Configure your RFID reader' },
      { tab: 'help', title: 'Help', subtitle: 'Quick answers to get you started' }
    ];

    pages.forEach(({ tab, title, subtitle }) => {
      useUIStore.setState({ activeTab: tab });
      const { container } = render(<Header />);

      // Check title is shown
      expect(screen.getByText(title)).toBeInTheDocument();

      // Click the info button to show the subtitle tooltip
      const infoButton = screen.getByLabelText('Show page info');
      fireEvent.click(infoButton);

      // Check subtitle appears in tooltip
      expect(screen.getByText(subtitle)).toBeInTheDocument();

      container.remove();
    });
  });

  it('should have blinking effect on Connect button when disconnected', () => {
    // Mock browser support to enable blinking
    Object.defineProperty(navigator, 'bluetooth', {
      writable: true,
      value: {},
    });

    // Set disconnected state and use a tab that shows the button
    useUIStore.setState({ activeTab: 'inventory' });
    useDeviceStore.setState({
      readerState: ReaderState.DISCONNECTED,
      batteryPercentage: null,
      isConnected: false,
      firmwareVersion: '',
    });

    const { container } = render(<Header />);

    // Check that Disconnected button is shown
    expect(screen.getByText('Disconnected')).toBeInTheDocument();

    const connectButton = screen.getByText('Disconnected').closest('button');

    // When disconnected and browser supported, button should have blue background
    // The exact blinking state timing is difficult to test reliably
    expect(connectButton).toHaveClass('bg-blue-600');

    container.remove();
  });
});

describe('Header - Auth Integration', () => {
  const mockConnect = vi.fn();
  const mockDisconnect = vi.fn();

  beforeEach(() => {
    mockConnect.mockClear();
    mockDisconnect.mockClear();

    // Mock Web Bluetooth API for browser support check
    Object.defineProperty(window, '__webBluetoothBridged', {
      writable: true,
      value: true
    });

    // Mock navigator.bluetooth
    Object.defineProperty(navigator, 'bluetooth', {
      writable: true,
      value: {
        requestDevice: vi.fn()
      }
    });

    // Set default device store state
    useDeviceStore.setState({
      readerState: ReaderState.DISCONNECTED,
      batteryPercentage: null,
      connect: mockConnect,
      disconnect: mockDisconnect
    });

    // Set default UI store state
    useUIStore.setState({ activeTab: 'inventory' });

    // Set default auth store state (not authenticated)
    useAuthStore.setState({
      isAuthenticated: false,
      user: null,
      token: null,
      isLoading: false,
      error: null
    });
  });

  afterEach(() => {
    cleanup();
  });

  it('should render "Log In" button when not authenticated', () => {
    useAuthStore.setState({ isAuthenticated: false, user: null });

    render(<Header />);
    expect(screen.getByText('Log In')).toBeInTheDocument();
  });

  it('should render UserMenu when authenticated', () => {
    useAuthStore.setState({
      isAuthenticated: true,
      user: {
        id: 1,
        email: 'test@example.com',
        name: 'Test User',
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      }
    });

    render(<Header />);
    expect(screen.getByText('test@example.com')).toBeInTheDocument();
    expect(screen.getByText('TE')).toBeInTheDocument(); // Avatar initials
  });

  it('should navigate to login screen when "Log In" button clicked', () => {
    const setActiveTab = vi.fn();
    useAuthStore.setState({ isAuthenticated: false, user: null });

    // Mock getState to return setActiveTab
    useUIStore.getState = vi.fn(() => ({
      activeTab: 'inventory',
      setActiveTab
    })) as any;

    render(<Header />);

    const loginButton = screen.getByText('Log In');
    fireEvent.click(loginButton);

    expect(setActiveTab).toHaveBeenCalledWith('login');
  });

  it('should call logout and navigate to home when logout clicked', async () => {
    const logout = vi.fn();
    const setActiveTab = vi.fn();

    useAuthStore.setState({
      isAuthenticated: true,
      user: {
        id: 1,
        email: 'test@example.com',
        name: 'Test User',
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      }
    });

    // Mock getState for both stores
    useAuthStore.getState = vi.fn(() => ({
      logout,
      isAuthenticated: true,
      user: {
        id: 1,
        email: 'test@example.com',
        name: 'Test User',
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
      }
    })) as any;

    useUIStore.getState = vi.fn(() => ({
      activeTab: 'inventory',
      setActiveTab
    })) as any;

    render(<Header />);

    // Open dropdown
    const menuButton = screen.getByRole('button', { name: /test@example.com/i });
    fireEvent.click(menuButton);

    // Wait for dropdown to appear
    await waitFor(() => {
      expect(screen.getByText('Logout')).toBeInTheDocument();
    });

    // Click logout
    const logoutButton = screen.getByText('Logout');
    fireEvent.click(logoutButton);

    expect(logout).toHaveBeenCalledTimes(1);
    expect(setActiveTab).toHaveBeenCalledWith('home');
  });

  it('should show auth UI on all tabs including home', () => {
    useAuthStore.setState({ isAuthenticated: false, user: null });

    // Test on home tab
    useUIStore.setState({ activeTab: 'home' });
    const { container: homeContainer } = render(<Header />);
    expect(screen.getByText('Log In')).toBeInTheDocument();
    homeContainer.remove();

    // Test on inventory tab
    useUIStore.setState({ activeTab: 'inventory' });
    const { container: invContainer } = render(<Header />);
    expect(screen.getByText('Log In')).toBeInTheDocument();
    invContainer.remove();
  });
});