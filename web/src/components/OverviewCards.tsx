'use client';

import { OverviewStats } from '@/types/api';
import { formatNumber, formatCurrency, formatPercentage, formatDuration } from '@/lib/utils';

interface OverviewCardsProps {
  stats: OverviewStats;
}

export function OverviewCards({ stats }: OverviewCardsProps) {
  const cards = [
    {
      title: 'Total Requests',
      value: formatNumber(stats.total_requests),
      subtitle: `Period: ${stats.period}`,
      color: 'bg-blue-500',
    },
    {
      title: 'Total Tokens',
      value: formatNumber(stats.total_tokens),
      subtitle: 'Input + Output',
      color: 'bg-green-500',
    },
    {
      title: 'Total Cost',
      value: formatCurrency(stats.total_cost),
      subtitle: `Period: ${stats.period}`,
      color: 'bg-amber-600',
    },
    {
      title: 'Avg Latency',
      value: formatDuration(stats.avg_latency_ms),
      subtitle: 'Per request',
      color: 'bg-yellow-500',
    },
    {
      title: 'Error Rate',
      value: formatPercentage(stats.error_rate),
      subtitle: stats.error_rate > 0.05 ? 'Above threshold' : 'Healthy',
      color: stats.error_rate > 0.05 ? 'bg-red-500' : 'bg-emerald-500',
    },
  ];

  return (
    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-4">
      {cards.map((card) => (
        <div
          key={card.title}
          className="bg-white rounded-lg shadow-sm border border-gray-200 p-4 hover:shadow-md transition-shadow"
        >
          <div className="flex items-center gap-2 mb-2">
            <div className={`w-2 h-2 rounded-full ${card.color}`} />
            <span className="text-sm text-gray-600">{card.title}</span>
          </div>
          <div className="text-2xl font-semibold text-gray-900">{card.value}</div>
          <div className="text-xs text-gray-500 mt-1">{card.subtitle}</div>
        </div>
      ))}
    </div>
  );
}
