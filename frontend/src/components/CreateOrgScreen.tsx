import { useState, useEffect, useRef } from 'react';
import { useOrgStore } from '@/stores';
import { ArrowLeft } from 'lucide-react';

export default function CreateOrgScreen() {
  const [name, setName] = useState('');
  const [errors, setErrors] = useState<{
    name?: string;
    general?: string;
  }>({});
  const nameInputRef = useRef<HTMLInputElement>(null);

  const { createOrg, isLoading } = useOrgStore();

  // Auto-focus name field on mount
  useEffect(() => {
    nameInputRef.current?.focus();
  }, []);

  const validateName = (name: string) => {
    if (!name) return 'Organization name is required';
    if (name.length < 2) return 'Name must be at least 2 characters';
    if (name.length > 100) return 'Name must be less than 100 characters';
    return null;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    setErrors({});

    const nameError = validateName(name);
    if (nameError) {
      setErrors({ name: nameError });
      return;
    }

    try {
      await createOrg(name);
      // Redirect to home after successful creation
      window.location.hash = '#home';
    } catch (err: unknown) {
      const data = (err as any).response?.data;
      const errorObj = data?.error || data;
      let errorMessage =
        (typeof errorObj?.detail === 'string' && errorObj.detail.trim()) ||
        (typeof errorObj?.title === 'string' && errorObj.title.trim()) ||
        (typeof data?.error === 'string' && data.error.trim()) ||
        (typeof (err as Error).message === 'string' && (err as Error).message.trim()) ||
        'Failed to create organization';

      if (typeof errorMessage !== 'string') {
        errorMessage = JSON.stringify(errorMessage);
      }

      setErrors({ general: errorMessage });
    }
  };

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
      <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md">
        <div className="flex items-center gap-4 mb-6">
          <a
            href="#home"
            className="text-gray-400 hover:text-gray-300 transition-colors"
          >
            <ArrowLeft className="w-5 h-5" />
          </a>
          <h1 className="text-2xl font-semibold text-white">Create Organization</h1>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label htmlFor="name" className="block text-sm font-medium text-gray-300 mb-2">
              Organization Name
            </label>
            <input
              ref={nameInputRef}
              id="name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              onBlur={() => {
                const error = validateName(name);
                if (error) setErrors(prev => ({ ...prev, name: error }));
              }}
              placeholder="My Organization"
              className="w-full px-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              disabled={isLoading}
            />
            {errors.name && (
              <p className="text-red-400 text-sm mt-1">{errors.name}</p>
            )}
          </div>

          {errors.general && (
            <div className="bg-red-900/20 border border-red-800 rounded-lg p-3">
              <p className="text-red-400 text-sm">{errors.general}</p>
            </div>
          )}

          <button
            type="submit"
            disabled={isLoading}
            className="w-full bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded-lg font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isLoading ? 'Creating...' : 'Create Organization'}
          </button>
        </form>

        <p className="text-gray-400 text-sm mt-6 text-center">
          You&apos;ll be the owner of this organization and can invite others to join.
        </p>
      </div>
    </div>
  );
}
