'use client';

import React from 'react';
import {
  alignedRows,
  formatValue,
  plotPath,
  pointText,
  seriesColor,
  type ChartHover,
  type ChartPoint,
  type ChartSpec,
} from './chart';
import {
  CHART_HEIGHT as HEIGHT,
  CHART_MARGINS as M,
  CHART_PLOT_H as PLOT_H,
  CHART_PLOT_W as PLOT_W,
  CHART_WIDTH as WIDTH,
} from './chart-layout';

type XY = { x: number; y: number };

export function Bars({
  spec,
  rows,
  y,
  xBand,
  yBand,
  xVal,
  horizontal,
  plotW,
  plotH,
  compact,
  onHover,
  onLeave,
  formatEvidence,
}: {
  spec: ChartSpec;
  rows: ReturnType<typeof alignedRows>;
  y: (value: number) => number;
  xBand: (index: number) => number;
  yBand: (index: number) => number;
  xVal: (value: number) => number;
  horizontal: boolean;
  plotW: number;
  plotH: number;
  compact: boolean;
  onHover: (hover: ChartHover) => void;
  onLeave: () => void;
  formatEvidence: (point?: ChartPoint) => string | undefined;
}) {
  const count = spec.series.length;
  if (horizontal) {
    const step = plotH / Math.max(1, rows.length);
    const group = step * 0.72;
    const bar = group / Math.max(1, count);
    const zero = xVal(0);
    return (
      <g>
        {rows.map((row, rowIndex) => (
          <g key={row.key}>
            {row.points.map((point, seriesIndex) => {
              if (!point || point.y == null || !Number.isFinite(point.y)) return null;
              const valX = xVal(point.y);
              const left = Math.min(valX, zero);
              const width = Math.max(2, Math.abs(valX - zero));
              const top = yBand(rowIndex) - group / 2 + seriesIndex * bar + 1;
              const value = pointText(point, spec.y);
              const series = spec.series[seriesIndex]?.name || spec.title;
              const evidence = formatEvidence(point);
              return (
                <rect
                  key={seriesIndex}
                  x={left}
                  y={top}
                  width={width}
                  height={Math.max(2, bar - 2)}
                  rx={3}
                  fill={seriesColor(seriesIndex)}
                  tabIndex={0}
                  onPointerEnter={() => onHover({ label: row.label, series, value, evidence, sources: point?.evidence, px: left + width, py: top })}
                  onPointerLeave={onLeave}
                  onFocus={() => onHover({ label: row.label, series, value, evidence, sources: point?.evidence, px: left + width, py: top })}
                  onBlur={onLeave}
                >
                  <title>{`${row.label}: ${value}`}</title>
                </rect>
              );
            })}
          </g>
        ))}
        {!compact && count === 1 && rows.length <= 14 && (
          <g>
            {rows.map((row, index) => {
              const point = row.points[0];
              if (!point || point.y == null) return null;
              return (
                <text
                  key={row.key}
                  x={Math.max(xVal(point.y), zero) + 6}
                  y={yBand(index) + 4}
                  textAnchor="start"
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
  const step = plotW / Math.max(1, rows.length);
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
            const left = xBand(rowIndex) - group / 2 + seriesIndex * bar + 1;
            const value = pointText(point, spec.y);
            const series = spec.series[seriesIndex]?.name || spec.title;
            const evidence = formatEvidence(point);
            return (
              <rect
                key={seriesIndex}
                x={left}
                y={top}
                width={Math.max(1, bar - 2)}
                height={height}
                rx={3}
                fill={seriesColor(seriesIndex)}
                tabIndex={0}
                onPointerEnter={() => onHover({ label: row.label, series, value, evidence, sources: point?.evidence, px: left + bar / 2, py: top })}
                onPointerLeave={onLeave}
                onFocus={() => onHover({ label: row.label, series, value, evidence, sources: point?.evidence, px: left + bar / 2, py: top })}
                onBlur={onLeave}
              >
                <title>{`${row.label}: ${value}`}</title>
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

export function Stacked({
  spec,
  rows,
  y,
  xBand,
  yBand,
  xVal,
  horizontal,
  plotW,
  plotH,
  compact,
  onHover,
  onLeave,
  formatEvidence,
}: {
  spec: ChartSpec;
  rows: ReturnType<typeof alignedRows>;
  y: (value: number) => number;
  xBand: (index: number) => number;
  yBand: (index: number) => number;
  xVal: (value: number) => number;
  horizontal: boolean;
  plotW: number;
  plotH: number;
  compact: boolean;
  onHover: (hover: ChartHover) => void;
  onLeave: () => void;
  formatEvidence: (point?: ChartPoint) => string | undefined;
}) {
  if (horizontal) {
    const step = plotH / Math.max(1, rows.length);
    const height = Math.max(4, step * 0.48);
    const zero = xVal(0);
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
                const left = xVal(Math.min(start, end));
                const width = Math.max(1, Math.abs(xVal(start) - xVal(end)));
                const top = yBand(rowIndex) - height / 2;
                const display = pointText(point, spec.y);
                const series = spec.series[seriesIndex]?.name || spec.title;
                const evidence = formatEvidence(point);
                return (
                  <rect
                    key={seriesIndex}
                    x={left}
                    y={top}
                    width={width}
                    height={height}
                    fill={seriesColor(seriesIndex)}
                    tabIndex={0}
                    onPointerEnter={() => onHover({ label: row.label, series, value: display, evidence, sources: point?.evidence, px: left + width, py: top })}
                    onPointerLeave={onLeave}
                    onFocus={() => onHover({ label: row.label, series, value: display, evidence, sources: point?.evidence, px: left + width, py: top })}
                    onBlur={onLeave}
                  >
                    <title>{`${row.label} · ${series}: ${display}`}</title>
                  </rect>
                );
              })}
              {!compact && (
                <text
                  x={(up > 0 ? xVal(up) : zero) + 6}
                  y={yBand(rowIndex) + 4}
                  textAnchor="start"
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
  const step = plotW / Math.max(1, rows.length);
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
              const left = xBand(rowIndex) - width / 2;
              const display = pointText(point, spec.y);
              const series = spec.series[seriesIndex]?.name || spec.title;
              const evidence = formatEvidence(point);
              return (
                <rect
                  key={seriesIndex}
                  x={left}
                  y={top}
                  width={width}
                  height={height}
                  fill={seriesColor(seriesIndex)}
                  tabIndex={0}
                  onPointerEnter={() => onHover({ label: row.label, series, value: display, evidence, px: left + width / 2, py: top })}
                  onPointerLeave={onLeave}
                  onFocus={() => onHover({ label: row.label, series, value: display, evidence, px: left + width / 2, py: top })}
                  onBlur={onLeave}
                >
                  <title>{`${row.label} · ${series}: ${display}`}</title>
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

export function Ranges({
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

export function Lines({
  spec,
  rows,
  y,
  yClamp,
  plotTop,
  plotBottom,
  xLine,
  compact,
  onHover,
  onLeave,
  formatEvidence,
}: {
  spec: ChartSpec;
  rows: ReturnType<typeof alignedRows>;
  y: (value: number) => number;
  yClamp: (value: number) => number;
  plotTop: number;
  plotBottom: number;
  xLine: (index: number) => number;
  compact: boolean;
  onHover: (hover: ChartHover) => void;
  onLeave: () => void;
  formatEvidence: (point?: ChartPoint) => string | undefined;
}) {
  const baseline = yClamp(0);
  const clamped = Boolean(spec.y?.clamp);
  return (
    <g>
      {spec.series.map((series, seriesIndex) => {
        const segments: { at: XY; label: string; text: string; evidence?: string }[][] = [];
        const outliers: { at: XY; label: string; text: string; evidence?: string }[] = [];
        let current: { at: XY; label: string; text: string; evidence?: string }[] = [];
        rows.forEach((row, index) => {
          const point = row.points[seriesIndex];
          if (!point || point.y == null || !Number.isFinite(point.y)) {
            if (current.length) segments.push(current);
            current = [];
            return;
          }
          const rawY = y(point.y);
          const offScale = clamped && (rawY < plotTop - 2 || rawY > plotBottom + 2);
          if (offScale) {
            if (current.length) segments.push(current);
            current = [];
            outliers.push({
              at: { x: xLine(index), y: rawY < plotTop ? plotTop + 5 : plotBottom - 5 },
              label: row.label,
              text: `${pointText(point, spec.y)} (off-scale)`,
              evidence: formatEvidence(point),
            });
            return;
          }
          current.push({
            at: { x: xLine(index), y: rawY },
            label: row.label,
            text: pointText(point, spec.y),
            evidence: formatEvidence(point),
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
                      d={`${plotPath(points)}L${points[points.length - 1]!.x},${baseline}L${points[0]!.x},${baseline}Z`}
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
                  {!compact && segment.length <= 48 && (
                    <g>
                      {segment.map((entry, pointIndex) => (
                        <circle
                          key={pointIndex}
                          cx={entry.at.x}
                          cy={entry.at.y}
                          r={3}
                          fill={seriesColor(seriesIndex)}
                          tabIndex={0}
                          onPointerEnter={() => onHover({
                            label: entry.label,
                            series: series.name,
                            value: entry.text,
                            evidence: entry.evidence,
                            px: entry.at.x,
                            py: entry.at.y,
                          })}
                          onPointerLeave={onLeave}
                          onFocus={() => onHover({
                            label: entry.label,
                            series: series.name,
                            value: entry.text,
                            evidence: entry.evidence,
                            px: entry.at.x,
                            py: entry.at.y,
                          })}
                          onBlur={onLeave}
                        >
                          <title>{`${entry.label}: ${entry.text}`}</title>
                        </circle>
                      ))}
                    </g>
                  )}
                </g>
              );
            })}
            {!compact && outliers.map((entry, idx) => (
              <polygon
                key={`outlier-${idx}`}
                data-outlier="true"
                points={`${entry.at.x},${entry.at.y - 4.5} ${entry.at.x + 4.5},${entry.at.y} ${entry.at.x},${entry.at.y + 4.5} ${entry.at.x - 4.5},${entry.at.y}`}
                fill="var(--color-warning)"
                stroke="var(--color-surface)"
                strokeWidth={1}
                tabIndex={0}
                onPointerEnter={() => onHover({
                  label: entry.label,
                  series: series.name,
                  value: entry.text,
                  evidence: entry.evidence,
                  px: entry.at.x,
                  py: entry.at.y,
                })}
                onPointerLeave={onLeave}
                onFocus={() => onHover({
                  label: entry.label,
                  series: series.name,
                  value: entry.text,
                  evidence: entry.evidence,
                  px: entry.at.x,
                  py: entry.at.y,
                })}
                onBlur={onLeave}
              >
                <title>{`${entry.label}: ${entry.text}`}</title>
              </polygon>
            ))}
          </g>
        );
      })}
    </g>
  );
}

export function Donut({ spec }: { spec: ChartSpec }) {
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

export function Radial({ spec }: { spec: ChartSpec }) {
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

export function Radar({ spec, keys, rows }: { spec: ChartSpec; keys: string[]; rows: ReturnType<typeof alignedRows> }) {
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

export function Heat({
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

export function ChartHeader({ spec, embedded, titleId }: { spec: ChartSpec; embedded: boolean; titleId: string }) {
  return (
    <header className={`k-chart-head${embedded ? ' k-chart-head-embedded' : ''}`}>
      <div className={embedded ? 'sr-only' : undefined}>
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
  );
}
