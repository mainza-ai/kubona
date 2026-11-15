'use client';

import React, { useEffect, useId, useMemo, useRef, useState } from 'react';
import {
  alignedRows,
  categoryKeys,
  chartSummary,
  formatValue,
  hasChartValues,
  pointText,
  seriesColor,
  valuesCsv,
  type ChartHover,
  type ChartPoint,
  type ChartSpec,
} from './chart';
import {
  CHART_MARGINS as M,
  CHART_WIDTH as WIDTH,
  computeChartLayout,
} from './chart-layout';
import { Bars, ChartHeader, Donut, Heat, Lines, Radar, Radial, Ranges, Stacked } from './chart-parts';


export function Chart({
  spec,
  className = '',
  embedded = false,
  onEvidence,
}: {
  spec: ChartSpec;
  className?: string;
  embedded?: boolean;
  /** D5: evidence becomes an actionable button instead of dead text. */
  onEvidence?: (source: NonNullable<ChartPoint['evidence']>[number]) => void;
}) {
  const [copied, setCopied] = useState(false);
  const [hovered, setHovered] = useState<ChartHover | null>(null);
  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const titleId = useId();
  const summaryId = useId();
  const hasValues = useMemo(() => hasChartValues(spec), [spec]);
  const rows = useMemo(() => alignedRows(spec), [spec]);
  const keys = useMemo(() => categoryKeys(spec), [spec]);
  const compact = !!spec.compact || spec.detail === 'tile';
  const layout = useMemo(() => computeChartLayout(spec, rows, keys, compact), [spec, rows, keys, compact]);
  const {
    overview,
    horizontal,
    svgHeight,
    plotLeft,
    plotW,
    plotH,
    maxHorizontalLabelChars,
    ticksY,
    xTickIndices,
    y,
    xVal,
    xBand,
    yBand,
    xLine,
  } = layout;

  useEffect(() => () => {
    if (hideTimer.current) clearTimeout(hideTimer.current);
  }, []);

  const showHover = (next: ChartHover) => {
    if (hideTimer.current) {
      clearTimeout(hideTimer.current);
      hideTimer.current = null;
    }
    setHovered(next);
  };

  const scheduleHide = () => {
    if (hideTimer.current) clearTimeout(hideTimer.current);
    hideTimer.current = setTimeout(() => {
      hideTimer.current = null;
      setHovered(null);
    }, 160);
  };

  const formatEvidence = (point?: ChartPoint) =>
    point?.evidence?.length ? point.evidence.map((source) => source.label).join(' · ') : undefined;

  function copy() {
    void navigator.clipboard.writeText(valuesCsv(spec)).then(
      () => {
        setCopied(true);
        setTimeout(() => setCopied(false), 1600);
      },
      () => setCopied(false),
    );
  }

  if (!hasValues) {
    return (
      <figure className={`k-chart ${className}`} data-testid="k-chart" data-unavailable="true">
        <ChartHeader spec={spec} embedded={embedded} titleId={titleId} />
        <div className="k-chart-unavailable" role="status" data-testid="k-chart-unavailable">
          <strong>Chart unavailable</strong>
          <p>{spec.basis || 'No measured values are recorded yet.'}</p>
        </div>
      </figure>
    );
  }

  return (
    <figure className={`k-chart ${className}`} data-testid="k-chart" data-horizontal={horizontal ? 'true' : undefined}>
      <ChartHeader spec={spec} embedded={embedded} titleId={titleId} />

      <div className="k-chart-canvas">
        <svg
          viewBox={`0 0 ${WIDTH} ${svgHeight}`}
          role="img"
          aria-labelledby={titleId}
          aria-describedby={summaryId}
          className="k-chart-svg"
        >
          <desc id={summaryId}>{chartSummary(spec)}</desc>
          {!compact && !horizontal && (
            <g aria-hidden="true">
              {ticksY.map((tick) => (
                <g key={tick}>
                  <line
                    x1={plotLeft}
                    x2={WIDTH - M.right}
                    y1={y(tick)}
                    y2={y(tick)}
                    className="k-chart-grid"
                  />
                  <text x={plotLeft - 8} y={y(tick) + 4} textAnchor="end" className="k-chart-tick">
                    {formatValue(tick, spec.y)}
                  </text>
                </g>
              ))}
            </g>
          )}
          {!compact && horizontal && (
            <g aria-hidden="true">
              {ticksY.map((tick) => (
                <g key={tick}>
                  <line
                    x1={xVal(tick)}
                    x2={xVal(tick)}
                    y1={M.top}
                    y2={M.top + plotH}
                    className="k-chart-grid"
                  />
                  <text x={xVal(tick)} y={svgHeight - 22} textAnchor="middle" className="k-chart-tick">
                    {formatValue(tick, spec.y)}
                  </text>
                </g>
              ))}
            </g>
          )}
          {spec.kind !== 'donut' && spec.kind !== 'radial' && !compact && (
            <text
              x={plotLeft + plotW / 2}
              y={svgHeight - 6}
              textAnchor="middle"
              className="k-chart-axis-label"
              aria-hidden="true"
            >
              {horizontal ? (spec.y?.label || '') : (spec.x?.label || '')}
            </text>
          )}

          {spec.kind === 'bars' && (
            <Bars
              spec={spec}
              rows={rows}
              y={y}
              xBand={xBand}
              yBand={yBand}
              xVal={xVal}
              horizontal={horizontal}
              plotH={plotH}
              plotW={plotW}
              compact={compact}
              onHover={showHover}
              onLeave={scheduleHide}
              formatEvidence={formatEvidence}
            />
          )}
          {spec.kind === 'stacked' && (
            <Stacked
              spec={spec}
              rows={rows}
              y={y}
              xBand={xBand}
              yBand={yBand}
              xVal={xVal}
              horizontal={horizontal}
              plotH={plotH}
              plotW={plotW}
              compact={compact}
              onHover={showHover}
              onLeave={scheduleHide}
              formatEvidence={formatEvidence}
            />
          )}
          {spec.kind === 'range' && (
            <Ranges spec={spec} rows={rows} y={y} xBand={xBand} compact={compact} />
          )}
          {(spec.kind === 'line' || spec.kind === 'area' || spec.kind === 'spark') && (
            <Lines
              spec={spec}
              rows={rows}
              y={y}
              yClamp={(val) => Math.max(M.top, Math.min(M.top + plotH, y(val)))}
              plotTop={M.top}
              plotBottom={M.top + plotH}
              xLine={xLine}
              compact={compact}
              onHover={showHover}
              onLeave={scheduleHide}
              formatEvidence={formatEvidence}
            />
          )}
          {spec.kind === 'donut' && <Donut spec={spec} />}
          {spec.kind === 'radial' && <Radial spec={spec} />}
          {spec.kind === 'radar' && (
            <Radar spec={spec} keys={keys} rows={rows} />
          )}
          {spec.kind === 'heat' && (
            <Heat spec={spec} keys={keys} rows={rows} xBand={xBand} />
          )}

          {!compact && horizontal && (
            <g aria-hidden="true">
              {keys.map((key, index) => (
                <text
                  key={key}
                  x={plotLeft - 8}
                  y={yBand(index) + 4}
                  textAnchor="end"
                  className="k-chart-tick"
                >
                  {key.length > maxHorizontalLabelChars ? `${key.slice(0, maxHorizontalLabelChars - 1)}…` : key}
                </text>
              ))}
            </g>
          )}
          {!compact && !horizontal && spec.kind !== 'donut' && spec.kind !== 'radial' && (
            <g aria-hidden="true">
              {xTickIndices.map((index) => {
                const key = keys[index]!;
                const xPos = spec.kind === 'line' || spec.kind === 'area' || spec.kind === 'spark' ? xLine(index) : xBand(index);
                return (
                  <text
                    key={key}
                    x={xPos}
                    y={svgHeight - 24}
                    textAnchor="middle"
                    className="k-chart-tick"
                  >
                    {key.length > maxHorizontalLabelChars ? `${key.slice(0, maxHorizontalLabelChars - 1)}…` : key}
                  </text>
                );
              })}
            </g>
          )}
        </svg>

        {hovered && (
          <div
            role="status"
            data-testid="k-chart-tip"
            className="k-chart-tip"
            style={{
              left: `${Math.max(10, Math.min(90, (hovered.px / WIDTH) * 100))}%`,
              top: `${Math.max(8, Math.min(82, (hovered.py / svgHeight) * 100))}%`,
            }}
            onPointerEnter={() => {
              if (hideTimer.current) {
                clearTimeout(hideTimer.current);
                hideTimer.current = null;
              }
            }}
            onPointerLeave={scheduleHide}
          >
            <div className="k-chart-tip-head">{hovered.label} · {hovered.series}</div>
            <strong>{hovered.value}</strong>
            {hovered.sources && onEvidence ? (
              <span className="k-chart-tip-evidence">
                {hovered.sources.map((source) => (
                  <button
                    key={source.key}
                    type="button"
                    data-testid="k-chart-evidence-button"
                    onClick={() => onEvidence(source)}
                  >
                    {source.label}
                  </button>
                ))}
              </span>
            ) : (
              hovered.evidence && <span className="k-chart-tip-evidence">{hovered.evidence}</span>
            )}
          </div>
        )}
      </div>

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

      {!overview && spec.values !== false && (
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
                            {onEvidence
                              ? point.evidence.map((source) => (
                                  <button
                                    key={source.key}
                                    type="button"
                                    data-testid="k-chart-evidence-button"
                                    onClick={() => onEvidence(source)}
                                  >
                                    {source.label}
                                  </button>
                                ))
                              : point.evidence.map((source) => source.label).join(' · ')}
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
