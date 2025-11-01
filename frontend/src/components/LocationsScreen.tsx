import { useUIStore } from '@/stores';
import { MapPinned, Home as HomeIcon } from 'lucide-react';
import { ProtectedRoute } from '@/components/ProtectedRoute';

export default function LocationsScreen() {
  const { setActiveTab } = useUIStore();

  const handleBackToHome = () => {
    setActiveTab('home');
    window.history.pushState({ tab: 'home' }, '', '#home');
  };

  return (
    <ProtectedRoute>
      <div className="max-w-4xl mx-auto">
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-lg border border-gray-200 dark:border-gray-700 p-8">
        {/* Header with icon */}
        <div className="flex items-center justify-center mb-6">
          <div className="w-16 h-16 text-green-600 dark:text-green-400">
            <MapPinned className="w-full h-full" />
          </div>
        </div>

        {/* Title */}
        <h1 className="text-3xl font-bold text-gray-900 dark:text-gray-100 text-center mb-4">
          Locations Management
        </h1>

        {/* Description */}
        <p className="text-lg text-gray-600 dark:text-gray-400 text-center mb-8">
          Location tracking and management features are coming soon. This page will allow you to view, create, edit, and manage your locations.
        </p>

        {/* Back to Home button */}
        <div className="flex justify-center">
          <button
            onClick={handleBackToHome}
            className="flex items-center gap-2 px-6 py-3 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors duration-200 font-medium"
          >
            <HomeIcon className="w-5 h-5" />
            Back to Home
          </button>
        </div>
      </div>
    </div>
    </ProtectedRoute>
  );
}
