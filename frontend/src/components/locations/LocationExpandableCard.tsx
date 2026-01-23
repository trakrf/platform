import { ChevronRight, ChevronDown, Building2, MapPin, Pencil, ArrowRightLeft, Trash2, Plus } from 'lucide-react';
import { useLocationStore } from '@/stores/locations/locationStore';
import { LocationBreadcrumb } from './LocationBreadcrumb';
import type { Location } from '@/types/locations';

export interface LocationExpandableCardProps {
  location: Location;
  depth?: number;
  onEdit: (id: number) => void;
  onMove: (id: number) => void;
  onDelete: (id: number) => void;
  onAddChild?: (parentId: number) => void;
  searchTerm?: string;
}

export function LocationExpandableCard({
  location,
  depth = 0,
  onEdit,
  onMove,
  onDelete,
  onAddChild,
  searchTerm = '',
}: LocationExpandableCardProps) {
  const expandedCardIds = useLocationStore((state) => state.expandedCardIds);
  const toggleCardExpanded = useLocationStore((state) => state.toggleCardExpanded);
  const getChildren = useLocationStore((state) => state.getChildren);
  const getDescendants = useLocationStore((state) => state.getDescendants);

  const isExpanded = expandedCardIds.has(location.id);
  const children = getChildren(location.id);
  const descendants = getDescendants(location.id);
  const hasChildren = children.length > 0;
  const isRoot = location.parent_location_id === null;
  const Icon = isRoot ? Building2 : MapPin;

  // Filter visibility when searching
  const isVisible = (() => {
    if (!searchTerm.trim()) return true;

    const term = searchTerm.toLowerCase();
    const matches =
      location.identifier.toLowerCase().includes(term) ||
      location.name.toLowerCase().includes(term);

    if (matches) return true;

    // Check if any descendant matches
    const hasMatchingDescendant = descendants.some(
      (d) =>
        d.identifier.toLowerCase().includes(term) ||
        d.name.toLowerCase().includes(term)
    );

    if (hasMatchingDescendant) return true;

    // Check if this location is an ancestor of a matching location
    // (This is handled by parent components, so we're visible if we got here)
    return false;
  })();

  // Check if children should be visible when filtering
  const getVisibleChildren = () => {
    if (!searchTerm.trim()) return children;

    const term = searchTerm.toLowerCase();
    return children.filter((child) => {
      const childMatches =
        child.identifier.toLowerCase().includes(term) ||
        child.name.toLowerCase().includes(term);

      if (childMatches) return true;

      // Check if any of child's descendants match
      const childDescendants = getDescendants(child.id);
      return childDescendants.some(
        (d) =>
          d.identifier.toLowerCase().includes(term) ||
          d.name.toLowerCase().includes(term)
      );
    });
  };

  if (!isVisible) return null;

  const visibleChildren = getVisibleChildren();

  const handleToggle = () => {
    toggleCardExpanded(location.id);
  };

  const handleEdit = (e: React.MouseEvent) => {
    e.stopPropagation();
    onEdit(location.id);
  };

  const handleMove = (e: React.MouseEvent) => {
    e.stopPropagation();
    onMove(location.id);
  };

  const handleDelete = (e: React.MouseEvent) => {
    e.stopPropagation();
    onDelete(location.id);
  };

  const handleAddChild = (e: React.MouseEvent) => {
    e.stopPropagation();
    onAddChild?.(location.id);
  };

  return (
    <div
      className="border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden"
      style={{ marginLeft: depth > 0 ? '1rem' : 0 }}
      data-testid="location-expandable-card"
      data-location-id={location.id}
    >
      {/* Header - always visible */}
      <button
        onClick={handleToggle}
        className={`
          w-full flex items-center gap-3 p-3 text-left transition-colors
          hover:bg-gray-50 dark:hover:bg-gray-800
          ${isExpanded ? 'bg-gray-50 dark:bg-gray-800' : ''}
        `}
        aria-expanded={isExpanded}
      >
        {/* Chevron */}
        <span className="flex-shrink-0 w-5 h-5 flex items-center justify-center">
          {hasChildren || isExpanded ? (
            isExpanded ? (
              <ChevronDown className="h-5 w-5 text-gray-500 dark:text-gray-400" />
            ) : (
              <ChevronRight className="h-5 w-5 text-gray-500 dark:text-gray-400" />
            )
          ) : (
            <span className="w-5 h-5" />
          )}
        </span>

        {/* Icon */}
        <Icon className="h-5 w-5 text-gray-500 dark:text-gray-400 flex-shrink-0" />

        {/* Content */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-medium text-gray-900 dark:text-white truncate">
              {location.identifier}
            </span>
            <span className="text-sm text-gray-500 dark:text-gray-400 truncate">
              {location.name}
            </span>
          </div>
        </div>

        {/* Status badge */}
        <span
          className={`
            flex-shrink-0 inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium
            ${
              location.is_active
                ? 'bg-green-50 text-green-700 border border-green-200 dark:bg-green-900/20 dark:text-green-400 dark:border-green-800'
                : 'bg-gray-50 text-gray-700 border border-gray-200 dark:bg-gray-900/20 dark:text-gray-400 dark:border-gray-800'
            }
          `}
        >
          {location.is_active ? 'Active' : 'Inactive'}
        </span>
      </button>

      {/* Expanded content */}
      {isExpanded && (
        <div className="border-t border-gray-200 dark:border-gray-700">
          {/* Details section */}
          <div className="p-4 space-y-4 bg-white dark:bg-gray-900">
            {/* Breadcrumb */}
            <div>
              <LocationBreadcrumb locationId={location.id} />
            </div>

            {/* Description */}
            {location.description && (
              <div>
                <p className="text-sm text-gray-600 dark:text-gray-400">
                  {location.description}
                </p>
              </div>
            )}

            {/* Action buttons */}
            <div className="flex gap-2 pt-2 border-t border-gray-200 dark:border-gray-700">
              <button
                onClick={handleEdit}
                className="flex-1 flex items-center justify-center gap-2 px-3 py-2 text-sm font-medium text-blue-700 bg-blue-50 hover:bg-blue-100 dark:text-blue-400 dark:bg-blue-900/20 dark:hover:bg-blue-900/40 border border-blue-200 dark:border-blue-800 rounded-lg transition-colors"
              >
                <Pencil className="h-4 w-4" />
                Edit
              </button>
              <button
                onClick={handleMove}
                className="flex-1 flex items-center justify-center gap-2 px-3 py-2 text-sm font-medium text-purple-700 bg-purple-50 hover:bg-purple-100 dark:text-purple-400 dark:bg-purple-900/20 dark:hover:bg-purple-900/40 border border-purple-200 dark:border-purple-800 rounded-lg transition-colors"
              >
                <ArrowRightLeft className="h-4 w-4" />
                Move
              </button>
              <button
                onClick={handleDelete}
                className="flex-1 flex items-center justify-center gap-2 px-3 py-2 text-sm font-medium text-red-700 bg-red-50 hover:bg-red-100 dark:text-red-400 dark:bg-red-900/20 dark:hover:bg-red-900/40 border border-red-200 dark:border-red-800 rounded-lg transition-colors"
              >
                <Trash2 className="h-4 w-4" />
                Delete
              </button>
              {onAddChild && (
                <button
                  onClick={handleAddChild}
                  className="flex-1 flex items-center justify-center gap-2 px-3 py-2 text-sm font-medium text-green-700 bg-green-50 hover:bg-green-100 dark:text-green-400 dark:bg-green-900/20 dark:hover:bg-green-900/40 border border-green-200 dark:border-green-800 rounded-lg transition-colors"
                >
                  <Plus className="h-4 w-4" />
                  Add Sub-location
                </button>
              )}
            </div>
          </div>

          {/* Children cards */}
          {visibleChildren.length > 0 && (
            <div className="p-3 pt-0 space-y-2 bg-gray-50 dark:bg-gray-800/50">
              <p className="text-xs font-medium text-gray-500 dark:text-gray-400 pt-3 pb-1">
                Children ({visibleChildren.length})
              </p>
              {visibleChildren.map((child) => (
                <LocationExpandableCard
                  key={child.id}
                  location={child}
                  depth={depth + 1}
                  onEdit={onEdit}
                  onMove={onMove}
                  onDelete={onDelete}
                  onAddChild={onAddChild}
                  searchTerm={searchTerm}
                />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
