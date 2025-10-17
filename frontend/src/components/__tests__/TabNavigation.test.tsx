import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import TabNavigation from '@/components/TabNavigation';
import { useUIStore, useDeviceStore } from '@/stores';
import { ReaderState } from '@/worker/types/reader';
import { version } from '../../../package.json';

describe('TabNavigation', () => {
  beforeEach(() => {
    // Set default store states
    useUIStore.setState({ activeTab: 'home' });
    useDeviceStore.setState({ readerState: ReaderState.DISCONNECTED });
  });

  afterEach(() => {
    cleanup();
  });

  it('should render all navigation items with correct labels', () => {
    render(<TabNavigation />);
    expect(screen.getByText('Home')).toBeInTheDocument();
    expect(screen.getByText('Inventory')).toBeInTheDocument();
    expect(screen.getByText('Locate')).toBeInTheDocument();
    expect(screen.getByText('Barcode')).toBeInTheDocument();
    expect(screen.getByText('Settings')).toBeInTheDocument();
    expect(screen.getByText('Help')).toBeInTheDocument();
  });

  it('should highlight active tab', () => {
    useUIStore.setState({ activeTab: 'inventory' });
    render(<TabNavigation />);
    
    const inventoryButton = screen.getByText('Inventory').closest('button');
    expect(inventoryButton).toHaveClass('bg-blue-600', 'text-white');
    
    const homeButton = screen.getByText('Home').closest('button');
    expect(homeButton).not.toHaveClass('bg-blue-600');
  });

  it('should navigate when clicking nav items', () => {
    const mockSetActiveTab = vi.fn();
    useUIStore.getState().setActiveTab = mockSetActiveTab;
    
    render(<TabNavigation />);
    const inventoryButton = screen.getByText('Inventory').closest('button');
    fireEvent.click(inventoryButton!);
    
    expect(mockSetActiveTab).toHaveBeenCalledWith('inventory');
  });

  it('should show tooltips on hover', async () => {
    render(<TabNavigation />);
    const inventoryButton = screen.getByText('Inventory').closest('button');
    
    fireEvent.mouseEnter(inventoryButton!);
    
    await waitFor(() => {
      expect(screen.getByText(/View scanned items and check what's missing/)).toBeInTheDocument();
    });
    
    fireEvent.mouseLeave(inventoryButton!);
    
    await waitFor(() => {
      expect(screen.queryByText(/View scanned items and check what's missing/)).not.toBeInTheDocument();
    });
  });

  it('should display correct device status for all states', () => {
    const testStates = [
      { state: ReaderState.DISCONNECTED, text: 'Disconnected', colorClass: 'text-red-600' },
      { state: ReaderState.CONNECTING, text: 'Connecting', colorClass: 'text-yellow-600' },
      { state: ReaderState.CONNECTED, text: 'Connected', colorClass: 'text-green-600' },
      { state: ReaderState.CONFIGURING, text: 'Configuring', colorClass: 'text-blue-600' },
      { state: ReaderState.BUSY, text: 'Scanning', colorClass: 'text-purple-600' },
      { state: ReaderState.SCANNING, text: 'Scanning', colorClass: 'text-purple-600' },
      { state: ReaderState.ERROR, text: 'Error', colorClass: 'text-red-600' }
    ];

    testStates.forEach(({ state, text, colorClass }) => {
      useDeviceStore.setState({ readerState: state });
      const { container } = render(<TabNavigation />);
      
      expect(screen.getByText(text)).toBeInTheDocument();
      const statusElement = screen.getByText(text);
      expect(statusElement).toHaveClass(colorClass);
      
      // Clean up for next iteration
      container.remove();
    });
  });

  it('should update URL hash when navigating', () => {
    const pushStateSpy = vi.spyOn(window.history, 'pushState');
    render(<TabNavigation />);
    
    const settingsButton = screen.getByText('Settings').closest('button');
    fireEvent.click(settingsButton!);
    
    expect(pushStateSpy).toHaveBeenCalledWith({ tab: 'settings' }, '', '#settings');
    pushStateSpy.mockRestore();
  });

  it('should handle browser back button navigation', () => {
    const mockSetActiveTab = vi.fn();
    useUIStore.getState().setActiveTab = mockSetActiveTab;
    
    render(<TabNavigation />);
    
    // Simulate browser back button
    const popStateEvent = new PopStateEvent('popstate', { state: { tab: 'inventory' } });
    window.dispatchEvent(popStateEvent);
    
    expect(mockSetActiveTab).toHaveBeenCalledWith('inventory');
  });

  it('should display TrakRF logo and version', () => {
    render(<TabNavigation />);
    expect(screen.getByText('TrakRF')).toBeInTheDocument();
    expect(screen.getByText('Handheld Tag Reader')).toBeInTheDocument();
    expect(screen.getByText(`v${version}`)).toBeInTheDocument();
  });

  it('should apply correct styling for dark mode', () => {
    // This test assumes dark mode class is applied at root level
    document.documentElement.classList.add('dark');
    render(<TabNavigation />);
    
    const navButtons = screen.getAllByRole('button');
    navButtons.forEach(button => {
      if (!button.classList.contains('bg-blue-600')) {
        expect(button).toHaveClass('dark:text-gray-300');
      }
    });
    
    document.documentElement.classList.remove('dark');
  });

  it('should verify Settings tooltip was updated', async () => {
    render(<TabNavigation />);
    const settingsButton = screen.getByText('Settings').closest('button');
    
    fireEvent.mouseEnter(settingsButton!);
    
    await waitFor(() => {
      expect(screen.getByText(/Configure device and application settings/)).toBeInTheDocument();
    });
    
    fireEvent.mouseLeave(settingsButton!);
  });
});