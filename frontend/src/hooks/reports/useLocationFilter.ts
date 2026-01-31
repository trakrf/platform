import { useState, useRef, useEffect, useMemo, useCallback } from 'react';
import type { Location } from '@/types/locations';

interface UseLocationFilterProps {
  value: number | null;
  onChange: (locationId: number | null) => void;
  locations: Location[];
}

interface UseLocationFilterReturn {
  isOpen: boolean;
  search: string;
  containerRef: React.RefObject<HTMLDivElement>;
  inputRef: React.RefObject<HTMLInputElement>;
  filteredLocations: Location[];
  selectedLocation: Location | undefined;
  handleSelect: (locationId: number | null) => void;
  handleInputClick: () => void;
  handleSearchChange: (value: string) => void;
}

export function useLocationFilter({
  value,
  onChange,
  locations,
}: UseLocationFilterProps): UseLocationFilterReturn {
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState('');
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const filteredLocations = useMemo(() => {
    if (!search.trim()) return locations;
    const query = search.toLowerCase();
    return locations.filter(
      (loc) =>
        loc.name.toLowerCase().includes(query) ||
        loc.identifier.toLowerCase().includes(query)
    );
  }, [locations, search]);

  const selectedLocation = useMemo(
    () => locations.find((loc) => loc.id === value),
    [locations, value]
  );

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false);
        setSearch('');
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const handleSelect = useCallback(
    (locationId: number | null) => {
      onChange(locationId);
      setIsOpen(false);
      setSearch('');
    },
    [onChange]
  );

  const handleInputClick = useCallback(() => {
    setIsOpen(true);
    setTimeout(() => inputRef.current?.focus(), 0);
  }, []);

  const handleSearchChange = useCallback((value: string) => {
    setSearch(value);
  }, []);

  return {
    isOpen,
    search,
    containerRef,
    inputRef,
    filteredLocations,
    selectedLocation,
    handleSelect,
    handleInputClick,
    handleSearchChange,
  };
}
