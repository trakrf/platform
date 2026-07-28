/**
 * useOrgModal - state for the Create Organization modal.
 *
 * TRA-1058 deleted the manage half (members, org name, delete). Those flows
 * live on MembersScreen / OrgSettingsScreen and own their own state.
 */
import { useState, useEffect, useRef } from 'react';
import { useOrgStore } from '@/stores';
import { useOrgSwitch } from '@/hooks/orgs/useOrgSwitch';
import { extractErrorMessage } from '@/lib/asset/helpers';
import toast from 'react-hot-toast';

interface UseOrgModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export function useOrgModal({ isOpen, onClose }: UseOrgModalProps) {
  const { isLoading: isOrgLoading } = useOrgStore();
  const { createOrg } = useOrgSwitch();

  const [newOrgName, setNewOrgName] = useState('');
  const [createError, setCreateError] = useState<string | null>(null);
  const [createNameError, setCreateNameError] = useState<string | null>(null);
  const nameInputRef = useRef<HTMLInputElement>(null);

  const validateOrgName = (name: string) => {
    if (!name) return 'Organization name is required';
    if (name.length < 2) return 'Name must be at least 2 characters';
    if (name.length > 100) return 'Name must be less than 100 characters';
    return null;
  };

  const handleCreateOrg = async (e: React.FormEvent) => {
    e.preventDefault();
    setCreateError(null);
    setCreateNameError(null);

    const nameError = validateOrgName(newOrgName);
    if (nameError) {
      setCreateNameError(nameError);
      return;
    }

    try {
      await createOrg(newOrgName);
      toast.success(`Organization "${newOrgName}" created`);
      onClose();
    } catch (err) {
      setCreateError(extractErrorMessage(err, 'Failed to create organization'));
    }
  };

  const handleCreateNameBlur = () => {
    const error = validateOrgName(newOrgName);
    if (error) setCreateNameError(error);
  };

  const handleBackdropClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget && !isOrgLoading) onClose();
  };

  // Reset on open so a cancelled attempt does not leak its name or error into
  // the next one.
  useEffect(() => {
    if (!isOpen) return;
    setNewOrgName('');
    setCreateError(null);
    setCreateNameError(null);
    const timer = setTimeout(() => nameInputRef.current?.focus(), 100);
    return () => clearTimeout(timer);
  }, [isOpen]);

  return {
    handleBackdropClick,
    newOrgName,
    setNewOrgName,
    createError,
    createNameError,
    isCreating: isOrgLoading,
    nameInputRef,
    handleCreateOrg,
    handleCreateNameBlur,
  };
}
