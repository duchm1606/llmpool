'use client';

import { useState } from 'react';
import { ModelStats } from '@/types/api';
import { formatNumber, formatCurrency, formatDuration } from '@/lib/utils';

interface ModelStatsTableProps {
  data: ModelStats[];
}

type SortKey = keyof ModelStats;
type SortDirection = 'asc' | 'desc';

export function ModelStatsTable({ data }: ModelStatsTableProps) {
  const [sortKey, setSortKey] = useState<SortKey>('requests');
  const [sortDirection, setSortDirection] = useState<SortDirection>('desc');

  const handleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc');
    } else {
      setSortKey(key);
      setSortDirection('desc');
    }
  };

  const sortedData = [...data].sort((a, b) => {
    const aVal = a[sortKey];
    const bVal = b[sortKey];
    const multiplier = sortDirection === 'asc' ? 1 : -1;

    if (typeof aVal === 'string' && typeof bVal === 'string') {
      return aVal.localeCompare(bVal) * multiplier;
    }
    return ((aVal as number) - (bVal as number)) * multiplier;
  });

  const SortIcon = ({ column }: { column: SortKey }) => {
    if (sortKey !== column) {
      return <span className="text-gray-300 ml-1">&#8645;</span>;
    }
    return (
      <span className="text-blue-500 ml-1">
        {sortDirection === 'asc' ? '&#8593;' : '&#8595;'}
      </span>
    );
  };

  return (
    <div className="bg-white rounded-lg shadow-sm border border-gray-200 overflow-hidden">
      <div className="px-4 py-3 border-b border-gray-200">
        <h3 className="text-sm font-medium text-gray-700">Model Statistics</h3>
      </div>
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              {[
                { key: 'model', label: 'Model' },
                { key: 'provider', label: 'Provider' },
                { key: 'requests', label: 'Requests' },
                { key: 'tokens_in', label: 'Tokens In' },
                { key: 'tokens_out', label: 'Tokens Out' },
                { key: 'cost', label: 'Cost' },
                { key: 'avg_latency_ms', label: 'Avg Latency' },
                { key: 'error_count', label: 'Errors' },
              ].map(({ key, label }) => (
                <th
                  key={key}
                  onClick={() => handleSort(key as SortKey)}
                  className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider cursor-pointer hover:bg-gray-100"
                >
                  {label}
                  <SortIcon column={key as SortKey} />
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="bg-white divide-y divide-gray-200">
            {sortedData.map((model) => (
              <tr key={`${model.provider}-${model.model}`} className="hover:bg-gray-50">
                <td className="px-4 py-3 whitespace-nowrap text-sm font-medium text-gray-900">
                  {model.model}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
                  <span
                    className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
                      model.provider === 'openai'
                        ? 'bg-green-100 text-green-800'
                        : model.provider === 'anthropic'
                        ? 'bg-orange-100 text-orange-800'
                        : model.provider === 'google'
                        ? 'bg-blue-100 text-blue-800'
                        : 'bg-gray-100 text-gray-800'
                    }`}
                  >
                    {model.provider}
                  </span>
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-900">
                  {formatNumber(model.requests)}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
                  {formatNumber(model.tokens_in)}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
                  {formatNumber(model.tokens_out)}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-900">
                  {formatCurrency(model.cost)}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
                  {formatDuration(model.avg_latency_ms)}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm">
                  <span
                    className={`${
                      model.error_count > 50 ? 'text-red-600' : 'text-gray-500'
                    }`}
                  >
                    {model.error_count}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
