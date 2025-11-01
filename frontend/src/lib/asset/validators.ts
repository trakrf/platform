import type { AssetType } from '@/types/assets';

/**
 * Validates that end date is after start date
 *
 * @param validFrom - Start date (ISO 8601 string)
 * @param validTo - End date (ISO 8601 string) or null
 * @returns Error message if invalid, null if valid
 *
 * @example
 * validateDateRange('2024-01-15', '2024-12-31') // null (valid)
 * validateDateRange('2024-12-31', '2024-01-15') // "End date must be after start date"
 * validateDateRange('2024-01-15', null)         // null (valid - no end date)
 */
export function validateDateRange(
  validFrom: string,
  validTo: string | null
): string | null {
  if (!validTo) {
    return null;
  }

  try {
    const fromDate = new Date(validFrom);
    const toDate = new Date(validTo);

    if (isNaN(fromDate.getTime()) || isNaN(toDate.getTime())) {
      return 'Invalid date format';
    }

    if (toDate <= fromDate) {
      return 'End date must be after start date';
    }

    return null; // Valid
  } catch (error) {
    return 'Invalid date format';
  }
}

/**
 * Validates that asset type is one of the allowed types
 *
 * @param type - Asset type to validate
 * @returns true if valid, false if invalid
 *
 * @example
 * validateAssetType('device')     // true
 * validateAssetType('person')     // true
 * validateAssetType('computer')   // false
 */
export function validateAssetType(type: string): type is AssetType {
  const validTypes: AssetType[] = [
    'person',
    'device',
    'asset',
    'inventory',
    'other',
  ];
  return validTypes.includes(type as AssetType);
}
