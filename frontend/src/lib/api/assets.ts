import { apiClient } from './client';
import type {
  AssetResponse,
  CreateAssetRequest,
  UpdateAssetRequest,
  DeleteResponse,
  ListAssetsResponse,
  BulkUploadResponse,
  JobStatusResponse,
} from '@/types/asset';

export interface ListAssetsOptions {
  limit?: number;
  offset?: number;
}

/**
 * Asset Management API Client
 *
 * Provides type-safe methods for all asset CRUD operations and bulk upload.
 * All methods use the shared apiClient with automatic JWT injection.
 * Errors propagate unchanged - caller handles RFC 7807 extraction.
 */
export const assetsApi = {
  /**
   * List assets with pagination
   * GET /api/v1/assets?limit={limit}&offset={offset}
   */
  list: (options: ListAssetsOptions = {}) => {
    const params = new URLSearchParams();
    if (options.limit !== undefined) {
      params.append('limit', String(options.limit));
    }
    if (options.offset !== undefined) {
      params.append('offset', String(options.offset));
    }

    const queryString = params.toString();
    const url = queryString ? `/assets?${queryString}` : '/assets';

    return apiClient.get<ListAssetsResponse>(url);
  },

  /**
   * Get single asset by ID
   * GET /api/v1/assets/:id
   */
  get: (id: number) => apiClient.get<AssetResponse>(`/assets/${id}`),

  /**
   * Create new asset
   * POST /api/v1/assets
   */
  create: (data: CreateAssetRequest) =>
    apiClient.post<AssetResponse>('/assets', data),

  /**
   * Update existing asset
   * PUT /api/v1/assets/:id
   */
  update: (id: number, data: UpdateAssetRequest) =>
    apiClient.put<AssetResponse>(`/assets/${id}`, data),

  /**
   * Soft delete asset
   * DELETE /api/v1/assets/:id
   */
  delete: (id: number) => apiClient.delete<DeleteResponse>(`/assets/${id}`),

  /**
   * Upload CSV for bulk asset creation
   * POST /api/v1/assets/bulk
   *
   * @param file - CSV file to upload
   * @returns Promise with job details for status polling
   */
  uploadCSV: (file: File) => {
    const formData = new FormData();
    formData.append('file', file);

    return apiClient.post<BulkUploadResponse>('/assets/bulk', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
  },

  /**
   * Get bulk import job status
   * GET /api/v1/assets/bulk/:jobId
   */
  getJobStatus: (jobId: string) =>
    apiClient.get<JobStatusResponse>(`/assets/bulk/${jobId}`),
};
