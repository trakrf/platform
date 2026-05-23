// Surface a useful error message from an axios-thrown API error.
// Priority: server-provided RFC 7807 detail → axios message → fallback.
export function getApiErrorMessage(err: unknown, fallback: string): string {
  const e = err as {
    response?: { data?: { error?: { detail?: unknown } } };
    message?: unknown;
  };
  const detail = e?.response?.data?.error?.detail;
  if (typeof detail === 'string' && detail.length > 0) return detail;
  const message = e?.message;
  if (typeof message === 'string' && message.length > 0) return message;
  return fallback;
}
