import type { JobStatusResponse } from '@/types/assets';

interface SuccessAlertProps {
  jobStatus: JobStatusResponse;
  onDismiss: () => void;
  onViewErrors: () => void;
}

export function SuccessAlert({ jobStatus, onDismiss, onViewErrors }: SuccessAlertProps) {
  const { successful_rows, failed_rows, total_rows } = jobStatus;
  const successCount = successful_rows ?? 0;
  const hasErrors = failed_rows > 0;

  return (
    <div className="w-full animate-slideDown mb-4">
      <div className="bg-green-100 dark:bg-green-900 border border-green-500 rounded-lg shadow-md">
        <div className="px-4 py-3">
          <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-3 md:gap-4">
            <div className="flex items-center gap-3 min-w-0 flex-1">
              <svg
                className="h-5 w-5 text-green-600 dark:text-green-400 flex-shrink-0"
                xmlns="http://www.w3.org/2000/svg"
                viewBox="0 0 20 20"
                fill="currentColor"
                aria-hidden="true"
              >
                <path
                  fillRule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.857-9.809a.75.75 0 00-1.214-.882l-3.483 4.79-1.88-1.88a.75.75 0 10-1.06 1.061l2.5 2.5a.75.75 0 001.137-.089l4-5.5z"
                  clipRule="evenodd"
                />
              </svg>

              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium text-green-900 dark:text-green-100">
                  CSV upload complete
                </p>
                <p className="text-xs text-green-700 dark:text-green-300 mt-0.5">
                  {successCount} {successCount === 1 ? 'asset' : 'assets'} created successfully
                  {hasErrors && ` • ${failed_rows} ${failed_rows === 1 ? 'error' : 'errors'}`} (
                  {total_rows} total)
                </p>
              </div>
            </div>

            <div className="flex items-center gap-2 justify-end md:justify-start">
              {hasErrors && (
                <button
                  onClick={onViewErrors}
                  className="text-green-700 dark:text-green-300 hover:text-green-900 dark:hover:text-green-100 text-sm font-medium px-3 py-2 rounded hover:bg-green-100 dark:hover:bg-green-900/30 transition-colors"
                  aria-label="View error details"
                >
                  View Errors
                </button>
              )}
              <button
                onClick={onDismiss}
                className="text-green-700 dark:text-green-300 hover:text-green-900 dark:hover:text-green-100 text-sm font-medium px-3 py-2 rounded hover:bg-green-100 dark:hover:bg-green-900/30 transition-colors"
                aria-label="Dismiss success alert"
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
