import { useCallback, useRef, useEffect, useMemo } from 'react';
import { ChevronRight, ChevronDown, Building2, MapPin } from 'lucide-react';
import { useLocationStore } from '@/stores/locations/locationStore';
import type { Location } from '@/types/locations';

export interface LocationTreePanelProps {
  onSelect: (locationId: number) => void;
  selectedId: number | null;
  searchTerm?: string;
  className?: string;
}

interface TreeNodeProps {
  location: Location;
  depth: number;
  onSelect: (locationId: number) => void;
  selectedId: number | null;
  visibleIds: Set<number> | null;
  focusedId: number | null;
  onFocusChange: (id: number) => void;
}

function TreeNode({
  location,
  depth,
  onSelect,
  selectedId,
  visibleIds,
  focusedId,
  onFocusChange,
}: TreeNodeProps) {
  const getChildren = useLocationStore((state) => state.getChildren);
  const expandedNodeIds = useLocationStore((state) => state.expandedNodeIds);
  const toggleNodeExpanded = useLocationStore((state) => state.toggleNodeExpanded);

  const children = getChildren(location.id);
  const hasChildren = children.length > 0;
  const isExpanded = expandedNodeIds.has(location.id);
  const isSelected = selectedId === location.id;
  const isFocused = focusedId === location.id;
  const isRoot = location.parent_location_id === null;
  const Icon = isRoot ? Building2 : MapPin;

  // If filtering, skip nodes not in the visible set
  if (visibleIds && !visibleIds.has(location.id)) {
    return null;
  }

  const handleChevronClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (hasChildren) {
      toggleNodeExpanded(location.id);
    }
  };

  const handleNodeClick = () => {
    onSelect(location.id);
    onFocusChange(location.id);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onSelect(location.id);
    }
  };

  return (
    <div role="treeitem" aria-expanded={hasChildren ? isExpanded : undefined}>
      <div
        onClick={handleNodeClick}
        onKeyDown={handleKeyDown}
        tabIndex={isFocused ? 0 : -1}
        data-location-id={location.id}
        className={`
          flex items-center gap-2 py-2 px-3 rounded-lg cursor-pointer transition-colors outline-none
          hover:bg-blue-50 dark:hover:bg-blue-900/20
          focus:ring-2 focus:ring-blue-500 focus:ring-inset
          ${isSelected ? 'bg-blue-100 dark:bg-blue-900/40 border border-blue-300 dark:border-blue-700' : ''}
        `}
        style={{ paddingLeft: `${depth * 1.5 + 0.75}rem` }}
      >
        <button
          onClick={handleChevronClick}
          className="flex-shrink-0 w-5 h-5 flex items-center justify-center"
          aria-label={hasChildren ? (isExpanded ? 'Collapse' : 'Expand') : undefined}
          tabIndex={-1}
        >
          {hasChildren ? (
            isExpanded ? (
              <ChevronDown className="h-4 w-4 text-gray-500 dark:text-gray-400" />
            ) : (
              <ChevronRight className="h-4 w-4 text-gray-500 dark:text-gray-400" />
            )
          ) : (
            <span className="w-4 h-4" />
          )}
        </button>

        <Icon className="h-5 w-5 text-gray-500 dark:text-gray-400 flex-shrink-0" />

        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-gray-900 dark:text-white truncate">
              {location.identifier}
            </span>
            <span className="text-xs text-gray-500 dark:text-gray-400 truncate">
              {location.name}
            </span>
          </div>
        </div>

        <span
          className={`
            inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium flex-shrink-0
            ${
              location.is_active
                ? 'bg-green-50 text-green-700 border border-green-200 dark:bg-green-900/20 dark:text-green-400 dark:border-green-800'
                : 'bg-gray-50 text-gray-700 border border-gray-200 dark:bg-gray-900/20 dark:text-gray-400 dark:border-gray-800'
            }
          `}
        >
          {location.is_active ? 'Active' : 'Inactive'}
        </span>
      </div>

      {hasChildren && isExpanded && (
        <div role="group">
          {children.map((child) => (
            <TreeNode
              key={child.id}
              location={child}
              depth={depth + 1}
              onSelect={onSelect}
              selectedId={selectedId}
              visibleIds={visibleIds}
              focusedId={focusedId}
              onFocusChange={onFocusChange}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export function LocationTreePanel({
  onSelect,
  selectedId,
  searchTerm = '',
  className = '',
}: LocationTreePanelProps) {
  const getRootLocations = useLocationStore((state) => state.getRootLocations);
  const getChildren = useLocationStore((state) => state.getChildren);
  const getAncestors = useLocationStore((state) => state.getAncestors);
  const getLocationById = useLocationStore((state) => state.getLocationById);
  const expandedNodeIds = useLocationStore((state) => state.expandedNodeIds);
  const toggleNodeExpanded = useLocationStore((state) => state.toggleNodeExpanded);

  const rootLocations = getRootLocations();
  const containerRef = useRef<HTMLDivElement>(null);
  const focusedIdRef = useRef<number | null>(selectedId);

  // Compute visible IDs when filtering by search term
  const visibleIds = useMemo(() => {
    if (!searchTerm.trim()) return null;

    const term = searchTerm.toLowerCase();
    const visible = new Set<number>();

    const checkLocation = (location: Location) => {
      const matches =
        location.identifier.toLowerCase().includes(term) ||
        location.name.toLowerCase().includes(term);

      if (matches) {
        visible.add(location.id);
        // Add all ancestors
        const ancestors = getAncestors(location.id);
        for (const ancestor of ancestors) {
          visible.add(ancestor.id);
        }
      }

      // Check children recursively
      const children = getChildren(location.id);
      for (const child of children) {
        checkLocation(child);
      }
    };

    for (const root of rootLocations) {
      checkLocation(root);
    }

    return visible;
  }, [searchTerm, rootLocations, getAncestors, getChildren]);

  // Build flat list of visible node IDs for keyboard navigation
  const flatVisibleIds = useMemo(() => {
    const ids: number[] = [];

    const collectVisibleIds = (locations: Location[], depth: number) => {
      for (const location of locations) {
        if (visibleIds && !visibleIds.has(location.id)) continue;
        ids.push(location.id);

        if (expandedNodeIds.has(location.id)) {
          const children = getChildren(location.id);
          collectVisibleIds(children, depth + 1);
        }
      }
    };

    collectVisibleIds(rootLocations, 0);
    return ids;
  }, [rootLocations, expandedNodeIds, visibleIds, getChildren]);

  const handleFocusChange = useCallback((id: number) => {
    focusedIdRef.current = id;
  }, []);

  const focusNode = useCallback((id: number) => {
    focusedIdRef.current = id;
    const element = containerRef.current?.querySelector(`[data-location-id="${id}"]`);
    if (element instanceof HTMLElement) {
      element.focus();
    }
  }, []);

  // Keyboard navigation handler
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      const currentIndex = flatVisibleIds.indexOf(focusedIdRef.current ?? -1);
      if (currentIndex === -1 && flatVisibleIds.length > 0) {
        focusNode(flatVisibleIds[0]);
        return;
      }

      const currentId = focusedIdRef.current;
      if (!currentId) return;

      const location = getLocationById(currentId);
      if (!location) return;

      const children = getChildren(currentId);
      const hasChildren = children.length > 0;
      const isExpanded = expandedNodeIds.has(currentId);

      switch (e.key) {
        case 'ArrowDown':
          e.preventDefault();
          if (currentIndex < flatVisibleIds.length - 1) {
            focusNode(flatVisibleIds[currentIndex + 1]);
          }
          break;
        case 'ArrowUp':
          e.preventDefault();
          if (currentIndex > 0) {
            focusNode(flatVisibleIds[currentIndex - 1]);
          }
          break;
        case 'ArrowRight':
          e.preventDefault();
          if (hasChildren && !isExpanded) {
            toggleNodeExpanded(currentId);
          } else if (hasChildren && isExpanded && children.length > 0) {
            // Move to first child if expanded
            focusNode(children[0].id);
          }
          break;
        case 'ArrowLeft':
          e.preventDefault();
          if (hasChildren && isExpanded) {
            toggleNodeExpanded(currentId);
          } else if (location.parent_location_id !== null) {
            // Move to parent
            focusNode(location.parent_location_id);
          }
          break;
        case 'Enter':
        case ' ':
          e.preventDefault();
          onSelect(currentId);
          break;
      }
    },
    [flatVisibleIds, focusNode, getLocationById, getChildren, expandedNodeIds, toggleNodeExpanded, onSelect]
  );

  // Set initial focus when component mounts or selectedId changes
  useEffect(() => {
    if (selectedId !== null) {
      focusedIdRef.current = selectedId;
    } else if (flatVisibleIds.length > 0) {
      focusedIdRef.current = flatVisibleIds[0];
    }
  }, [selectedId, flatVisibleIds]);

  if (rootLocations.length === 0) {
    return (
      <div className={`p-8 text-center text-gray-500 dark:text-gray-400 ${className}`}>
        <Building2 className="h-12 w-12 mx-auto mb-3 opacity-50" />
        <p className="text-sm">No locations found.</p>
        <p className="text-xs mt-1">Create a root location to get started.</p>
      </div>
    );
  }

  if (visibleIds && visibleIds.size === 0) {
    return (
      <div className={`p-8 text-center text-gray-500 dark:text-gray-400 ${className}`}>
        <MapPin className="h-12 w-12 mx-auto mb-3 opacity-50" />
        <p className="text-sm">No matching locations.</p>
        <p className="text-xs mt-1">Try a different search term.</p>
      </div>
    );
  }

  return (
    <div
      ref={containerRef}
      role="tree"
      aria-label="Location hierarchy"
      className={`space-y-1 overflow-y-auto ${className}`}
      onKeyDown={handleKeyDown}
    >
      {rootLocations.map((root) => (
        <TreeNode
          key={root.id}
          location={root}
          depth={0}
          onSelect={onSelect}
          selectedId={selectedId}
          visibleIds={visibleIds}
          focusedId={focusedIdRef.current}
          onFocusChange={handleFocusChange}
        />
      ))}
    </div>
  );
}
