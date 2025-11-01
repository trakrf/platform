import { CSV_VALIDATION } from '@/types/assets';

/**
 * Creates FormData for CSV upload with proper field naming
 *
 * @param file - CSV file to upload
 * @returns FormData instance ready for API submission
 *
 * @example
 * const formData = createAssetCSVFormData(csvFile);
 * await assetsApi.uploadCSV(formData);
 */
export function createAssetCSVFormData(file: File): FormData {
  const formData = new FormData();
  formData.append('file', file);
  return formData;
}

/**
 * Validates CSV file before upload (client-side only)
 *
 * @param file - File to validate
 * @returns Error message if invalid, null if valid
 */
export function validateCSVFile(file: File): string | null {
  if (file.size > CSV_VALIDATION.MAX_FILE_SIZE) {
    const sizeMB = (file.size / (1024 * 1024)).toFixed(2);
    return `File size must not exceed 5MB (current: ${sizeMB}MB)`;
  }

  if (!file.name.toLowerCase().endsWith(CSV_VALIDATION.ALLOWED_EXTENSION)) {
    return `Invalid file extension. File must be ${CSV_VALIDATION.ALLOWED_EXTENSION}`;
  }

  if (
    file.type &&
    !(CSV_VALIDATION.ALLOWED_MIME_TYPES as readonly string[]).includes(
      file.type
    )
  ) {
    return `Invalid file type: ${file.type}. Expected CSV file.`;
  }

  return null;
}

/**
 * Extracts user-friendly error message from API error responses
 * Handles RFC 7807 Problem Details format and axios error structure
 *
 * @param err - Error object from API call
 * @param defaultMessage - Fallback message if extraction fails
 * @returns Extracted error message
 *
 * @example
 * try {
 *   await assetsApi.create(data);
 * } catch (err) {
 *   const message = extractErrorMessage(err, 'Failed to create asset');
 *   console.error(message);
 * }
 */
export function extractErrorMessage(
  err: unknown,
  defaultMessage = 'An error occurred'
): string {
  const axiosError = err as {
    response?: {
      data?: {
        error?:
          | {
              detail?: string;
              title?: string;
            }
          | string;
        detail?: string;
        title?: string;
      };
    };
    message?: string;
  };

  const data = axiosError.response?.data;

  const errorObj = typeof data?.error === 'object' ? data.error : data;

  // Try RFC 7807 detail field
  if (typeof errorObj?.detail === 'string' && errorObj.detail.trim()) {
    return errorObj.detail;
  }

  // Try RFC 7807 title field
  if (typeof errorObj?.title === 'string' && errorObj.title.trim()) {
    return errorObj.title;
  }

  if (typeof data?.error === 'string' && data.error.trim()) {
    return data.error;
  }

  if (typeof axiosError.message === 'string' && axiosError.message.trim()) {
    return axiosError.message;
  }

  return defaultMessage;
}
