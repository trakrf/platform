import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import HomeScreen from '@/components/HomeScreen';
import { useUIStore } from '@/stores';

describe('HomeScreen Navigation', () => {
  const mockSetActiveTab = vi.fn();

  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    // Reset mocks and store state
    mockSetActiveTab.mockClear();
    useUIStore.setState({ activeTab: 'home' });
    // Mock setActiveTab
    useUIStore.getState().setActiveTab = mockSetActiveTab;
  });

  it('should render all navigation cards', () => {
    render(<HomeScreen />);
    expect(screen.getByText('Inventory')).toBeInTheDocument();
    expect(screen.getByText('Locate')).toBeInTheDocument();
    expect(screen.getByText('Barcode')).toBeInTheDocument();
  });

  it('should navigate to inventory when Inventory card is clicked', () => {
    render(<HomeScreen />);
    const inventoryCard = screen.getByText('Inventory').closest('button');
    fireEvent.click(inventoryCard!);
    expect(mockSetActiveTab).toHaveBeenCalledWith('inventory');
  });

  it('should navigate to locate when Locate card is clicked', () => {
    render(<HomeScreen />);
    const locateCard = screen.getByText('Locate').closest('button');
    fireEvent.click(locateCard!);
    expect(mockSetActiveTab).toHaveBeenCalledWith('locate');
  });

  it('should navigate to barcode when Barcode card is clicked', () => {
    render(<HomeScreen />);
    const barcodeCard = screen.getByText('Barcode').closest('button');
    fireEvent.click(barcodeCard!);
    expect(mockSetActiveTab).toHaveBeenCalledWith('barcode');
  });

  it('should update browser history when navigating', () => {
    const pushStateSpy = vi.spyOn(window.history, 'pushState');
    render(<HomeScreen />);
    
    const inventoryCard = screen.getByText('Inventory').closest('button');
    fireEvent.click(inventoryCard!);
    
    expect(pushStateSpy).toHaveBeenCalledWith({ tab: 'inventory' }, '', '#inventory');
    pushStateSpy.mockRestore();
  });

  it('should render demo video section', () => {
    render(<HomeScreen />);
    expect(screen.getByText('Watch Demo')).toBeInTheDocument();
    expect(screen.getByText('Learn how to use the RFID scanner')).toBeInTheDocument();
  });
});