import { useState, useEffect, useRef } from 'react';
import { useOrgStore, useAuthStore } from '@/stores';
import { useOrgSwitch } from '@/hooks/orgs/useOrgSwitch';
import { orgsApi } from '@/lib/api/orgs';
import { extractErrorMessage } from '@/lib/asset/helpers';
import type { OrgMember, OrgRole } from '@/types/org';
import toast from 'react-hot-toast';

export type ModalMode = 'create' | 'manage';
export type TabType = 'members' | 'settings';
export const ROLES: OrgRole[] = ['owner', 'admin', 'manager', 'operator', 'viewer'];

interface UseOrgModalProps {
  isOpen: boolean;
  onClose: () => void;
  mode: ModalMode;
  defaultTab: TabType;
}

export function useOrgModal({ isOpen, onClose, mode, defaultTab }: UseOrgModalProps) {
  const { currentOrg, currentRole, isLoading: isOrgLoading } = useOrgStore();
  const { createOrg } = useOrgSwitch();
  const { profile, fetchProfile } = useAuthStore();

  // Manage mode state
  const [activeTab, setActiveTab] = useState<TabType>(defaultTab);
  const [members, setMembers] = useState<OrgMember[]>([]);
  const [isLoadingMembers, setIsLoadingMembers] = useState(true);
  const [membersError, setMembersError] = useState<string | null>(null);
  const [updatingUserId, setUpdatingUserId] = useState<number | null>(null);
  const [removingUserId, setRemovingUserId] = useState<number | null>(null);
  const [orgName, setOrgName] = useState('');
  const [originalName, setOriginalName] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [settingsError, setSettingsError] = useState<string | null>(null);

  // Create mode state
  const [newOrgName, setNewOrgName] = useState('');
  const [createError, setCreateError] = useState<string | null>(null);
  const [createNameError, setCreateNameError] = useState<string | null>(null);
  const nameInputRef = useRef<HTMLInputElement>(null);

  const isAdmin = currentRole === 'owner' || currentRole === 'admin';
  const currentUserId = profile?.id;
  const hasNameChanges = orgName !== originalName;
  const isNameValid = orgName.trim().length >= 2;

  // Validation
  const validateOrgName = (name: string) => {
    if (!name) return 'Organization name is required';
    if (name.length < 2) return 'Name must be at least 2 characters';
    if (name.length > 100) return 'Name must be less than 100 characters';
    return null;
  };

  // Load members
  const loadMembers = async () => {
    if (!currentOrg) return;
    setIsLoadingMembers(true);
    setMembersError(null);
    try {
      const response = await orgsApi.listMembers(currentOrg.id);
      setMembers(response.data.data ?? []);
    } catch (err) {
      setMembersError(extractErrorMessage(err, 'Failed to load members'));
    } finally {
      setIsLoadingMembers(false);
    }
  };

  // Modal open handlers
  const handleManageModeOpen = () => {
    setActiveTab(defaultTab);
    if (currentOrg) {
      setOrgName(currentOrg.name);
      setOriginalName(currentOrg.name);
    }
    if (defaultTab === 'members') loadMembers();
  };

  const handleCreateModeOpen = () => {
    setNewOrgName('');
    setCreateError(null);
    setCreateNameError(null);
    setTimeout(() => nameInputRef.current?.focus(), 100);
  };

  const handleTabChange = (tab: TabType) => {
    setActiveTab(tab);
    if (tab === 'members') loadMembers();
  };

  // Member management
  const handleRoleChange = async (userId: number, newRole: OrgRole) => {
    if (!currentOrg || updatingUserId !== null) return;
    setUpdatingUserId(userId);
    setMembersError(null);
    try {
      await orgsApi.updateMemberRole(currentOrg.id, userId, newRole);
      setMembers(prev => prev.map(m => (m.user_id === userId ? { ...m, role: newRole } : m)));
      if (userId === currentUserId) await fetchProfile();
      toast.success('Member role updated');
    } catch (err) {
      setMembersError(extractErrorMessage(err, 'Failed to update role'));
    } finally {
      setUpdatingUserId(null);
    }
  };

  const handleRemoveMember = async (userId: number) => {
    if (!currentOrg || removingUserId !== null) return;
    if (userId === currentUserId) {
      setMembersError('You cannot remove yourself from the organization');
      return;
    }
    setRemovingUserId(userId);
    setMembersError(null);
    try {
      await orgsApi.removeMember(currentOrg.id, userId);
      setMembers(prev => prev.filter(m => m.user_id !== userId));
      toast.success('Member removed');
    } catch (err) {
      setMembersError(extractErrorMessage(err, 'Failed to remove member'));
    } finally {
      setRemovingUserId(null);
    }
  };

  // Settings management
  const handleSaveSettings = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!currentOrg || !hasNameChanges || isSaving) return;
    setSettingsError(null);
    setIsSaving(true);
    try {
      await orgsApi.update(currentOrg.id, { name: orgName });
      await fetchProfile();
      setOriginalName(orgName);
      toast.success('Organization name updated');
    } catch (err) {
      setSettingsError(extractErrorMessage(err, 'Failed to update organization'));
    } finally {
      setIsSaving(false);
    }
  };

  const handleDeleteOrg = async (confirmName: string) => {
    if (!currentOrg || isDeleting) return;
    setIsDeleting(true);
    try {
      await orgsApi.delete(currentOrg.id, confirmName);
      await fetchProfile();
      toast.success('Organization deleted');
      onClose();
      window.location.hash = '#home';
    } catch (err) {
      setSettingsError(extractErrorMessage(err, 'Failed to delete organization'));
      setShowDeleteModal(false);
    } finally {
      setIsDeleting(false);
    }
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

  useEffect(() => {
    if (isOpen) {
      if (mode === 'create') {
        handleCreateModeOpen();
      } else {
        handleManageModeOpen();
      }
    }
  }, [isOpen, mode]);

  return {
    // Common
    currentOrg,
    currentRole,
    isAdmin,
    handleBackdropClick,

    // Manage mode
    activeTab,
    members,
    isLoadingMembers,
    membersError,
    updatingUserId,
    removingUserId,
    orgName,
    setOrgName,
    isSaving,
    isDeleting,
    showDeleteModal,
    settingsError,
    currentUserId,
    hasNameChanges,
    isNameValid,
    handleTabChange,
    handleRoleChange,
    handleRemoveMember,
    handleSaveSettings,
    handleDeleteOrg,
    openDeleteModal: () => setShowDeleteModal(true),
    closeDeleteModal: () => setShowDeleteModal(false),

    // Create mode
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
