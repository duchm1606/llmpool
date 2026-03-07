'use client';

import { useState, useCallback } from 'react';
import { AuditResponse, AuditFilters } from '@/types/api';
import { formatDateTime, formatNumber, formatCurrency, formatDuration } from '@/lib/utils';

interface AuditTrailTableProps {
  data: AuditResponse;
  onFiltersChange: (filters: AuditFilters) => void;
  filters: AuditFilters;
}

export function AuditTrailTable({ data, onFiltersChange, filters }: AuditTrailTableProps) {
  const [localFilters, setLocalFilters] = useState<AuditFilters>(filters);

  const handleFilterChange = useCallback(
    (key: keyof AuditFilters, value: string) => {
      const newFilters = { ...localFilters, [key]: value || undefined, page: 1 };
      setLocalFilters(newFilters);
    },
    [localFilters]
  );

  const applyFilters = useCallback(() => {
    onFiltersChange(localFilters);
  }, [localFilters, onFiltersChange]);

  const handlePageChange = useCallback(
    (page: number) => {
      const newFilters = { ...filters, page };
      onFiltersChange(newFilters);
    },
    [filters, onFiltersChange]
  );

  const clearFilters = useCallback(() => {
    const cleared: AuditFilters = { page: 1, page_size: 20 };
    setLocalFilters(cleared);
    onFiltersChange(cleared);
  }, [onFiltersChange]);

  return (
    <div className="bg-white rounded-lg shadow-sm border border-gray-200 overflow-hidden">
      <div className="px-4 py-3 border-b border-gray-200">
        <h3 className="text-sm font-medium text-gray-700">Audit Trail</h3>
      </div>

      {/* Filters */}
      <div className="px-4 py-3 bg-gray-50 border-b border-gray-200">
        <div className="flex flex-wrap gap-3">
          <input
            type="text"
            placeholder="Model"
            value={localFilters.model || ''}
            onChange={(e) => handleFilterChange('model', e.target.value)}
            className="px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:ring-blue-500 focus:border-blue-500 w-32"
          />
          <input
            type="text"
            placeholder="Provider"
            value={localFilters.provider || ''}
            onChange={(e) => handleFilterChange('provider', e.target.value)}
            className="px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:ring-blue-500 focus:border-blue-500 w-28"
          />
          <input
            type="text"
            placeholder="Credential ID"
            value={localFilters.credential_id || ''}
            onChange={(e) => handleFilterChange('credential_id', e.target.value)}
            className="px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:ring-blue-500 focus:border-blue-500 w-28"
          />
          <select
            value={localFilters.status || ''}
            onChange={(e) => handleFilterChange('status', e.target.value)}
            className="px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:ring-blue-500 focus:border-blue-500"
          >
            <option value="">All Status</option>
            <option value="done">Done</option>
            <option value="failed">Failed</option>
            <option value="canceled">Canceled</option>
          </select>
          <button
            onClick={applyFilters}
            className="px-4 py-1.5 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700"
          >
            Apply
          </button>
          <button
            onClick={clearFilters}
            className="px-4 py-1.5 text-sm font-medium text-gray-600 bg-white border border-gray-300 rounded-md hover:bg-gray-50"
          >
            Clear
          </button>
        </div>
      </div>

      {/* Table */}
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                Timestamp
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                Model
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                Provider
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                Credential
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                Tokens
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                Cost
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                Latency
              </th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                Status
              </th>
            </tr>
          </thead>
          <tbody className="bg-white divide-y divide-gray-200">
            {data.entries.map((entry) => (
              <tr key={entry.id} className="hover:bg-gray-50">
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
                  {formatDateTime(entry.timestamp)}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm font-medium text-gray-900">
                  {entry.model}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
                  {entry.provider}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500 font-mono">
                  {entry.credential_id}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
                  <span className="text-green-600">{formatNumber(entry.tokens_in)}</span>
                  {' / '}
                  <span className="text-blue-600">{formatNumber(entry.tokens_out)}</span>
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-900">
                  {formatCurrency(entry.cost)}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
                  {formatDuration(entry.latency_ms)}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm">
                  <span
                    className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
                      entry.status === 'done'
                        ? 'bg-green-100 text-green-800'
                        : entry.status === 'failed'
                        ? 'bg-red-100 text-red-800'
                        : 'bg-yellow-100 text-yellow-800'
                    }`}
                  >
                    {entry.status}
                  </span>
                  {entry.error_message && (
                    <span className="ml-2 text-xs text-red-500" title={entry.error_message}>
                      (!)
                    </span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      <div className="px-4 py-3 bg-gray-50 border-t border-gray-200 flex items-center justify-between">
        <div className="text-sm text-gray-500">
          Showing {(data.page - 1) * data.page_size + 1} to{' '}
          {Math.min(data.page * data.page_size, data.total)} of {formatNumber(data.total)} entries
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => handlePageChange(data.page - 1)}
            disabled={data.page <= 1}
            className="px-3 py-1.5 text-sm font-medium text-gray-600 bg-white border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Previous
          </button>
          <span className="px-3 py-1.5 text-sm text-gray-600">
            Page {data.page} of {data.total_pages}
          </span>
          <button
            onClick={() => handlePageChange(data.page + 1)}
            disabled={data.page >= data.total_pages}
            className="px-3 py-1.5 text-sm font-medium text-gray-600 bg-white border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Next
          </button>
        </div>
      </div>
    </div>
  );
}
