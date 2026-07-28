import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup, act } from '@testing-library/react';
import TabNavigation from '@/components/TabNavigation';
import { useUIStore, useDeviceStore, useOrgStore } from '@/stores';
import { useAuthStore } from '@/stores/authStore';
import { ReaderState } from '@/worker/types/reader';
import { appVersion } from '@/version';

/** Set the current org's capability grants; `null` models "profile not loaded". */
function setCapabilities(capabilities: string[] | null, role = 'owner') {
  useAuthStore.setState({ isAuthenticated: true } as never);
  useOrgStore.setState({
    currentRole: role,
    currentOrg:
      capabilities === null
        ? null
        : ({
            id: 1,
            name: 'Acme',
            identifier: 'acme',
            role,
            is_entitled: true,
            subscription_enabled: true,
            subscription_expires_at: null,
            capabilities,
          } as never),
  } as never);
}

describe('TabNavigation', () => {
  beforeEach(() => {
    // Set default store states
    useUIStore.setState({ activeTab: 'scan' });
    useDeviceStore.setState({ readerState: ReaderState.DISCONNECTED });
    // Default to no org role; device-management tests opt into a role.
    // Auth/org are reset too so capability tests can't leak into the rest.
    useAuthStore.setState({ isAuthenticated: false } as never);
    useOrgStore.setState({ currentRole: null, currentOrg: null } as never);
  });

  afterEach(() => {
    cleanup();
    useAuthStore.setState({ isAuthenticated: false } as never);
    useOrgStore.setState({ currentRole: null, currentOrg: null } as never);
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

    // These set a real signed-in org with the geofence grant: Outputs is
    // capability-gated as well as role-gated, and signed-out hides it outright.
    it('should show Readers, Live feed, and Outputs sub-options for an operator', () => {
      setCapabilities(['geofence'], 'operator');
      render(<TabNavigation />);

      expect(screen.getByText('Readers')).toBeInTheDocument();
      expect(screen.getByText('Live feed')).toBeInTheDocument();
      expect(screen.getByText('Outputs')).toBeInTheDocument();
    });

    it('should show device-management sub-options for owner/admin/manager', () => {
      for (const role of ['owner', 'admin', 'manager'] as const) {
        setCapabilities(['geofence'], role);
        const { unmount } = render(<TabNavigation />);
        expect(screen.getByText('Readers')).toBeInTheDocument();
        expect(screen.getByText('Outputs')).toBeInTheDocument();
        unmount();
      }
    });

    it('should hide device-management sub-options from a viewer', () => {
      setCapabilities(['geofence'], 'viewer');
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
      setCapabilities(['geofence'], 'operator');
      render(<TabNavigation />);

      fireEvent.click(screen.getByText('Readers').closest('button')!);
      expect(mockSetActiveTab).toHaveBeenCalledWith('scan-devices');

      fireEvent.click(screen.getByText('Live feed').closest('button')!);
      expect(mockSetActiveTab).toHaveBeenCalledWith('live-reads');

      fireEvent.click(screen.getByText('Outputs').closest('button')!);
      expect(mockSetActiveTab).toHaveBeenCalledWith('output-devices');
    });
  });

  describe('capability gating (TRA-1026)', () => {
    it('renders Mustering with a lock without the grant (`locked`)', () => {
      setCapabilities(['geofence']);
      render(<TabNavigation />);

      expect(screen.getByText('Mustering')).toBeInTheDocument();
      expect(screen.getByTestId('menu-item-mustering-locked')).toBeInTheDocument();
    });

    it('shows Mustering normally with the grant', () => {
      setCapabilities(['mustering']);
      render(<TabNavigation />);

      expect(screen.getByText('Mustering')).toBeInTheDocument();
      expect(screen.queryByTestId('menu-item-mustering-locked')).not.toBeInTheDocument();
    });

    it('navigates to the mustering tab when granted', () => {
      const mockSetActiveTab = vi.fn();
      useUIStore.getState().setActiveTab = mockSetActiveTab;
      setCapabilities(['mustering']);
      render(<TabNavigation />);

      fireEvent.click(screen.getByText('Mustering').closest('button')!);
      expect(mockSetActiveTab).toHaveBeenCalledWith('mustering');
    });

    it('renders the geofence entries with a lock without the grant (`locked`)', () => {
      setCapabilities([]);
      render(<TabNavigation />);

      expect(screen.getByText('Outputs')).toBeInTheDocument();
      expect(screen.getByText('Geofence defaults')).toBeInTheDocument();
      expect(screen.getByTestId('menu-item-output-devices-locked')).toBeInTheDocument();
      expect(screen.getByTestId('menu-item-org-geofence-defaults-locked')).toBeInTheDocument();
    });

    it('drops the lock on the geofence entries once granted', () => {
      setCapabilities(['geofence']);
      render(<TabNavigation />);

      expect(screen.getByText('Outputs')).toBeInTheDocument();
      expect(screen.queryByTestId('menu-item-output-devices-locked')).not.toBeInTheDocument();
      expect(
        screen.queryByTestId('menu-item-org-geofence-defaults-locked')
      ).not.toBeInTheDocument();
    });

    it('still routes to the surface when a locked entry is clicked (upsell resolves it)', () => {
      const mockSetActiveTab = vi.fn();
      useUIStore.getState().setActiveTab = mockSetActiveTab;
      setCapabilities([]);
      render(<TabNavigation />);

      fireEvent.click(screen.getByText('Outputs').closest('button')!);
      expect(mockSetActiveTab).toHaveBeenCalledWith('output-devices');
    });

    it('renders no gated entry at all while the profile is loading (fail-closed)', () => {
      setCapabilities(null);
      render(<TabNavigation />);

      // Not even the locked teaser — a lock that appears mid-load reads as a
      // downgrade to anyone who actually holds the grant.
      expect(screen.queryByText('Mustering')).not.toBeInTheDocument();
      expect(screen.queryByText('Outputs')).not.toBeInTheDocument();
      expect(screen.queryByText('Geofence defaults')).not.toBeInTheDocument();
      // Ungated entries are unaffected.
      expect(screen.getByText('Readers')).toBeInTheDocument();
    });

    // Signed out there is no org, so no capability question to answer. Showing
    // a lock would offer "contact us to enable it for your organization" to a
    // visitor with no organization. Mustering used to escape this because it is
    // the only gated entry without a role condition wrapping it.
    it('renders no gated entry when signed out', () => {
      useAuthStore.setState({ isAuthenticated: false } as never);
      useOrgStore.setState({ currentRole: null, currentOrg: null } as never);
      render(<TabNavigation />);

      expect(screen.queryByText('Mustering')).not.toBeInTheDocument();
      expect(screen.queryByText('Outputs')).not.toBeInTheDocument();
      expect(screen.queryByText('Geofence defaults')).not.toBeInTheDocument();
      // Ungated entries are untouched — Scan and Locate work without an account.
      expect(screen.getByText('Scan')).toBeInTheDocument();
      expect(screen.getByText('Locate')).toBeInTheDocument();
    });

    it('keeps Geofence defaults admin-only even when granted', () => {
      setCapabilities(['geofence'], 'manager');
      render(<TabNavigation />);

      expect(screen.getByText('Outputs')).toBeInTheDocument();
      expect(screen.queryByText('Geofence defaults')).not.toBeInTheDocument();
    });

    it('updates nav state when the org switches from granted to ungated', () => {
      setCapabilities(['mustering', 'geofence']);
      const { rerender } = render(<TabNavigation />);
      expect(screen.queryByTestId('menu-item-mustering-locked')).not.toBeInTheDocument();
      expect(screen.queryByTestId('menu-item-output-devices-locked')).not.toBeInTheDocument();

      act(() => setCapabilities([]));
      rerender(<TabNavigation />);
      expect(screen.getByTestId('menu-item-mustering-locked')).toBeInTheDocument();
      expect(screen.getByTestId('menu-item-output-devices-locked')).toBeInTheDocument();
    });

    it('hides Kits entirely without the grant (`absent`)', () => {
      setCapabilities(['geofence']);
      render(<TabNavigation />);

      // `absent` means no trace at all — not a lock, not a disabled entry.
      expect(screen.queryByText('Kits')).not.toBeInTheDocument();
      expect(screen.queryByTestId('menu-item-kits')).not.toBeInTheDocument();
      expect(screen.queryByTestId('menu-item-kits-locked')).not.toBeInTheDocument();
    });

    it('shows Kits with the kitting grant, unlocked', () => {
      setCapabilities(['kitting']);
      render(<TabNavigation />);

      expect(screen.getByTestId('menu-item-kits')).toBeInTheDocument();
      expect(screen.queryByTestId('menu-item-kits-locked')).not.toBeInTheDocument();
    });

    it('hides Kits when signed out rather than teasing it', () => {
      setCapabilities(null);
      render(<TabNavigation />);

      expect(screen.queryByTestId('menu-item-kits')).not.toBeInTheDocument();
    });

    it('navigates to the kits tab when granted', () => {
      const mockSetActiveTab = vi.fn();
      useUIStore.getState().setActiveTab = mockSetActiveTab;
      setCapabilities(['kitting']);
      render(<TabNavigation />);

      fireEvent.click(screen.getByTestId('menu-item-kits'));
      expect(mockSetActiveTab).toHaveBeenCalledWith('kits');
    });
  });
});
