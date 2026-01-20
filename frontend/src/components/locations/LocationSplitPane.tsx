import { useCallback } from 'react';
import { SplitPane, Pane } from 'react-split-pane';
import 'react-split-pane/styles.css';
import { LocationTreePanel } from './LocationTreePanel';
import { LocationDetailsPanel } from './LocationDetailsPanel';
import { useLocationStore } from '@/stores/locations/locationStore';

export interface LocationSplitPaneProps {
  searchTerm?: string;
  onEdit: (id: number) => void;
  onMove: (id: number) => void;
  onDelete: (id: number) => void;
  className?: string;
}

export function LocationSplitPane({
  searchTerm = '',
  onEdit,
  onMove,
  onDelete,
  className = '',
}: LocationSplitPaneProps) {
  const selectedLocationId = useLocationStore((state) => state.selectedLocationId);
  const setSelectedLocation = useLocationStore((state) => state.setSelectedLocation);
  const treePanelWidth = useLocationStore((state) => state.treePanelWidth);
  const setTreePanelWidth = useLocationStore((state) => state.setTreePanelWidth);
  const expandToLocation = useLocationStore((state) => state.expandToLocation);

  const handleSelect = useCallback(
    (locationId: number) => {
      setSelectedLocation(locationId);
      expandToLocation(locationId);
    },
    [setSelectedLocation, expandToLocation]
  );

  const handleChildClick = useCallback(
    (locationId: number) => {
      setSelectedLocation(locationId);
      expandToLocation(locationId);
    },
    [setSelectedLocation, expandToLocation]
  );

  const handleResizeEnd = useCallback(
    (sizes: number[]) => {
      if (sizes[0]) {
        setTreePanelWidth(sizes[0]);
      }
    },
    [setTreePanelWidth]
  );

  return (
    <div className={`h-full ${className}`}>
      <SplitPane
        direction="horizontal"
        onResizeEnd={handleResizeEnd}
        dividerClassName="bg-gray-200 dark:bg-gray-700 hover:bg-blue-400 dark:hover:bg-blue-500 transition-colors"
        dividerSize={4}
      >
        <Pane
          defaultSize={treePanelWidth}
          minSize={200}
          maxSize={400}
          className="bg-white dark:bg-gray-900"
        >
          <div className="h-full overflow-hidden border-r border-gray-200 dark:border-gray-700">
            <LocationTreePanel
              onSelect={handleSelect}
              selectedId={selectedLocationId}
              searchTerm={searchTerm}
              className="h-full p-2"
            />
          </div>
        </Pane>
        <Pane className="bg-white dark:bg-gray-900">
          <LocationDetailsPanel
            locationId={selectedLocationId}
            onEdit={onEdit}
            onMove={onMove}
            onDelete={onDelete}
            onChildClick={handleChildClick}
            className="h-full"
          />
        </Pane>
      </SplitPane>
    </div>
  );
}
