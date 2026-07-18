import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import TabNavigation from '@/components/TabNavigation';
import { useUIStore, useDeviceStore, useOrgStore } from '@/stores';
import { ReaderState } from '@/worker/types/reader';
import { appVersion } from '@/version';

describe('TabNavigation', () => {
  beforeEach(() => {
    // Set default store states
    useUIStore.setState({ activeTab: 'scan' });
    useDeviceStore.setState({ readerState: ReaderState.DISCONNECTED });
    // Default to no org role; device-management tests opt into a role.
    useOrgStore.setState({ currentRole: null });
  });

  afterEach(() => {
    cleanup();
    useOrgStore.setState({ currentRole: null });
  });

  it('should render all navigation items with correct labels', () => {
    render(<TabNavigation />);
    expect(screen.getByText('Scan')).toBeInTheDocument();
    expect(screen.getByText('Locate')).toBeInTheDocument();
    expect(screen.getByText('Settings')).toBeInTheDocument();
    expect(screen.getByText('Help')).toBeInTheDocument();
  });

  it('should highlight active tab', () => {
    useUIStore.setState({ activeTab: 'scan' });
    render(<TabNavigation />);

    const scanButton = screen.getByText('Scan').closest('button');
    expect(scanButton).toHaveClass('bg-blue-600', 'text-white');

    const settingsButton = screen.getByText('Settings').closest('button');
    expect(settingsButton).not.toHaveClass('bg-blue-600');
  });

  it('should navigate when clicking nav items', () => {
    const mockSetActiveTab = vi.fn();
    useUIStore.getState().setActiveTab = mockSetActiveTab;

    render(<TabNavigation />);
    const scanButton = screen.getByText('Scan').closest('button');
    fireEvent.click(scanButton!);

    expect(mockSetActiveTab).toHaveBeenCalledWith('scan');
  });

  it('should show tooltips on hover', async () => {
    render(<TabNavigation />);
    const scanButton = screen.getByText('Scan').closest('button');

    fireEvent.mouseEnter(scanButton!);

    await waitFor(() => {
      expect(screen.getByText(/Read tags and check what's missing/)).toBeInTheDocument();
    });

    fireEvent.mouseLeave(scanButton!);

    await waitFor(() => {
      expect(screen.queryByText(/Read tags and check what's missing/)).not.toBeInTheDocument();
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
    const popStateEvent = new PopStateEvent('popstate', { state: { tab: 'scan' } });
    window.dispatchEvent(popStateEvent);

    expect(mockSetActiveTab).toHaveBeenCalledWith('scan');
  });

  it('should display TrakRF logo and version', () => {
    render(<TabNavigation />);
    expect(screen.getByText('TrakRF')).toBeInTheDocument();
    expect(screen.getByText('Handheld Tag Reader')).toBeInTheDocument();
    expect(screen.getByText(appVersion)).toBeInTheDocument();
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

  describe('device management under Settings (TRA-930)', () => {
    it('should not show Scan Devices, Output Devices, or Live Reads as top-level items', () => {
      useOrgStore.setState({ currentRole: 'owner' });
      render(<TabNavigation />);

      // Old top-level labels are gone; replaced by Settings sub-options.
      expect(screen.queryByText('Scan Devices')).not.toBeInTheDocument();
      expect(screen.queryByText('Output Devices')).not.toBeInTheDocument();
      expect(screen.queryByText('Live Reads')).not.toBeInTheDocument();
    });

    it('should show Readers, Live feed, and Outputs sub-options for an operator', () => {
      useOrgStore.setState({ currentRole: 'operator' });
      render(<TabNavigation />);

      expect(screen.getByText('Readers')).toBeInTheDocument();
      expect(screen.getByText('Live feed')).toBeInTheDocument();
      expect(screen.getByText('Outputs')).toBeInTheDocument();
    });

    it('should show device-management sub-options for owner/admin/manager', () => {
      for (const role of ['owner', 'admin', 'manager'] as const) {
        useOrgStore.setState({ currentRole: role });
        const { unmount } = render(<TabNavigation />);
        expect(screen.getByText('Readers')).toBeInTheDocument();
        expect(screen.getByText('Outputs')).toBeInTheDocument();
        unmount();
      }
    });

    it('should hide device-management sub-options from a viewer', () => {
      useOrgStore.setState({ currentRole: 'viewer' });
      render(<TabNavigation />);

      expect(screen.queryByText('Readers')).not.toBeInTheDocument();
      expect(screen.queryByText('Live feed')).not.toBeInTheDocument();
      expect(screen.queryByText('Outputs')).not.toBeInTheDocument();
      // The Settings entry itself remains visible to everyone.
      expect(screen.getByText('Settings')).toBeInTheDocument();
    });

    it('should navigate to the correct tab when clicking each sub-option', () => {
      const mockSetActiveTab = vi.fn();
      useUIStore.getState().setActiveTab = mockSetActiveTab;
      useOrgStore.setState({ currentRole: 'operator' });
      render(<TabNavigation />);

      fireEvent.click(screen.getByText('Readers').closest('button')!);
      expect(mockSetActiveTab).toHaveBeenCalledWith('scan-devices');

      fireEvent.click(screen.getByText('Live feed').closest('button')!);
      expect(mockSetActiveTab).toHaveBeenCalledWith('live-reads');

      fireEvent.click(screen.getByText('Outputs').closest('button')!);
      expect(mockSetActiveTab).toHaveBeenCalledWith('output-devices');
    });
  });
});
