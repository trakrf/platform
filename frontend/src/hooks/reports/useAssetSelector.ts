import { useState, useRef, useEffect, useMemo, useCallback } from 'react';
import type { AssetOption } from './useAssetHistoryTab';

interface UseAssetSelectorProps {
  value: number | null;
  onChange: (assetId: number | null) => void;
  assets: AssetOption[];
}

interface UseAssetSelectorReturn {
  isOpen: boolean;
  search: string;
  containerRef: React.RefObject<HTMLDivElement>;
  inputRef: React.RefObject<HTMLInputElement>;
  filteredAssets: AssetOption[];
  selectedAsset: AssetOption | undefined;
  handleSelect: (assetId: number) => void;
  handleClear: (e: React.MouseEvent) => void;
  handleInputClick: () => void;
  handleSearchChange: (value: string) => void;
}

export function useAssetSelector({
  value,
  onChange,
  assets,
}: UseAssetSelectorProps): UseAssetSelectorReturn {
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState('');
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const filteredAssets = useMemo(() => {
    if (!search.trim()) return assets;
    const query = search.toLowerCase();
    return assets.filter(
      (a) =>
        a.name.toLowerCase().includes(query) ||
        a.identifier.toLowerCase().includes(query)
    );
  }, [assets, search]);

  const selectedAsset = useMemo(
    () => assets.find((a) => a.id === value),
    [assets, value]
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

  const handleSelect = useCallback((assetId: number) => {
    onChange(assetId);
    setIsOpen(false);
    setSearch('');
  }, [onChange]);

  const handleClear = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    onChange(null);
    setSearch('');
  }, [onChange]);

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
    filteredAssets,
    selectedAsset,
    handleSelect,
    handleClear,
    handleInputClick,
    handleSearchChange,
  };
}
