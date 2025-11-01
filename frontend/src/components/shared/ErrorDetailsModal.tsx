import type { BulkErrorDetail } from '@/types/assets';

interface ErrorDetailsModalProps {
  isOpen: boolean;
  errors: BulkErrorDetail[];
  onClose: () => void;
}

export function ErrorDetailsModal({ isOpen, errors, onClose }: ErrorDetailsModalProps) {
  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div
        className="absolute inset-0 bg-black bg-opacity-50"
        onClick={onClose}
        aria-label="Close error details modal"
      />

      <div className="relative bg-white dark:bg-gray-800 rounded-lg shadow-xl max-w-4xl w-full mx-4 max-h-[80vh] flex flex-col">
        <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-700">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
            CSV Upload Errors ({errors.length})
          </h3>
        </div>

        <div className="overflow-y-auto flex-1 px-6 py-4">
          <table className="w-full text-sm">
            <thead className="sticky top-0 bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700">
              <tr>
                <th className="text-left py-2 px-3 font-semibold text-gray-700 dark:text-gray-300">
                  Row
                </th>
                <th className="text-left py-2 px-3 font-semibold text-gray-700 dark:text-gray-300">
                  Field
                </th>
                <th className="text-left py-2 px-3 font-semibold text-gray-700 dark:text-gray-300">
                  Error Message
                </th>
              </tr>
            </thead>
            <tbody>
              {errors.map((errorDetail, index) => (
                <tr
                  key={index}
                  className="border-b border-gray-100 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-700/50"
                >
                  <td className="py-2 px-3 text-gray-900 dark:text-gray-100 font-mono">
                    {errorDetail.row}
                  </td>
                  <td className="py-2 px-3 text-gray-700 dark:text-gray-300 font-mono">
                    {errorDetail.field ?? '—'}
                  </td>
                  <td className="py-2 px-3 text-red-700 dark:text-red-400">
                    {errorDetail.error}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="px-6 py-4 border-t border-gray-200 dark:border-gray-700 flex justify-end">
          <button
            onClick={onClose}
            className="px-4 py-2 bg-gray-200 dark:bg-gray-700 text-gray-800 dark:text-gray-200 rounded hover:bg-gray-300 dark:hover:bg-gray-600 transition-colors"
            aria-label="Close error details modal"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
}
