import { ReaderState, type ReaderStateType } from '../worker/types/reader';

interface ConfigurationSpinnerProps {
  readerState: ReaderStateType;
  mode: string;
}

/**
 * Full-screen loading overlay shown during reader configuration
 * Makes it very clear that the user needs to wait for configuration to complete
 */
export function ConfigurationSpinner({ readerState, mode }: ConfigurationSpinnerProps) {
  // Show spinner when reader is busy (configuring) after mode change
  const showSpinner = readerState === ReaderState.BUSY;

  if (!showSpinner) {
    return null;
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm">
      <div className="flex flex-col items-center space-y-6 bg-white dark:bg-zinc-800 p-12 rounded-2xl shadow-2xl">
        {/* Large animated spinner */}
        <div className="relative">
          <div className="h-24 w-24 animate-spin rounded-full border-8 border-gray-300 dark:border-gray-600 border-t-blue-600 dark:border-t-blue-400" />
          <div className="absolute inset-0 flex items-center justify-center">
            <div className="h-16 w-16 animate-pulse rounded-full bg-blue-100 dark:bg-blue-900/30" />
          </div>
        </div>

        {/* Status text */}
        <div className="text-center space-y-2">
          <h2 className="text-2xl font-bold text-gray-900 dark:text-white">
            Configuring Reader
          </h2>
          <p className="text-lg text-gray-600 dark:text-gray-300">
            Setting up {mode} mode...
          </p>
          <p className="text-sm text-gray-500 dark:text-gray-400 animate-pulse">
            Please wait while the hardware initializes
          </p>
        </div>

        {/* Progress dots animation */}
        <div className="flex space-x-2">
          <div className="h-3 w-3 animate-bounce rounded-full bg-blue-600 dark:bg-blue-400" style={{ animationDelay: '0ms' }} />
          <div className="h-3 w-3 animate-bounce rounded-full bg-blue-600 dark:bg-blue-400" style={{ animationDelay: '150ms' }} />
          <div className="h-3 w-3 animate-bounce rounded-full bg-blue-600 dark:bg-blue-400" style={{ animationDelay: '300ms' }} />
        </div>
      </div>
    </div>
  );
}