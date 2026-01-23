import { useState, useCallback, useMemo, useRef, useEffect } from 'react';
import { useDeviceStore, useTagStore, useSettingsStore, useAuthStore } from '@/stores';
import { useAssets } from '@/hooks/assets';
import { useLocations } from '@/hooks/locations';
import { ReaderState } from '@/worker/types/reader';
import { Package2, Search } from 'lucide-react';
import { ShareModal } from '@/components/ShareModal';
import type { ExportFormat } from '@/types/export';
import { useInventoryAudio } from '@/hooks/useInventoryAudio';
import { useReconciliation } from '@/hooks/useReconciliation';
import { useInventorySave } from '@/hooks/inventory/useInventorySave';
import { ConfigurationSpinner } from '@/components/ConfigurationSpinner';
import { useSortableInventory } from '@/hooks/useSortableInventory';
import { usePagination } from '@/hooks/usePagination';
import { BrowserSupportBanner } from '@/components/inventory/BrowserSupportBanner';
import { ProcessingOverlay } from '@/components/loaders/ProcessingOverlay';
import { ErrorBanner } from '@/components/banners/ErrorBanner';
import { InventoryHeader } from '@/components/inventory/InventoryHeader';
import { InventoryStats } from '@/components/inventory/InventoryStats';
import { InventoryTableContent } from '@/components/inventory/InventoryTableContent';
import { InventorySettingsPanel } from '@/components/inventory/InventorySettingsPanel';
import { LocationBar } from '@/components/inventory/LocationBar';

