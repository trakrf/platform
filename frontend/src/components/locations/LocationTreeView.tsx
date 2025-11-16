import { useState } from 'react';
import { ChevronRight, ChevronDown, Building2, MapPin, Pencil, Trash2 } from 'lucide-react';
import { useLocationStore } from '@/stores/locations/locationStore';
import type { Location } from '@/types/locations';

interface LocationTreeViewProps {
  onLocationClick?: (location: Location) => void;
  onEdit?: (location: Location) => void;
  onDelete?: (location: Location) => void;
  selectedLocationId?: number | null;
  className?: string;
}

interface TreeNodeProps {
  location: Location;
  depth: number;
  onLocationClick?: (location: Location) => void;
  onEdit?: (location: Location) => void;
  onDelete?: (location: Location) => void;
  selectedLocationId?: number | null;
}

function TreeNode({
  location,
  depth,
  onLocationClick,
  onEdit,
  onDelete,
  selectedLocationId,
}: TreeNodeProps) {
  const [isExpanded, setIsExpanded] = useState(true);
  const getChildren = useLocationStore((state) => state.getChildren);
  const children = getChildren(location.id);
  const hasChildren = children.length > 0;
  const isSelected = selectedLocationId === location.id;
  const isRoot = location.parent_location_id === null;
  const Icon = isRoot ? Building2 : MapPin;

  const handleToggle = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (hasChildren) {
      setIsExpanded(!isExpanded);
    }
  };

  const handleClick = () => {
    onLocationClick?.(location);
  };

  const handleEdit = (e: React.MouseEvent) => {
    e.stopPropagation();
    onEdit?.(location);
  };

  const handleDelete = (e: React.MouseEvent) => {
    e.stopPropagation();
    onDelete?.(location);
  };

  return (
    <div>
      <div
        onClick={handleClick}
        className={`
          flex items-center gap-2 py-2 px-3 rounded-lg cursor-pointer transition-colors
          hover:bg-blue-50 dark:hover:bg-blue-900/20
          ${isSelected ? 'bg-blue-100 dark:bg-blue-900/40 border border-blue-300 dark:border-blue-700' : ''}
        `}
        style={{ paddingLeft: `${depth * 1.5 + 0.75}rem` }}
      >
        <button
          onClick={handleToggle}
          className="flex-shrink-0 w-5 h-5 flex items-center justify-center"
          aria-label={hasChildren ? (isExpanded ? 'Collapse' : 'Expand') : undefined}
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

        <div className="flex items-center gap-1 flex-shrink-0">
          <button
            onClick={handleEdit}
            className="p-1 text-blue-600 hover:bg-blue-100 dark:text-blue-400 dark:hover:bg-blue-900/40 rounded transition-colors"
            aria-label={`Edit ${location.identifier}`}
          >
            <Pencil className="h-4 w-4" />
          </button>
          <button
            onClick={handleDelete}
            className="p-1 text-red-600 hover:bg-red-100 dark:text-red-400 dark:hover:bg-red-900/40 rounded transition-colors"
            aria-label={`Delete ${location.identifier}`}
          >
            <Trash2 className="h-4 w-4" />
          </button>
        </div>
      </div>

      {hasChildren && isExpanded && (
        <div>
          {children.map((child) => (
            <TreeNode
              key={child.id}
              location={child}
              depth={depth + 1}
              onLocationClick={onLocationClick}
              onEdit={onEdit}
              onDelete={onDelete}
              selectedLocationId={selectedLocationId}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export function LocationTreeView({
  onLocationClick,
  onEdit,
  onDelete,
  selectedLocationId,
  className = '',
}: LocationTreeViewProps) {
  const getRootLocations = useLocationStore((state) => state.getRootLocations);
  const rootLocations = getRootLocations();

  if (rootLocations.length === 0) {
    return (
      <div className={`p-8 text-center text-gray-500 dark:text-gray-400 ${className}`}>
        <Building2 className="h-12 w-12 mx-auto mb-3 opacity-50" />
        <p className="text-sm">No locations found. Create a root location to get started.</p>
      </div>
    );
  }

  return (
    <div className={`space-y-1 ${className}`}>
      {rootLocations.map((root) => (
        <TreeNode
          key={root.id}
          location={root}
          depth={0}
          onLocationClick={onLocationClick}
          onEdit={onEdit}
          onDelete={onDelete}
          selectedLocationId={selectedLocationId}
        />
      ))}
    </div>
  );
}
