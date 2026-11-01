'use client';

import React, { useId, useMemo, useState } from 'react';
import {
  alignedRows,
  categoryKeys,
  chartSummary,
  extents,
  formatValue,
  plotPath,
  pointText,
  seriesColor,
  ticks,
  valuesCsv,
  type ChartSpec,
} from './chart';

const WIDTH = 680;
const HEIGHT = 300;
const M = { top: 18, right: 18, bottom: 44, left: 62 };
const PLOT_W = WIDTH - M.left - M.right;
const PLOT_H = HEIGHT - M.top - M.bottom;

type XY = { x: number; y: number };

export function Chart({ spec, className = '' }: { spec: ChartSpec; className?: string }) {
  const [copied, setCopied] = useState(false);
  const titleId = useId();
  const rows = useMemo(() => alignedRows(spec), [spec]);
  const keys = useMemo(() => categoryKeys(spec), [spec]);
  const range = useMemo(() => extents(spec), [spec]);
  const compact = !!spec.compact || spec.detail === 'tile';
  const overview = spec.detail === 'overview';
  const low = range.low ?? 0;
  const high = range.high ?? 1;
  const yLow = low === high ? low - 1 : low;
  const yHigh = low === high ? high + 1 : high;
  const ySpan = yHigh - yLow;
  const ticksY = useMemo(() => ticks(yLow, yHigh, compact ? 3 : 4), [yLow, yHigh, compact]);
  const y = (value: number) => M.top + PLOT_H - ((value - yLow) / ySpan) * PLOT_H;
  const xBand = (index: number) => {
    const step = PLOT_W / Math.max(1, keys.length);
    return M.left + step * index + step / 2;
  };
  const xLine = (index: number) =>
    M.left + (keys.length <= 1 ? PLOT_W / 2 : (PLOT_W * index) / (keys.length - 1));

  function copy() {
    void navigator.clipboard.writeText(valuesCsv(spec)).then(
      () => {
        setCopied(true);
        setTimeout(() => setCopied(false), 1600);
      },
      () => setCopied(false),
    );
  }

  return (
    <figure className={`k-chart ${className}`} data-testid="k-chart">
      <header className="k-chart-head">
        <div>
          <figcaption id={titleId}>{spec.title}</figcaption>
          {spec.basis && <p className="k-chart-basis">{spec.basis}</p>}
        </div>
        {spec.headline && (
          <div className="k-chart-headline">
            <strong>{spec.headline.value}</strong>
            <span>{spec.headline.label}</span>
          </div>
        )}
      </header>

      <svg
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        role="img"
        aria-labelledby={titleId}
        aria-label={chartSummary(spec)}
        className="k-chart-svg"
      >
        {!compact && (
          <g aria-hidden="true">
            {ticksY.map((tick) => (
              <g key={tick}>
                <line
                  x1={M.left}
                  x2={WIDTH - M.right}
                  y1={y(tick)}
                  y2={y(tick)}
                  className="k-chart-grid"
                />
                <text x={M.left - 8} y={y(tick) + 4} textAnchor="end" className="k-chart-tick">
                  {formatValue(tick, spec.y)}
                </text>
              </g>
            ))}
          </g>
        )}
        {spec.kind !== 'donut' && spec.kind !== 'radial' && !compact && (
          <text
            x={WIDTH / 2}
            y={HEIGHT - 8}
            textAnchor="middle"
            className="k-chart-axis-label"
            aria-hidden="true"
          >
            {spec.x?.label || ''}
          </text>
        )}

        {spec.kind === 'bars' && (
          <Bars spec={spec} rows={rows} y={y} xBand={xBand} compact={compact} />
        )}
        {spec.kind === 'stacked' && (
          <Stacked spec={spec} rows={rows} y={y} xBand={xBand} compact={compact} />
        )}
        {spec.kind === 'range' && (
          <Ranges spec={spec} rows={rows} y={y} xBand={xBand} compact={compact} />
        )}
        {(spec.kind === 'line' || spec.kind === 'area' || spec.kind === 'spark') && (
          <Lines spec={spec} rows={rows} y={y} xLine={xLine} compact={compact} />
        )}
        {spec.kind === 'donut' && <Donut spec={spec} />}
        {spec.kind === 'radial' && <Radial spec={spec} />}
        {spec.kind === 'radar' && (
          <Radar spec={spec} keys={keys} rows={rows} />
        )}
        {spec.kind === 'heat' && (
          <Heat spec={spec} keys={keys} rows={rows} xBand={xBand} />
        )}

        {!compact && keys.length <= 12 && spec.kind !== 'donut' && spec.kind !== 'radial' && (
          <g aria-hidden="true">
            {keys.map((key, index) => (
              <text
                key={key}
                x={xBand(index)}
                y={HEIGHT - 24}
                textAnchor="middle"
                className="k-chart-tick"
              >
                {key.length > 16 ? `${key.slice(0, 15)}…` : key}
              </text>
            ))}
          </g>
        )}
      </svg>

      {spec.series.length > 1 && spec.kind !== 'heat' && spec.kind !== 'radar' && (
        <div className="k-chart-legend" aria-hidden="true">
          {spec.series.map((series, index) => (
            <span key={series.name}>
              <i style={{ background: seriesColor(index) }} />
              {series.name}
            </span>
          ))}
        </div>
      )}

      {!compact && !overview && spec.values !== false && (
        <details className="k-chart-values">
          <summary>Exact values</summary>
          {spec.basis && <p className="k-chart-basis">{spec.basis}</p>}
          <div className="k-chart-table-wrap">
            <table>
              <caption className="sr-only">{spec.title} exact values</caption>
              <thead>
                <tr>
                  <th scope="col">{spec.x?.label || 'Value'}</th>
                  {spec.series.map((series) => (
                    <th key={series.name} scope="col">
                      {series.name}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.key}>
                    <th scope="row">{row.label}</th>
                    {row.points.map((point, index) => (
                      <td key={`${row.key}-${index}`}>
                        {pointText(point, spec.y)}
                        {point?.evidence?.length ? (
                          <span className="k-chart-evidence">
                            {point.evidence.map((source) => source.label).join(' · ')}
                          </span>
                        ) : null}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <button type="button" className="k-chart-copy" onClick={copy}>
            {copied ? 'Copied' : 'Copy CSV'}
          </button>
        </details>
      )}
    </figure>
  );
}

function Bars({
  spec,
  rows,
  y,
  xBand,
  compact,
}: {
  spec: ChartSpec;
  rows: ReturnType<typeof alignedRows>;
  y: (value: number) => number;
  xBand: (index: number) => number;
  compact: boolean;
}) {
  const count = spec.series.length;
  const step = PLOT_W / Math.max(1, rows.length);
  const group = step * 0.72;
  const bar = group / Math.max(1, count);
  const zero = y(0);
  return (
    <g>
      {rows.map((row, rowIndex) => (
        <g key={row.key}>
          {row.points.map((point, seriesIndex) => {
            if (!point || point.y == null || !Number.isFinite(point.y)) return null;
            const top = Math.min(y(point.y), zero);
            const height = Math.max(1, Math.abs(zero - y(point.y)));
            return (
              <rect
                key={seriesIndex}
                x={xBand(rowIndex) - group / 2 + seriesIndex * bar + 1}
                y={top}
                width={Math.max(1, bar - 2)}
                height={height}
                rx={3}
                fill={seriesColor(seriesIndex)}
              >
                <title>{`${row.label}: ${pointText(point, spec.y)}`}</title>
              </rect>
            );
          })}
        </g>
      ))}
      {!compact && count === 1 && rows.length <= 12 && (
        <g>
          {rows.map((row, index) => {
            const point = row.points[0];
            if (!point || point.y == null) return null;
            return (
              <text
                key={row.key}
                x={xBand(index)}
                y={y(point.y) - 6}
                textAnchor="middle"
                className="k-chart-tick"
              >
                {formatValue(point.y, spec.y)}
              </text>
            );
          })}
        </g>
      )}
    </g>
  );
}

function Stacked({
  spec,
  rows,
  y,
  xBand,
  compact,
}: {
  spec: ChartSpec;
  rows: ReturnType<typeof alignedRows>;
  y: (value: number) => number;
  xBand: (index: number) => number;
  compact: boolean;
}) {
  const step = PLOT_W / Math.max(1, rows.length);
  const width = step * 0.48;
  const zero = y(0);
  return (
    <g>
      {rows.map((row, rowIndex) => {
        let up = 0;
        let down = 0;
        return (
          <g key={row.key}>
            {row.points.map((point, seriesIndex) => {
              if (!point || point.y == null || !Number.isFinite(point.y)) return null;
              const value = point.y;
              const start = value < 0 ? down : up;
              const end = start + value;
              if (value < 0) down = end;
              else up = end;
              const top = y(Math.max(start, end));
              const height = Math.max(1, Math.abs(y(start) - y(end)));
              return (
                <rect
                  key={seriesIndex}
                  x={xBand(rowIndex) - width / 2}
                  y={top}
                  width={width}
                  height={height}
                  fill={seriesColor(seriesIndex)}
                >
                  <title>{`${row.label} · ${spec.series[seriesIndex]?.name}: ${pointText(point, spec.y)}`}</title>
                </rect>
              );
            })}
            {!compact && (
              <text
                x={xBand(rowIndex)}
                y={(up > 0 ? y(up) : zero) - 6}
                textAnchor="middle"
                className="k-chart-tick"
              >
                {formatValue(up || down, spec.y)}
              </text>
            )}
          </g>
        );
      })}
    </g>
  );
}

function Ranges({
  spec,
  rows,
  y,
  xBand,
  compact,
}: {
  spec: ChartSpec;
  rows: ReturnType<typeof alignedRows>;
  y: (value: number) => number;
  xBand: (index: number) => number;
  compact: boolean;
}) {
  const width = Math.max(10, (PLOT_W / Math.max(1, rows.length)) * 0.4);
  return (
    <g>
      {rows.map((row, index) => {
        const point = row.points[0];
        if (!point || point.y == null || point.y2 == null) return null;
        const top = Math.min(y(point.y), y(point.y2));
        const height = Math.max(1, Math.abs(y(point.y2) - y(point.y)));
        return (
          <g key={row.key}>
            <rect x={xBand(index) - width / 2} y={top} width={width} height={height} rx={5} fill={seriesColor(0)}>
              <title>{`${row.label}: ${pointText(point, spec.y)}`}</title>
            </rect>
            {!compact && (
              <text x={xBand(index)} y={top - 6} textAnchor="middle" className="k-chart-tick">
                {pointText(point, spec.y)}
              </text>
            )}
          </g>
        );
      })}
    </g>
  );
}

function Lines({
  spec,
  rows,
  y,
  xLine,
  compact,
}: {
  spec: ChartSpec;
  rows: ReturnType<typeof alignedRows>;
  y: (value: number) => number;
  xLine: (index: number) => number;
  compact: boolean;
}) {
  return (
    <g>
      {spec.series.map((series, seriesIndex) => {
        const segments: { at: XY; label: string; text: string }[][] = [];
        let current: { at: XY; label: string; text: string }[] = [];
        rows.forEach((row, index) => {
          const point = row.points[seriesIndex];
          if (!point || point.y == null || !Number.isFinite(point.y)) {
            if (current.length) segments.push(current);
            current = [];
            return;
          }
          current.push({
            at: { x: xLine(index), y: y(point.y) },
            label: row.label,
            text: pointText(point, spec.y),
          });
        });
        if (current.length) segments.push(current);
        return (
          <g key={series.name}>
            {segments.map((segment, segmentIndex) => {
              const points = segment.map((entry) => entry.at);
              return (
                <g key={segmentIndex}>
                  {spec.kind === 'area' && (
                    <path
                      d={`${plotPath(points)}L${points[points.length - 1]!.x},${y(0)}L${points[0]!.x},${y(0)}Z`}
                      fill={seriesColor(seriesIndex)}
                      opacity={0.12}
                    />
                  )}
                  <path
                    d={plotPath(points)}
                    fill="none"
                    stroke={seriesColor(seriesIndex)}
                    strokeWidth={2}
                    strokeLinecap="round"
                  />
                  {!compact && points.length <= 24 && (
                    <g>
                      {segment.map((entry, pointIndex) => (
                        <circle
                          key={pointIndex}
                          cx={entry.at.x}
                          cy={entry.at.y}
                          r={2.5}
                          fill={seriesColor(seriesIndex)}
                        >
                          <title>{`${entry.label}: ${entry.text}`}</title>
                        </circle>
                      ))}
                    </g>
                  )}
                </g>
              );
            })}
          </g>
        );
      })}
    </g>
  );
}

function Donut({ spec }: { spec: ChartSpec }) {
  const parts = (spec.series[0]?.points ?? [])
    .filter((point) => point.y != null && Number.isFinite(point.y) && point.y > 0)
    .map((point) => ({ label: String(point.x), value: point.y as number }))
    .sort((a, b) => b.value - a.value);
  const total = parts.reduce((sum, part) => sum + part.value, 0) || 1;
  const radius = 74;
  const circumference = 2 * Math.PI * radius;
  const slices = parts.map((part, index) => {
    const length = (part.value / total) * circumference;
    const previous = parts.slice(0, index).reduce((sum, item) => sum + item.value, 0);
    return {
      part,
      index,
      dash: `${Math.max(0, length - 2)} ${circumference}`,
      rotate: -90 + (previous / total) * 360,
    };
  });
  return (
    <g transform={`translate(${WIDTH / 2 - 110} ${HEIGHT / 2})`}>
      <circle r={radius} fill="none" stroke="var(--color-fill)" strokeWidth={22} />
      {slices.map(({ part, index, dash, rotate }) => (
        <circle
          key={part.label}
          r={radius}
          fill="none"
          stroke={seriesColor(index)}
          strokeWidth={22}
          strokeDasharray={dash}
          transform={`rotate(${rotate})`}
        >
          <title>{`${part.label}: ${formatValue(part.value, spec.y)}`}</title>
        </circle>
      ))}
      <text textAnchor="middle" y={4} className="k-chart-donut-total">
        {formatValue(total, spec.y)}
      </text>
      <text textAnchor="middle" y={22} className="k-chart-tick">
        total
      </text>
      <g transform={`translate(150 -74)`}>
        {parts.slice(0, 7).map((part, index) => (
          <g key={part.label} transform={`translate(0 ${index * 20})`}>
            <rect width={8} height={8} rx={2} y={-8} fill={seriesColor(index)} />
            <text x={14} className="k-chart-tick">
              {part.label} · {Math.round((part.value / total) * 100)}%
            </text>
          </g>
        ))}
      </g>
    </g>
  );
}

function Radial({ spec }: { spec: ChartSpec }) {
  const points = (spec.series[0]?.points ?? []).filter((point) => point.y != null);
  const max = spec.y?.max ?? Math.max(1, ...points.map((point) => point.y ?? 0));
  const radius = 96;
  return (
    <g transform={`translate(${WIDTH / 2} ${HEIGHT / 2})`}>
      {points.slice(0, 5).map((point, index) => {
        const ring = radius - index * 16;
        const share = Math.max(0, Math.min(1, (point.y ?? 0) / (max || 1)));
        const circumference = 2 * Math.PI * ring;
        return (
          <g key={`${point.x}-${index}`}>
            <circle r={ring} fill="none" stroke="var(--color-fill)" strokeWidth={8} />
            <circle
              r={ring}
              fill="none"
              stroke={seriesColor(index)}
              strokeWidth={8}
              strokeLinecap="round"
              strokeDasharray={`${circumference * share} ${circumference}`}
              transform="rotate(-90)"
            >
              <title>{`${point.x}: ${pointText(point, spec.y)}`}</title>
            </circle>
            <text x={0} y={-ring + 4} textAnchor="middle" className="k-chart-tick">
              {String(point.x)} · {formatValue(point.y, spec.y)}
            </text>
          </g>
        );
      })}
    </g>
  );
}

function Radar({ spec, keys, rows }: { spec: ChartSpec; keys: string[]; rows: ReturnType<typeof alignedRows> }) {
  const cx = WIDTH / 2;
  const cy = HEIGHT / 2;
  const radius = 100;
  const max = spec.y?.max ?? Math.max(1, ...spec.series.flatMap((series) => series.points.map((point) => point.y ?? 0)));
  const angle = (index: number) => (Math.PI * 2 * index) / Math.max(1, keys.length) - Math.PI / 2;
  const point = (index: number, value: number) => ({
    x: cx + Math.cos(angle(index)) * radius * Math.max(0, Math.min(1, value / (max || 1))),
    y: cy + Math.sin(angle(index)) * radius * Math.max(0, Math.min(1, value / (max || 1))),
  });
  return (
    <g>
      {[0.25, 0.5, 0.75, 1].map((share) => (
        <polygon
          key={share}
          points={keys
            .map((_, index) => {
              const at = point(index, max * share);
              return `${at.x},${at.y}`;
            })
            .join(' ')}
          fill="none"
          stroke="var(--color-border)"
        />
      ))}
      {keys.map((key, index) => {
        const at = point(index, max);
        return (
          <g key={key}>
            <line x1={cx} y1={cy} x2={at.x} y2={at.y} stroke="var(--color-border)" />
            <text
              x={cx + Math.cos(angle(index)) * (radius + 16)}
              y={cy + Math.sin(angle(index)) * (radius + 16) + 4}
              textAnchor="middle"
              className="k-chart-tick"
            >
              {key}
            </text>
          </g>
        );
      })}
      {spec.series.map((series, seriesIndex) => (
        <polygon
          key={series.name}
          points={rows
            .map((row, index) => {
              const value = row.points[seriesIndex]?.y ?? 0;
              const at = point(index, value);
              return `${at.x},${at.y}`;
            })
            .join(' ')}
          fill={seriesColor(seriesIndex)}
          fillOpacity={0.14}
          stroke={seriesColor(seriesIndex)}
          strokeWidth={2}
        />
      ))}
    </g>
  );
}

function Heat({
  spec,
  keys,
  rows,
  xBand,
}: {
  spec: ChartSpec;
  keys: string[];
  rows: ReturnType<typeof alignedRows>;
  xBand: (index: number) => number;
}) {
  const values = rows.flatMap((row) => row.points.map((point) => point?.y ?? null)).filter((value): value is number => value != null);
  const max = Math.max(...values, 1);
  const min = Math.min(...values, 0);
  const step = PLOT_W / Math.max(1, keys.length);
  const rowHeight = PLOT_H / Math.max(1, rows.length);
  return (
    <g>
      {rows.map((row, rowIndex) => (
        <g key={row.key}>
          {row.points.map((point, index) => {
            const share = point?.y == null ? 0 : (point.y - min) / (max - min || 1);
            return (
              <rect
                key={`${row.key}-${index}`}
                x={xBand(index) - step / 2 + 1}
                y={M.top + rowIndex * rowHeight}
                width={Math.max(1, step - 2)}
                height={Math.max(1, rowHeight - 2)}
                rx={3}
                fill={`color-mix(in oklab, var(--color-chart-1) ${Math.round(share * 70)}%, var(--color-surface))`}
              >
                <title>{`${row.label} · ${String(point?.x ?? '')}: ${pointText(point, spec.y)}`}</title>
              </rect>
            );
          })}
          <text x={M.left - 8} y={M.top + rowIndex * rowHeight + rowHeight / 2 + 4} textAnchor="end" className="k-chart-tick">
            {row.label}
          </text>
        </g>
      ))}
    </g>
  );
}
