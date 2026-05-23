import { describe, it, expect } from 'vitest';
import { getApiErrorMessage } from './errorMessage';

describe('getApiErrorMessage', () => {
  it('returns server detail when present (the 409 cascade case)', () => {
    const err = {
      message: 'Request failed with status code 409',
      response: {
        status: 409,
        data: {
          error: {
            detail:
              'has descendant locations; reassign or remove them before deleting (cascade is not supported)',
          },
        },
      },
    };
    expect(getApiErrorMessage(err, 'Failed to delete')).toBe(
      'has descendant locations; reassign or remove them before deleting (cascade is not supported)'
    );
  });

  it('falls back to err.message when no detail', () => {
    const err = { message: 'Network Error' };
    expect(getApiErrorMessage(err, 'Failed to delete')).toBe('Network Error');
  });

  it('falls back to the provided default when neither is present', () => {
    expect(getApiErrorMessage({}, 'Failed to delete')).toBe('Failed to delete');
    expect(getApiErrorMessage(null, 'Failed to delete')).toBe('Failed to delete');
    expect(getApiErrorMessage(undefined, 'Failed to delete')).toBe('Failed to delete');
  });

  it('ignores empty-string detail and message', () => {
    const err = { message: '', response: { data: { error: { detail: '' } } } };
    expect(getApiErrorMessage(err, 'fallback')).toBe('fallback');
  });

  it('ignores non-string detail', () => {
    const err = {
      message: 'fallback msg',
      response: { data: { error: { detail: { nested: 'object' } } } },
    };
    expect(getApiErrorMessage(err, 'default')).toBe('fallback msg');
  });
});
