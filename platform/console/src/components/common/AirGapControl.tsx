'use client';

import React, { useCallback, useEffect, useState } from 'react';
import { Globe, Lock } from 'lucide-react';
import ConfirmDialog from '@/components/common/ConfirmDialog';
import { fetchLLMSettings, setLLMAirGap } from '@/lib/api';
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

  useEffect(() => {
    if (typeof options.initialValue === 'boolean') {
      setAirGap(options.initialValue);
      return;
    }
    let live = true;
    void fetchLLMSettings().then((s) => {
      if (live && s && typeof s.air_gapped === 'boolean') {
        setAirGap(s.air_gapped);
      }
    });
    return () => { live = false; };
  }, [options.initialValue]);

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
      const res = await setLLMAirGap(next);
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

function AirGapRelease({
  value,
  onToggle,
  children,
}: {
  value: boolean | null;
  onToggle: () => void;
  children: (request: () => void) => React.ReactNode;
}) {
  const [confirming, setConfirming] = useState(false);
  const request = () => {
    if (value === true) setConfirming(true);
    else onToggle();
  };
  return (
    <>
      {children(request)}
      <ConfirmDialog
        open={confirming}
        title="Release sovereign air-gap"
        body="Cloud models and metered hardware become reachable. This does not release them until you confirm."
        confirmLabel="Release air-gap"
        danger
        onConfirm={() => {
          setConfirming(false);
          onToggle();
        }}
        onCancel={() => setConfirming(false)}
      />
    </>
  );
}

export function AirGapToggleButton({
  value,
  onToggle,
  offLabel = 'Air-Gap: OFF',
}: AirGapToggleButtonProps) {
  return (
    <AirGapRelease value={value} onToggle={onToggle}>
      {(request) => (
    <button
      type="button"
      onClick={request}
      disabled={value === null}
      title={value === null ? 'Air-gap state unknown — settings unreachable' : 'Toggle sovereign air-gap'}
      className={`px-3.5 py-1.5 rounded-row text-label font-semibold flex items-center space-x-2 border transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] shadow-sm active:scale-95 disabled:opacity-50 ${
        value === true
          ? 'bg-success-wash border-success-edge text-success-ink'
          : value === false
            ? 'bg-tile hover:bg-fill-hover border-border text-label-secondary'
            : 'bg-tile border-border text-muted'
      }`}
    >
      {value === true ? (
        <>
          <Lock className="w-3.5 h-3.5 text-success-ink" />
          <span>Air-Gap: ENGAGED</span>
        </>
      ) : value === false ? (
        <>
          <Globe className="w-3.5 h-3.5 text-muted" />
          <span>{offLabel}</span>
        </>
      ) : (
        <>
          <Globe className="w-3.5 h-3.5 text-faint" />
          <span>Air-Gap: UNKNOWN</span>
        </>
      )}
    </button>
      )}
    </AirGapRelease>
  );
}

export interface AirGapSwitchRowProps {
  value: boolean | null;
  onToggle: () => void;
}

export function AirGapSwitchRow({ value, onToggle }: AirGapSwitchRowProps) {
  return (
    <div className="w-full flex items-center justify-between p-4 rounded-card bg-surface border border-border">
      <div className="flex items-center space-x-3">
        <div
          className={`w-8 h-8 rounded-row flex items-center justify-center ${
            value === true
              ? 'bg-info-wash text-info-ink border border-info-edge'
              : 'bg-tile text-muted border border-border'
          }`}
        >
          <Lock className="w-4 h-4" />
        </div>
        <div>
          <div className="text-label font-semibold text-text">Sovereign Air-Gap Mode</div>
          <div className="text-[12px] text-muted">
            {value === null ? 'State unknown — settings unreachable' : 'Strictly keep computations on local Mac hardware'}
          </div>
        </div>
      </div>

      <AirGapRelease value={value} onToggle={onToggle}>
      {(request) => (
      <button
        type="button"
        onClick={request}
        disabled={value === null}
        title={value === null ? 'Air-gap state unknown — settings unreachable' : 'Persists to intelligence settings (same control as Cognitive Mesh)'}
        className={`w-11 h-6 rounded-capsule transition-colors relative p-0.5 ${
          value === true
            ? 'bg-accent'
            : 'bg-tile border border-border-strong'
        } disabled:opacity-40`}
      >
        <div
          className={`w-5 h-5 rounded-capsule bg-on-accent shadow-md transform transition-transform ${
            value === true ? 'translate-x-5' : 'translate-x-0'
          }`}
        />
      </button>
      )}
      </AirGapRelease>
    </div>
  );
}
