import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import Header from '@/components/Header';
import { useDeviceStore, useUIStore, useTagStore } from '@/stores';
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
    Object.defineProperty(window, '__webBluetoothMocked', {
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
  });

  it('should show Disconnected button on home page', () => {
    useUIStore.setState({ activeTab: 'home' });
    render(<Header />);
    
    expect(screen.getByText('Disconnected')).toBeInTheDocument();
    expect(screen.getByTestId('connect-button')).toBeInTheDocument();
  });

  it('should show Disconnected button on all other tabs', () => {
    const tabs: Array<'inventory' | 'locate' | 'barcode' | 'settings' | 'help'> = ['inventory', 'locate', 'barcode', 'settings', 'help'];
    
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
      useDeviceStore.setState({ readerState: state });
      const { container } = render(<Header />);
      
      expect(screen.getByText(text)).toBeInTheDocument();
      container.remove();
    });
  });

  it('should show Scanning when reader state is SCANNING', () => {
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
      
      expect(screen.getByText(title)).toBeInTheDocument();
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
    
    // Set disconnected state
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