export default function InventoryScreen() {
  const [searchTerm, setSearchTerm] = useState('');
  const [statusFilters, setStatusFilters] = useState<Set<string>>(new Set());
  const [isSettingsPanelOpen, setIsSettingsPanelOpen] = useState(false);
  const [isShareModalOpen, setIsShareModalOpen] = useState(false);
  const [selectedExportFormat, setSelectedExportFormat] = useState<ExportFormat>('csv');
  const [isBrowserSupported, setIsBrowserSupported] = useState(true);
  const [showClearPulse, setShowClearPulse] = useState(false);


  const scrollContainerRef = useRef<HTMLDivElement>(null);

  const readerState = useDeviceStore((state) => state.readerState);
  const tags = useTagStore((state) => state.tags);
  const clearTags = useTagStore((state) => state.clearTags);
  const sortColumn = useTagStore((state) => state.sortColumn);
  const sortDirection = useTagStore((state) => state.sortDirection);
  const setSortConfig = useTagStore((state) => state.setSortConfig);
  const currentPage = useTagStore((state) => state.currentPage);
  const pageSize = useTagStore((state) => state.pageSize);
  const setCurrentPage = useTagStore((state) => state.setCurrentPage);
  const setPageSize = useTagStore((state) => state.setPageSize);
  const goToNextPage = useTagStore((state) => state.goToNextPage);
  const goToPreviousPage = useTagStore((state) => state.goToPreviousPage);
  const goToFirstPage = useTagStore((state) => state.goToFirstPage);
  const goToLastPage = useTagStore((state) => state.goToLastPage);

  const rfPower = useSettingsStore((state) => state.rfid?.transmitPower ?? 30);
  const setTransmitPower = useSettingsStore((state) => state.setTransmitPower);
  const showLeadingZeros = useSettingsStore((state) => state.showLeadingZeros);
  const setShowLeadingZeros = useSettingsStore((state) => state.setShowLeadingZeros);

  const audio = useInventoryAudio();
  const { error, setError, isProcessingCSV, fileInputRef, handleReconciliationUpload, downloadSampleReconFile } = useReconciliation();
  const { save, isSaving } = useInventorySave();

  // Load assets for tag enrichment (only when authenticated)
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  useAssets({ enabled: isAuthenticated });

  // Load locations for dropdown selection (only when authenticated)
  const { locations } = useLocations({ enabled: isAuthenticated });

  // Manual location selection state
  const [manualLocationId, setManualLocationId] = useState<number | null>(null);

  const sortedTags = useSortableInventory(tags, sortColumn, sortDirection);

  // Filter out location tags - they're used for detection, not display
  const displayableTags = useMemo(() => {
    return sortedTags.filter(tag => tag.type !== 'location');
  }, [sortedTags]);

  // Detect location from scanned location tags (strongest RSSI wins)
  const detectedLocation = useMemo(() => {
    const locationTags = tags.filter(t => t.type === 'location');
    if (locationTags.length === 0) return null;

    // Strongest RSSI wins
    const strongest = locationTags.reduce((best, current) =>
      (current.rssi ?? -120) > (best.rssi ?? -120) ? current : best
    );

    if (!strongest.locationId || !strongest.locationName) return null;

    return {
      id: strongest.locationId,
      name: strongest.locationName,
    };
  }, [tags]);

  // Detection method for display
  const detectionMethod = useMemo(() => {
    if (!detectedLocation) return null;
    return 'tag' as const; // Always 'tag' for auto-detected
  }, [detectedLocation]);

  // Resolved location = manual override OR detected
  const resolvedLocation = useMemo(() => {
    if (manualLocationId) {
      const location = locations.find(l => l.id === manualLocationId);
      return location ? { id: location.id, name: location.name } : null;
    }
    return detectedLocation;
  }, [manualLocationId, detectedLocation, locations]);

  // Display detection method
  const displayDetectionMethod = useMemo(() => {
    if (manualLocationId) return 'manual' as const;
    return detectionMethod;
  }, [manualLocationId, detectionMethod]);

  // Count of saveable assets (asset type tags only)
  const saveableCount = useMemo(() => {
    return tags.filter(t => t.type === 'asset').length;
  }, [tags]);

  const filteredTags = useMemo(() => {
    return displayableTags.filter(tag => {
      const matchesSearch = !searchTerm ||
        (tag.displayEpc || tag.epc).toLowerCase().includes(searchTerm.toLowerCase());

      // Multi-select: empty set = show all, otherwise OR logic
      const matchesStatus = statusFilters.size === 0 ||
        (statusFilters.has('Found') && tag.reconciled === true) ||
        (statusFilters.has('Missing') && tag.reconciled === false) ||
        (statusFilters.has('Not Listed') && (tag.reconciled === null || tag.reconciled === undefined));

      return matchesSearch && matchesStatus;
    });
  }, [displayableTags, searchTerm, statusFilters]);

  useEffect(() => {
    setCurrentPage(1);
  }, [searchTerm, statusFilters, setCurrentPage]);

  const { paginatedTags, startIndex, endIndex } = usePagination(filteredTags, currentPage, pageSize);

  const stats = useMemo(() => {
    const foundTags = filteredTags.filter(tag => tag.reconciled === true).length;
    const missingTags = filteredTags.filter(tag => tag.reconciled === false).length;
    const notListedTags = filteredTags.filter(tag => tag.reconciled === null || tag.reconciled === undefined).length;
    const hasReconciliation = filteredTags.some(tag => tag.reconciled !== null && tag.reconciled !== undefined);

    return {
      total: filteredTags.length,
      totalScanned: filteredTags.length,
      found: foundTags,
      missing: missingTags,
      notListed: notListedTags,
      hasReconciliation,
      saveable: saveableCount,
    };
  }, [filteredTags, saveableCount]);

  const handleSort = useCallback((column: string) => {
    if (sortColumn !== column) {
      setSortConfig(column, 'asc');
    } else if (sortDirection === 'asc') {
      setSortConfig(column, 'desc');
    } else {
      setSortConfig('timestamp', 'desc');
    }
  }, [sortColumn, sortDirection, setSortConfig]);

  const handleClearInventory = useCallback(() => {
    clearTags();
    setError(null);
  }, [clearTags, setError]);

  const handleToggleFilter = useCallback((filter: string) => {
    setStatusFilters(prev => {
      const next = new Set(prev);
      if (next.has(filter)) {
        next.delete(filter);
      } else {
        next.add(filter);
      }
      return next;
    });
  }, []);

  const handleClearFilters = useCallback(() => {
    setStatusFilters(new Set());
  }, []);

  const handleSave = useCallback(async () => {
    if (!isAuthenticated) {
      // Save current route for redirect after login (same pattern as ProtectedRoute)
      sessionStorage.setItem('redirectAfterLogin', 'inventory');
      window.location.hash = '#login';
      return;
    }

    if (!resolvedLocation) return;

    // Get saveable asset IDs (asset type tags only)
    const saveableAssets = tags
      .filter(t => t.type === 'asset' && t.assetId)
      .map(t => t.assetId!);

    if (saveableAssets.length === 0) return;

    try {
      await save({
        location_id: resolvedLocation.id,
        asset_ids: saveableAssets,
      });
      // Trigger clear button pulse animation on success
      setShowClearPulse(true);
    } catch {
      // Error handling is done in the hook with toast
    }
  }, [isAuthenticated, resolvedLocation, tags, save]);

  useEffect(() => {
    const hasBluetoothAPI = typeof navigator !== 'undefined' && !!navigator.bluetooth;
    const isMocked = typeof window !== 'undefined' && !!window.__webBluetoothBridged;
    setIsBrowserSupported(hasBluetoothAPI || isMocked);
  }, []);

  return (
    <div className="h-full flex flex-col p-2 md:p-3 space-y-2">
      <ConfigurationSpinner readerState={readerState} mode="Inventory" />
      <BrowserSupportBanner isSupported={isBrowserSupported} readerState={readerState} />

      <input
        ref={fileInputRef}
        type="file"
        accept=".csv"
        className="hidden"
        onChange={handleReconciliationUpload}
        disabled={isProcessingCSV}
      />

      <ProcessingOverlay isProcessing={isProcessingCSV} message="Processing CSV..." />
      <ErrorBanner error={error} />

      <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg flex-1 flex flex-col min-h-0">
        <InventoryHeader
          filteredCount={filteredTags.length}
          totalCount={displayableTags.length}
          searchTerm={searchTerm}
          onSearchChange={setSearchTerm}
          onDownloadSample={downloadSampleReconFile}
          onUploadCSV={() => fileInputRef.current?.click()}
          onClearInventory={handleClearInventory}
          onToggleAudio={audio.toggleSound}
          isAudioEnabled={audio.isEnabled}
          isProcessingCSV={isProcessingCSV}
          onShareFormatSelect={(format) => {
            setSelectedExportFormat(format);
            setIsShareModalOpen(true);
          }}
          hasItems={filteredTags.length > 0}
          readerState={readerState}
          onSave={handleSave}
          isSaveDisabled={isAuthenticated ? (!resolvedLocation || saveableCount === 0 || isSaving) : displayableTags.length === 0}
          isSaving={isSaving}
          saveableCount={saveableCount}
          showClearPulse={showClearPulse}
          onClearPulseEnd={() => setShowClearPulse(false)}
        />
        <LocationBar
          detectedLocation={detectedLocation}
          detectionMethod={displayDetectionMethod}
          selectedLocationId={manualLocationId}
          onLocationChange={setManualLocationId}
          locations={locations}
          isAuthenticated={isAuthenticated}
        />


        {tags.length === 0 ? (
          <div className="flex-1 flex items-center justify-center p-12">
            <div className="text-center">
              <div className="w-16 h-16 bg-gray-100 dark:bg-gray-700 rounded-lg flex items-center justify-center mx-auto mb-4">
                <Package2 className="w-8 h-8 text-gray-400 dark:text-gray-500" />
              </div>
              <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-2">
                {readerState === ReaderState.SCANNING ? 'Searching for items...' : 'No items scanned'}
              </h3>
              <p className="text-gray-500 dark:text-gray-400">
                {readerState === ReaderState.SCANNING ? 'Scan in progress, items will appear here' : 'Press and hold trigger button to start scanning'}
              </p>
            </div>
          </div>
        ) : filteredTags.length === 0 ? (
          <div className="flex-1 flex items-center justify-center p-12">
            <div className="text-center">
              <div className="w-16 h-16 bg-gray-100 dark:bg-gray-700 rounded-lg flex items-center justify-center mx-auto mb-4">
                <Search className="w-8 h-8 text-gray-400 dark:text-gray-500" />
              </div>
              <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-2">
                No items match your filters
              </h3>
              <p className="text-gray-500 dark:text-gray-400">
                Try adjusting your search or status filter
              </p>
            </div>
          </div>
        ) : (
          <InventoryTableContent
            tags={tags}
            paginatedTags={paginatedTags}
            filteredTags={filteredTags}
            sortColumn={sortColumn}
            sortDirection={sortDirection}
            onSort={handleSort}
            currentPage={currentPage}
            pageSize={pageSize}
            startIndex={startIndex}
            endIndex={endIndex}
            onPageChange={setCurrentPage}
            onNext={goToNextPage}
            onPrevious={goToPreviousPage}
            onFirstPage={goToFirstPage}
            onLastPage={goToLastPage}
            onPageSizeChange={setPageSize}
            scrollContainerRef={scrollContainerRef}
            onAssetUpdated={() => {
              // Asset enrichment runs automatically via worker/inventory subsystem
              console.log('[InventoryScreen] Asset updated, enrichment will refresh');
            }}
          />
        )}
      </div>

      <InventoryStats
        stats={stats}
        activeFilters={statusFilters}
        onToggleFilter={handleToggleFilter}
        onClearFilters={handleClearFilters}
      />

      <InventorySettingsPanel
        isOpen={isSettingsPanelOpen}
        onToggle={() => setIsSettingsPanelOpen(!isSettingsPanelOpen)}
        rfPower={rfPower}
        onRfPowerChange={setTransmitPower}
        showLeadingZeros={showLeadingZeros}
        onShowLeadingZerosChange={setShowLeadingZeros}
      />

      <ShareModal
        isOpen={isShareModalOpen}
        onClose={() => setIsShareModalOpen(false)}
        tags={filteredTags}
        reconciliationList={tags.some(t => t.reconciled !== undefined) ? tags.filter(t => t.reconciled !== null).map(t => t.displayEpc || t.epc) : null}
        selectedFormat={selectedExportFormat}
      />
    </div>
  );
}