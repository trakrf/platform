interface Props {
  keyName: string;
  onConfirm: () => void;
  onCancel: () => void;
  busy?: boolean;
}

export function RevokeConfirmModal({ keyName, onConfirm, onCancel, busy }: Props) {
  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg p-6 w-full max-w-md space-y-4">
        <h2 className="text-lg font-semibold">Revoke API key?</h2>
        <p className="text-sm">
          Revoke key <strong>{keyName}</strong>? Applications using this key
          stop working immediately. This action cannot be undone.
        </p>
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            className="px-4 py-2 text-sm border rounded"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onConfirm}
            disabled={busy}
            className="px-4 py-2 text-sm bg-red-600 text-white rounded disabled:opacity-50"
          >
            Revoke
          </button>
        </div>
      </div>
    </div>
  );
}
