/**
 * Validates that end date is after start date
 *
 * @param validFrom - Start date (ISO 8601 string)
 * @param validTo - End date (ISO 8601 string) or null
 * @returns Error message if invalid, null if valid
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
