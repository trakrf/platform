/**
 * Common type definitions for the TrakRF Handheld application
 */

// Re-export commonly used types from stores
export type { TagInfo } from '@/stores/tagStore';
export type { TabType } from '@/stores/uiStore';

// Re-export RFID constants and types
export { ReaderState, type ReaderStateType } from '@/worker/types/reader';

// Re-export org types
export type {
  OrgRole,
  UserOrg,
  UserOrgWithRole,
  UserProfile,
  Organization,
  OrgMember,
  Invitation,
} from './org';

// Re-export shared types
export type { TagIdentifier, IdentifierType } from './shared';

// Re-export report types
export type {
  CurrentLocationItem,
  CurrentLocationsResponse,
  CurrentLocationsParams,
  AssetInfo,
  AssetHistoryItem,
  AssetHistoryResponse,
  AssetHistoryParams,
  FreshnessStatus,
} from './reports';