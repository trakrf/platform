interface ProcessingOverlayProps {
  isProcessing: boolean;
  message?: string;
}

export function ProcessingOverlay({ isProcessing, message = 'Processing...' }: ProcessingOverlayProps) {
  if (!isProcessing) return null;

  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg p-6 flex items-center space-x-3">
        <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600"></div>
        <span className="text-gray-700 dark:text-gray-300">{message}</span>
      </div>
    </div>
  );
}