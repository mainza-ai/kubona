'use client';

import React from 'react';

interface SpeedometerGaugeProps {
  value: number;
  min?: number;
  max?: number;
  label: string;
  unit?: string;
  thresholds?: {
    warning?: number;
    danger?: number;
  };
  colorScheme?: 'cyan' | 'emerald' | 'amber' | 'violet';
  size?: number;
  subtext?: string;
}

export function SpeedometerGauge({
  value,
  min = 0,
  max = 100,
  label,
  unit = '',
  thresholds,
  colorScheme = 'cyan',
  size = 140,
  subtext,
}: SpeedometerGaugeProps) {
  const clamped = Math.max(min, Math.min(max, value));
  const fraction = (clamped - min) / (max - min || 1);

  // Gauge sweeps 220 degrees (-110 deg to +110 deg)
  const angle = -110 + fraction * 220;

  const colorMap = {
    cyan: { stroke: '#00f0ff', glow: 'rgba(0, 240, 255, 0.4)', text: 'text-cyan-400' },
    emerald: { stroke: '#10b981', glow: 'rgba(16, 185, 129, 0.4)', text: 'text-emerald-400' },
    amber: { stroke: '#f59e0b', glow: 'rgba(245, 158, 11, 0.4)', text: 'text-amber-400' },
    violet: { stroke: '#8a2be2', glow: 'rgba(138, 43, 226, 0.4)', text: 'text-purple-400' },
  };

  const scheme = colorMap[colorScheme] || colorMap.cyan;
  const radius = size * 0.38;
  const strokeWidth = size * 0.08;
  const center = size / 2;
  const circumference = 2 * Math.PI * radius;
  // Sweep arc is 220 / 360 = ~61.1% of circumference
  const arcLength = circumference * (220 / 360);
  const strokeDashoffset = arcLength * (1 - fraction);

  return (
    <div className="flex flex-col items-center justify-center p-3 rounded-xl bg-[#0A0D14]/90 border border-slate-800/80 shadow-lg relative overflow-hidden group hover:border-slate-700 transition-all">
      <div className="text-[10px] uppercase font-bold tracking-wider text-slate-400 text-center mb-1">
        {label}
      </div>

      <div className="relative flex items-center justify-center" style={{ width: size, height: size }}>
        <svg width={size} height={size} className="transform rotate-[160deg]">
          {/* Background Track Arc */}
          <circle
            cx={center}
            cy={center}
            r={radius}
            fill="none"
            stroke="#1e293b"
            strokeWidth={strokeWidth}
            strokeDasharray={`${arcLength} ${circumference}`}
            strokeLinecap="round"
          />
          {/* Active Glowing Value Arc */}
          <circle
            cx={center}
            cy={center}
            r={radius}
            fill="none"
            stroke={scheme.stroke}
            strokeWidth={strokeWidth}
            strokeDasharray={`${arcLength} ${circumference}`}
            strokeDashoffset={strokeDashoffset}
            strokeLinecap="round"
            style={{
              transition: 'stroke-dashoffset 0.8s cubic-bezier(0.4, 0, 0.2, 1)',
              filter: `drop-shadow(0 0 6px ${scheme.glow})`,
            }}
          />
        </svg>

        {/* Center Readout Value */}
        <div className="absolute inset-0 flex flex-col items-center justify-center text-center pt-2">
          <span className={`text-xl font-mono font-bold ${scheme.text} tracking-tight`}>
            {typeof value === 'number' && !Number.isInteger(value) ? value.toFixed(2) : value}
            {unit && <span className="text-[11px] text-slate-300 ml-0.5 font-medium">{unit}</span>}
          </span>
          {subtext && (
            <span className="text-[10px] text-slate-300 font-mono font-medium mt-0.5 max-w-[90px] truncate">
              {subtext}
            </span>
          )}
        </div>
      </div>

      <div className="flex justify-between w-full px-2 text-[10px] text-slate-300 font-mono font-medium -mt-1">
        <span>{min}</span>
        <span>{max}</span>
      </div>
    </div>
  );
}
