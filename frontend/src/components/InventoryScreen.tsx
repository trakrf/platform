import { useState, useCallback, useMemo, useRef, useEffect } from 'react';
import { useDeviceStore, useTagStore, useSettingsStore } from '@/stores';
import { ReaderState } from '@/worker/types/reader';
import { Package2, Search } from 'lucide-react';
import { ShareModal } from '@/components/ShareModal';
import type { ExportFormat } from '@/types/export';
import { useInventoryAudio } from '@/hooks/useInventoryAudio';
import { useReconciliation } from '@/hooks/useReconciliation';
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

export default function InventoryScreen() {
  const [searchTerm, setSearchTerm] = useState('');
  const [statusFilter, setStatusFilter] = useState('All Status');
  const [isSettingsPanelOpen, setIsSettingsPanelOpen] = useState(false);
  const [isShareModalOpen, setIsShareModalOpen] = useState(false);
  const [selectedExportFormat, setSelectedExportFormat] = useState<ExportFormat>('csv');
  const [isBrowserSupported, setIsBrowserSupported] = useState(true);


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

  // TODO: REMOVE - Sample data injection for testing asset enrichment
  // These identifiers match existing assets in the system
  useEffect(() => {
    const addTag = useTagStore.getState().addTag;

    // Simulate RFID reads for existing assets
    // These EPCs match actual assets: 300833B2DDD9014000000001, 300833B2DDD9014000000002, 300833B2DDD9014000000003
    setTimeout(() => {
      addTag({
        epc: '300833B2DDD9014000000001',
        rssi: -45,
        count: 1,
        antenna: 1,
        timestamp: Date.now(),
        source: 'rfid',
      });
    }, 500);

    setTimeout(() => {
      addTag({
        epc: '300833B2DDD9014000000002',
        rssi: -52,
        count: 1,
        antenna: 1,
        timestamp: Date.now(),
        source: 'rfid',
      });
    }, 1000);

    setTimeout(() => {
      addTag({
        epc: '300833B2DDD9014000000003',
        rssi: -38,
        count: 1,
        antenna: 1,
        timestamp: Date.now(),
        source: 'rfid',
      });
    }, 1500);

    // Add one tag without matching asset
    setTimeout(() => {
      addTag({
        epc: '300833B2DDD9014099999999',
        rssi: -60,
        count: 1,
        antenna: 1,
        timestamp: Date.now(),
        source: 'rfid',
      });
    }, 2000);
  }, []); // Run only once on mount

  const sortedTags = useSortableInventory(tags, sortColumn, sortDirection);

  const filteredTags = useMemo(() => {
    return sortedTags.filter(tag => {
      const matchesSearch = !searchTerm ||
        (tag.displayEpc || tag.epc).toLowerCase().includes(searchTerm.toLowerCase());

      const matchesStatus = statusFilter === 'All Status' ||
        (statusFilter === 'Found' && tag.reconciled === true) ||
        (statusFilter === 'Missing' && tag.reconciled === false) ||
        (statusFilter === 'Not Listed' && (tag.reconciled === null || tag.reconciled === undefined));

      return matchesSearch && matchesStatus;
    });
  }, [sortedTags, searchTerm, statusFilter]);

  useEffect(() => {
    setCurrentPage(1);
  }, [searchTerm, statusFilter, setCurrentPage]);

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
      hasReconciliation
    };
  }, [filteredTags]);

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

  useEffect(() => {
    const hasBluetoothAPI = typeof navigator !== 'undefined' && !!navigator.bluetooth;
    const isMocked = typeof window !== 'undefined' && !!window.__webBluetoothMocked;
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
          totalCount={sortedTags.length}
          searchTerm={searchTerm}
          onSearchChange={setSearchTerm}
          statusFilter={statusFilter}
          onStatusFilterChange={setStatusFilter}
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
          />
        )}
      </div>

      <InventoryStats stats={stats} />

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