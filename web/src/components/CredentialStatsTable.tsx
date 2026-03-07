'use client';

import { useState } from 'react';
import { CredentialStats } from '@/types/api';
import { formatNumber, formatCurrency } from '@/lib/utils';

interface CredentialStatsTableProps {
  data: CredentialStats[];
}

type SortKey = keyof CredentialStats;
type SortDirection = 'asc' | 'desc';

export function CredentialStatsTable({ data }: CredentialStatsTableProps) {
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
        <h3 className="text-sm font-medium text-gray-700">Credential Account Usage</h3>
      </div>
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              {[
                { key: 'credential_type', label: 'Type' },
                { key: 'credential_account_id', label: 'Account' },
                { key: 'credential_id', label: 'Credential ID' },
                { key: 'requests', label: 'Requests' },
                { key: 'tokens_in', label: 'Tokens In' },
                { key: 'tokens_out', label: 'Tokens Out' },
                { key: 'total_tokens', label: 'Total Tokens' },
                { key: 'cost', label: 'Cost' },
                { key: 'error_count', label: 'Errors' },
                { key: 'canceled_count', label: 'Canceled' },
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
            {sortedData.map((credential) => (
              <tr
                key={`${credential.credential_type}-${credential.credential_account_id}-${credential.credential_id}`}
                className="hover:bg-gray-50"
              >
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-900 capitalize">
                  {credential.credential_type || 'unknown'}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500 font-mono">
                  {credential.credential_account_id || '-'}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500 font-mono">
                  {credential.credential_id || '-'}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-900">
                  {formatNumber(credential.requests)}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
                  {formatNumber(credential.tokens_in)}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
                  {formatNumber(credential.tokens_out)}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-900">
                  {formatNumber(credential.total_tokens)}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-900">
                  {formatCurrency(credential.cost)}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
                  {formatNumber(credential.error_count)}
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
                  {formatNumber(credential.canceled_count)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
