import type { JobStatusResponse } from '@/types/assets';

interface ErrorAlertProps {
  jobStatus?: JobStatusResponse;
  error?: Error;
  onDismiss: () => void;
  onRetry: () => void;
  onViewDetails?: () => void;
  isRetrying?: boolean;
}

export function ErrorAlert({
  jobStatus,
  error,
  onDismiss,
  onRetry,
  onViewDetails,
  isRetrying = false,
}: ErrorAlertProps) {
  const errorMessage = error
    ? error.message
    : jobStatus?.errors && jobStatus.errors.length > 0
      ? 'CSV upload failed with validation errors'
      : 'CSV upload failed';

  const hasDetails = jobStatus?.errors && jobStatus.errors.length > 0;

  return (
    <div className="w-full animate-slideDown mb-4">
      <div className="bg-red-100 dark:bg-red-900 border border-red-500 rounded-lg shadow-md">
        <div className="px-4 py-3">
          <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-3 md:gap-4">
            <div className="flex items-center gap-3 min-w-0 flex-1">
              <svg
                className="h-5 w-5 text-red-600 dark:text-red-400 flex-shrink-0"
                xmlns="http://www.w3.org/2000/svg"
                viewBox="0 0 20 20"
                fill="currentColor"
                aria-hidden="true"
              >
                <path
                  fillRule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z"
                  clipRule="evenodd"
                />
              </svg>

              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium text-red-900 dark:text-red-100">
                  {errorMessage}
                </p>
                {jobStatus && (
                  <p className="text-xs text-red-700 dark:text-red-300 mt-0.5">
                    {jobStatus.failed_rows} {jobStatus.failed_rows === 1 ? 'row' : 'rows'} failed
                    out of {jobStatus.total_rows} total
                  </p>
                )}
              </div>
            </div>

            <div className="flex items-center gap-2 justify-end md:justify-start">
              {hasDetails && onViewDetails && (
                <button
                  onClick={onViewDetails}
                  className="text-red-700 dark:text-red-300 hover:text-red-900 dark:hover:text-red-100 text-sm font-medium px-3 py-2 rounded hover:bg-red-100 dark:hover:bg-red-900/30 transition-colors"
                  aria-label="View error details"
                >
                  View Details
                </button>
              )}
              <button
                onClick={onRetry}
                disabled={isRetrying}
                className="text-red-700 dark:text-red-300 hover:text-red-900 dark:hover:text-red-100 text-sm font-medium px-3 py-2 rounded hover:bg-red-100 dark:hover:bg-red-900/30 transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1.5"
                aria-label="Retry checking upload status"
              >
                {isRetrying && (
                  <svg className="animate-spin h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                  </svg>
                )}
                {isRetrying ? 'Retrying...' : 'Retry'}
              </button>
              <button
                onClick={onDismiss}
                className="text-red-700 dark:text-red-300 hover:text-red-900 dark:hover:text-red-100 text-sm font-medium px-3 py-2 rounded hover:bg-red-100 dark:hover:bg-red-900/30 transition-colors"
                aria-label="Dismiss error alert"
              >
                Dismiss
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
