'use client';

import { useRef, useMemo } from 'react';
import Highcharts from 'highcharts';
import HighchartsReact from 'highcharts-react-official';
import HighchartsHeatmap from 'highcharts/modules/heatmap';
import { HeatmapDataPoint } from '@/types/api';

// Initialize heatmap module
if (typeof Highcharts === 'object') {
  HighchartsHeatmap(Highcharts);
}

interface ActivityHeatmapProps {
  data: HeatmapDataPoint[];
}

export function ActivityHeatmap({ data }: ActivityHeatmapProps) {
  const chartRef = useRef<HighchartsReact.RefObject>(null);

  const { chartData, categories } = useMemo(() => {
    // Group data by week and day of week (GitHub-style layout)
    const weeks: Map<number, HeatmapDataPoint[]> = new Map();

    data.forEach((point) => {
      const date = new Date(point.date);
      const startOfYear = new Date(date.getFullYear(), 0, 1);
      const weekNumber = Math.floor(
        (date.getTime() - startOfYear.getTime()) / (7 * 24 * 60 * 60 * 1000)
      );

      if (!weeks.has(weekNumber)) {
        weeks.set(weekNumber, []);
      }
      weeks.get(weekNumber)!.push(point);
    });

    // Convert to Highcharts format: [x (week), y (day), value]
    const heatmapData: [number, number, number][] = [];

    data.forEach((point) => {
      const date = new Date(point.date);
      const startOfYear = new Date(date.getFullYear(), 0, 1);
      const weekNumber = Math.floor(
        (date.getTime() - startOfYear.getTime()) / (7 * 24 * 60 * 60 * 1000)
      );
      const dayOfWeek = date.getDay();

      heatmapData.push([weekNumber, dayOfWeek, point.count]);
    });

    return {
      chartData: heatmapData,
      categories: ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'],
    };
  }, [data]);

  const options: Highcharts.Options = {
    chart: {
      type: 'heatmap',
      height: 180,
      backgroundColor: 'transparent',
    },
    title: {
      text: undefined,
    },
    credits: {
      enabled: false,
    },
    xAxis: {
      visible: false,
    },
    yAxis: {
      categories: categories,
      title: undefined,
      reversed: true,
      labels: {
        style: {
          fontSize: '10px',
          color: '#6b7280',
        },
      },
    },
    colorAxis: {
      min: 0,
      stops: [
        [0, '#ebedf0'],
        [0.25, '#9be9a8'],
        [0.5, '#40c463'],
        [0.75, '#30a14e'],
        [1, '#216e39'],
      ],
    },
    legend: {
      enabled: false,
    },
    tooltip: {
      formatter: function () {
        const point = this.point as Highcharts.Point & { value: number };
        return `<b>${point.value}</b> requests`;
      },
    },
    series: [
      {
        type: 'heatmap',
        data: chartData,
        borderWidth: 2,
        borderColor: '#ffffff',
        dataLabels: {
          enabled: false,
        },
      },
    ],
  };

  return (
    <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-4">
      <h3 className="text-sm font-medium text-gray-700 mb-4">Activity (Last 365 Days)</h3>
      <div className="overflow-x-auto">
        <HighchartsReact highcharts={Highcharts} options={options} ref={chartRef} />
      </div>
      <div className="flex items-center justify-end gap-1 mt-2 text-xs text-gray-500">
        <span>Less</span>
        <div className="flex gap-0.5">
          {['#ebedf0', '#9be9a8', '#40c463', '#30a14e', '#216e39'].map((color) => (
            <div
              key={color}
              className="w-3 h-3 rounded-sm"
              style={{ backgroundColor: color }}
            />
          ))}
        </div>
        <span>More</span>
      </div>
    </div>
  );
}
