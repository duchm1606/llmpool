'use client';

import { useMemo } from 'react';
import { HeatmapDataPoint } from '@/types/api';

interface ActivityHeatmapProps {
  data: HeatmapDataPoint[];
}

type GridCell = {
  date: Date;
  dateKey: string;
  count: number;
  inRange: boolean;
};

type WeekColumn = {
  start: Date;
  cells: GridCell[];
};

const DAY_MS = 24 * 60 * 60 * 1000;
const MONTH_LABELS = ['J', 'F', 'M', 'A', 'M', 'J', 'J', 'A', 'S', 'O', 'N', 'D'];
const DOW_LABELS = ['', 'M', '', 'W', '', 'F', ''];

function toUTCDate(dateString: string): Date {
  const [y, m, d] = dateString.split('-').map((part) => Number(part));
  return new Date(Date.UTC(y, (m || 1) - 1, d || 1));
}

function dateKeyUTC(date: Date): string {
  return date.toISOString().slice(0, 10);
}

function startOfUTCDay(date: Date): Date {
  return new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate()));
}

function startOfWeekSundayUTC(date: Date): Date {
  const day = date.getUTCDay();
  return new Date(date.getTime() - day * DAY_MS);
}

function addDaysUTC(date: Date, days: number): Date {
  return new Date(date.getTime() + days * DAY_MS);
}

function formatFullDate(date: Date): string {
  return new Intl.DateTimeFormat('en-US', {
    weekday: 'long',
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    timeZone: 'UTC',
  }).format(date);
}

function activityLevelColor(count: number, maxCount: number): string {
  if (count <= 0 || maxCount <= 0) {
    return '#ebedf0';
  }

  const ratio = count / maxCount;
  if (ratio <= 0.25) {
    return '#9be9a8';
  }
  if (ratio <= 0.5) {
    return '#40c463';
  }
  if (ratio <= 0.75) {
    return '#30a14e';
  }
  return '#216e39';
}

export function ActivityHeatmap({ data }: ActivityHeatmapProps) {
  const model = useMemo(() => {
    const dateCount = new Map<string, number>();
    let firstDate: Date | null = null;
    let lastDate: Date | null = null;
    let maxCount = 0;

    for (const point of data) {
      const day = toUTCDate(point.date);
      const key = dateKeyUTC(day);
      const count = Number.isFinite(point.count) ? Math.max(0, point.count) : 0;

      dateCount.set(key, count);
      if (!firstDate || day < firstDate) {
        firstDate = day;
      }
      if (!lastDate || day > lastDate) {
        lastDate = day;
      }
      if (count > maxCount) {
        maxCount = count;
      }
    }

    if (!firstDate || !lastDate) {
      return {
        columns: [] as WeekColumn[],
        monthLabels: [] as string[],
        maxCount: 0,
      };
    }

    const normalizedStart = startOfUTCDay(firstDate);
    const normalizedEnd = startOfUTCDay(lastDate);
    const start = startOfWeekSundayUTC(normalizedStart);
    const end = startOfWeekSundayUTC(normalizedEnd);

    const columns: WeekColumn[] = [];
    for (let cursor = start; cursor <= end; cursor = addDaysUTC(cursor, 7)) {
      const cells: GridCell[] = [];

      for (let dayOffset = 0; dayOffset < 7; dayOffset++) {
        const current = addDaysUTC(cursor, dayOffset);
        const key = dateKeyUTC(current);
        const inRange = current >= normalizedStart && current <= normalizedEnd;
        cells.push({
          date: current,
          dateKey: key,
          count: inRange ? dateCount.get(key) || 0 : 0,
          inRange,
        });
      }

      columns.push({
        start: cursor,
        cells,
      });
    }

    const monthLabels = columns.map((column, index) => {
      if (index === 0) {
        return MONTH_LABELS[column.start.getUTCMonth()];
      }

      const prevMonth = columns[index - 1].start.getUTCMonth();
      const currentMonth = column.start.getUTCMonth();
      if (currentMonth !== prevMonth) {
        return MONTH_LABELS[currentMonth];
      }
      return '';
    });

    return { columns, monthLabels, maxCount };
  }, [data]);

  return (
    <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-4">
      <h3 className="text-sm font-medium text-gray-700 mb-4">Activity (Last 365 Days)</h3>

      {model.columns.length === 0 ? (
        <div className="text-sm text-gray-500">No activity data.</div>
      ) : (
        <div className="overflow-x-auto">
          <div className="inline-block min-w-max whitespace-nowrap mx-auto">
            <div className="flex mb-2">
              <div className="w-8 mr-2" />
              {model.monthLabels.map((label, idx) => (
                <div key={`${label}-${idx}`} className="w-3 mr-1">
                  {label ? <span className="text-xs text-gray-500">{label}</span> : null}
                </div>
              ))}
            </div>

            <div className="flex items-start">
              <div className="flex flex-col mr-2">
                {DOW_LABELS.map((label, index) => (
                  <div key={`${label}-${index}`} className="h-3 mb-1 flex items-center justify-end">
                    <span className="text-xs text-gray-500 w-8 text-right pr-1">{label}</span>
                  </div>
                ))}
              </div>

              {model.columns.map((column) => (
                <div key={column.start.toISOString()} className="flex flex-col mr-1 flex-shrink-0">
                  {column.cells.map((cell) => {
                    if (!cell.inRange) {
                      return <div key={cell.dateKey} className="w-3 h-3 mb-1" />;
                    }

                    const color = activityLevelColor(cell.count, model.maxCount);
                    const detail =
                      cell.count === 0
                        ? 'No requests'
                        : `${cell.count.toLocaleString('en-US')} request${cell.count > 1 ? 's' : ''}`;

                    return (
                      <div
                        key={cell.dateKey}
                        className="w-3 h-3 mb-1 rounded-sm cursor-pointer transition-all hover:ring-2 hover:ring-gray-300"
                        style={{ backgroundColor: color }}
                        title={`${formatFullDate(cell.date)}\n${detail}`}
                      />
                    );
                  })}
                </div>
              ))}
            </div>

            <div className="flex items-center justify-center gap-1 mt-2 text-xs text-gray-500">
              <span>Less</span>
              <div className="flex gap-0.5">
                {['#ebedf0', '#9be9a8', '#40c463', '#30a14e', '#216e39'].map((color) => (
                  <div key={color} className="w-3 h-3 rounded-sm" style={{ backgroundColor: color }} />
                ))}
              </div>
              <span>More</span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
