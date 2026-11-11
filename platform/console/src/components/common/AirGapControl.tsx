'use client';

import React, { useCallback, useState } from 'react';
import { Globe, Lock } from 'lucide-react';
import { fetchLLMSettings, saveLLMSettings } from '@/lib/api';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { writeSettlementLabel } from '@/domain/write-settlement';

export interface UseAirGapControlOptions {
  initialValue?: boolean | null;
  onStatusMessage?: (msg: string | null) => void;
  onPersisted?: (next: boolean) => void;
}

export function useAirGapControl(options: UseAirGapControlOptions = {}) {
  const [airGap, setAirGap] = useState<boolean | null>(options.initialValue ?? null);
  const [error, setError] = useState<string | null>(null);
  const write = useWriteSettlement();

  const toggle = useCallback(async () => {
    if (airGap === null) {
      setError('Air-gap state unknown — reload settings before changing it.');
      return;
    }
    const prev = airGap;
    const next = !airGap;
    const requestId = write.begin(`air-gap ${next ? 'engage' : 'release'}`);
    setAirGap(next);
    setError(null);
    try {
      const current = await fetchLLMSettings();
      if (!current) throw new Error('settings unreachable — nothing changed');
      const res = await saveLLMSettings({ ...(current as Record<string, unknown>), air_gapped: next });
      if (res.ok) {
        write.applied(requestId, `air-gap ${next ? 'engaged' : 'released'}`);
        options.onStatusMessage?.(`Sovereign Air-Gap ${next ? 'ENGAGED' : 'released'} (persisted).`);
        options.onPersisted?.(next);
      } else {
        const reason = res.error || 'save not confirmed';
        write[res.unknown ? 'unknown' : 'refused'](requestId, reason);
        setAirGap(prev);
        const msg = `Air-gap save refused — switch reverted (${reason}). Nothing changed.`;
        setError(msg);
        options.onStatusMessage?.(msg);
      }
    } catch (err) {
      const reason = err instanceof Error ? err.message : String(err);
      write.unknown(requestId, reason);
      setAirGap(prev);
      const msg = `Air-gap save failed — switch reverted, nothing changed (${reason}).`;
      setError(msg);
      options.onStatusMessage?.(msg);
    }
  }, [airGap, write, options]);

  return {
    airGap,
    setAirGap,
    error,
    setError,
    write,
    settlementLabel: writeSettlementLabel(write.write),
    toggle,
  };
}

export interface AirGapToggleButtonProps {
  value: boolean | null;
  onToggle: () => void;
  offLabel?: string;
}

export function AirGapToggleButton({
  value,
  onToggle,
  offLabel = 'Air-Gap: OFF',
}: AirGapToggleButtonProps) {
  return (
    <button
      type="button"
      onClick={onToggle}
      disabled={value === null}
      title={value === null ? 'Air-gap state unknown — settings unreachable' : 'Toggle sovereign air-gap'}
      className={`px-3.5 py-1.5 rounded-xl text-xs font-semibold flex items-center space-x-2 border transition-all shadow-sm active:scale-95 disabled:opacity-50 ${
        value === true
          ? 'bg-[var(--color-success-wash)] border-[var(--color-success-edge)] text-[var(--color-success-ink)]'
          : value === false
            ? 'bg-[var(--color-tile)] hover:bg-[var(--color-fill-hover)] border-[var(--color-border)] text-[var(--color-label-secondary)]'
            : 'bg-[var(--color-tile)] border-[var(--color-border)] text-[var(--color-muted)]'
      }`}
    >
      {value === true ? (
        <>
          <Lock className="w-3.5 h-3.5 text-[var(--color-success-ink)]" />
          <span>Air-Gap: ENGAGED</span>
        </>
      ) : value === false ? (
        <>
          <Globe className="w-3.5 h-3.5 text-[var(--color-muted)]" />
          <span>{offLabel}</span>
        </>
      ) : (
        <>
          <Globe className="w-3.5 h-3.5 text-[var(--color-faint)]" />
          <span>Air-Gap: UNKNOWN</span>
        </>
      )}
    </button>
  );
}

export interface AirGapSwitchRowProps {
  value: boolean | null;
  onToggle: () => void;
}

export function AirGapSwitchRow({ value, onToggle }: AirGapSwitchRowProps) {
  return (
    <div className="w-full flex items-center justify-between p-4 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)]">
      <div className="flex items-center space-x-3">
        <div
          className={`w-8 h-8 rounded-xl flex items-center justify-center ${
            value === true
              ? 'bg-[var(--color-info-wash)] text-[var(--color-info-ink)] border border-[var(--color-info-edge)]'
              : 'bg-[var(--color-tile)] text-[var(--color-muted)] border border-[var(--color-border)]'
          }`}
        >
          <Lock className="w-4 h-4" />
        </div>
        <div>
          <div className="text-xs font-semibold text-[var(--color-text)]">Sovereign Air-Gap Mode</div>
          <div className="text-[12px] text-[var(--color-muted)]">
            {value === null ? 'State unknown — settings unreachable' : 'Strictly keep computations on local Mac hardware'}
          </div>
        </div>
      </div>

      <button
        type="button"
        onClick={onToggle}
        disabled={value === null}
        title={value === null ? 'Air-gap state unknown — settings unreachable' : 'Persists to intelligence settings (same control as Cognitive Mesh)'}
        className={`w-11 h-6 rounded-full transition-colors relative p-0.5 ${
          value === true
            ? 'bg-[var(--color-accent)]'
            : 'bg-[var(--color-tile)] border border-[var(--color-border-strong)]'
        } disabled:opacity-40`}
      >
        <div
          className={`w-5 h-5 rounded-full bg-[var(--color-on-accent)] shadow-md transform transition-transform ${
            value === true ? 'translate-x-5' : 'translate-x-0'
          }`}
        />
      </button>
    </div>
  );
}
