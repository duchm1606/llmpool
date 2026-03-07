'use client';

import { useState, useMemo } from 'react';
import Highcharts from 'highcharts';
import HighchartsReact from 'highcharts-react-official';
import { TimeSeriesPoint } from '@/types/api';
import { formatNumber, formatCurrency } from '@/lib/utils';

interface UsageChartProps {
  data: TimeSeriesPoint[];
  granularity: 'hourly' | 'daily';
  onGranularityChange: (granularity: 'hourly' | 'daily') => void;
}

type MetricType = 'requests' | 'tokens' | 'cost';

export function UsageChart({ data, granularity, onGranularityChange }: UsageChartProps) {
  const [metric, setMetric] = useState<MetricType>('requests');

  const chartOptions: Highcharts.Options = useMemo(() => {
    const seriesData = data.map((point) => ({
      x: new Date(point.timestamp).getTime(),
      y: point[metric],
    }));

    const formatValue = (value: number) => {
      if (metric === 'cost') return formatCurrency(value);
      return formatNumber(value);
    };

    const colors = {
      requests: '#3b82f6',
      tokens: '#10b981',
      cost: '#8b5cf6',
    };

    return {
      chart: {
        type: 'areaspline',
        height: 300,
        backgroundColor: 'transparent',
      },
      title: {
        text: undefined,
      },
      credits: {
        enabled: false,
      },
      xAxis: {
        type: 'datetime',
        labels: {
          style: {
            color: '#6b7280',
            fontSize: '11px',
          },
        },
        lineColor: '#e5e7eb',
        tickColor: '#e5e7eb',
      },
      yAxis: {
        title: {
          text: undefined,
        },
        labels: {
          formatter: function () {
            return formatValue(this.value as number);
          },
          style: {
            color: '#6b7280',
            fontSize: '11px',
          },
        },
        gridLineColor: '#f3f4f6',
      },
      tooltip: {
        shared: true,
        formatter: function () {
          const date = Highcharts.dateFormat(
            granularity === 'hourly' ? '%b %e, %H:%M' : '%b %e, %Y',
            this.x as number
          );
          return `<b>${date}</b><br/>${formatValue(this.y as number)}`;
        },
      },
      legend: {
        enabled: false,
      },
      plotOptions: {
        areaspline: {
          fillOpacity: 0.2,
          lineWidth: 2,
          marker: {
            enabled: false,
            symbol: 'circle',
            radius: 3,
            states: {
              hover: {
                enabled: true,
              },
            },
          },
        },
      },
      series: [
        {
          type: 'areaspline',
          name: metric.charAt(0).toUpperCase() + metric.slice(1),
          data: seriesData,
          color: colors[metric],
        },
      ],
    };
  }, [data, metric, granularity]);

  return (
    <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-4">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-4">
        <h3 className="text-sm font-medium text-gray-700">Usage Over Time</h3>
        <div className="flex flex-wrap gap-2">
          <div className="inline-flex rounded-md shadow-sm">
            {(['requests', 'tokens', 'cost'] as MetricType[]).map((m) => (
              <button
                key={m}
                onClick={() => setMetric(m)}
                className={`px-3 py-1.5 text-xs font-medium border ${
                  metric === m
                    ? 'bg-blue-50 text-blue-600 border-blue-200'
                    : 'bg-white text-gray-600 border-gray-200 hover:bg-gray-50'
                } ${m === 'requests' ? 'rounded-l-md' : ''} ${
                  m === 'cost' ? 'rounded-r-md' : ''
                } ${m !== 'requests' ? '-ml-px' : ''}`}
              >
                {m.charAt(0).toUpperCase() + m.slice(1)}
              </button>
            ))}
          </div>
          <div className="inline-flex rounded-md shadow-sm">
            {(['hourly', 'daily'] as const).map((g) => (
              <button
                key={g}
                onClick={() => onGranularityChange(g)}
                className={`px-3 py-1.5 text-xs font-medium border ${
                  granularity === g
                    ? 'bg-blue-50 text-blue-600 border-blue-200'
                    : 'bg-white text-gray-600 border-gray-200 hover:bg-gray-50'
                } ${g === 'hourly' ? 'rounded-l-md' : 'rounded-r-md -ml-px'}`}
              >
                {g.charAt(0).toUpperCase() + g.slice(1)}
              </button>
            ))}
          </div>
        </div>
      </div>
      <HighchartsReact highcharts={Highcharts} options={chartOptions} />
    </div>
  );
}
