import type { JobStatusResponse } from '@/types/assets';
import { ProgressBar } from './ProgressBar';

interface ProcessingAlertProps {
  jobStatus: JobStatusResponse;
  onDismiss: () => void;
}

export function ProcessingAlert({ jobStatus, onDismiss }: ProcessingAlertProps) {
  const { total_rows, processed_rows, failed_rows, successful_rows } = jobStatus;
  const successCount = successful_rows ?? 0;

  return (
    <div className="w-full animate-slideDown mb-4">
      <div className="bg-blue-100 dark:bg-blue-900 border border-blue-500 rounded-lg shadow-md">
        <div className="px-4 py-3">
          <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-3 md:gap-4">
            <div className="flex items-center gap-3 min-w-0 flex-1">
              <svg
                className="animate-spin h-5 w-5 text-blue-600 dark:text-blue-400 flex-shrink-0"
                xmlns="http://www.w3.org/2000/svg"
                fill="none"
                viewBox="0 0 24 24"
                aria-hidden="true"
              >
                <circle
                  className="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  strokeWidth="4"
                />
                <path
                  className="opacity-75"
                  fill="currentColor"
                  d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                />
              </svg>

              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium text-blue-900 dark:text-blue-100">
                  Processing CSV upload...
                </p>
                <p className="text-xs text-blue-700 dark:text-blue-300 mt-0.5">
                  {processed_rows} / {total_rows} rows processed
                  {successCount > 0 && ` • ${successCount} successful`}
                  {failed_rows > 0 && ` • ${failed_rows} failed`}
                </p>
              </div>
            </div>

            <div className="flex justify-end md:justify-start">
              <button
                onClick={onDismiss}
                className="text-blue-700 dark:text-blue-300 hover:text-blue-900 dark:hover:text-blue-100 text-sm font-medium px-3 py-2 rounded hover:bg-blue-100 dark:hover:bg-blue-900/30 transition-colors"
                aria-label="Dismiss upload progress alert"
              >
                Dismiss
              </button>
            </div>
          </div>

          <div className="mt-2">
            <ProgressBar value={processed_rows} max={total_rows} variant="blue" />
          </div>
        </div>
      </div>
    </div>
  );
}
