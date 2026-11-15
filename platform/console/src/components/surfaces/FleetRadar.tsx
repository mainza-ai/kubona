'use client';

import React, { useState } from 'react';
import {
  Radio,
  ShieldCheck,
  Cpu,
  Lock,
  Globe,
  Star,
  ChevronDown,
  ChevronUp,
  MapPin,
} from 'lucide-react';
import type { CampaignState } from '@/types';

import { Chart } from '@/components/data/LazyChart';
import type { ChartSpec } from '@/components/data/chart';
import { useFleetProjection } from '@/domain/useFleetProjection';
import { refreshCalibration } from '@/lib/api';
import { useCostProjection } from '@/domain/useCostProjection';


interface FleetRadarProps {
  campaign?: CampaignState | null;
}

export function FleetRadar({ campaign }: FleetRadarProps) {
  const [sovereignOnly, setSovereignOnly] = useState(false);
  const [expandedBackend, setExpandedBackend] = useState<string | null>(null);
  const { fleet, status, error, intervalMs, refresh } = useFleetProjection();
  const { data: spend } = useCostProjection();
  const [calibrationBackend, setCalibrationBackend] = useState('');
  const [calibrationNote, setCalibrationNote] = useState<string | null>(null);
  const [calibrationBusy, setCalibrationBusy] = useState(false);
  const backends = fleet.backends;
  const radarEndpoints = fleet.radarEndpoints;
  const refused = fleet.refused ?? (status === 'refused' ? error : null);
  const loaded = status === 'ready' || status === 'stale';
  const pollIntervalMs = intervalMs;

  const filteredBackends = sovereignOnly
    ? backends.filter((b) => b.jurisdiction === 'LOCAL_SOVEREIGN' || (b.name || '').toLowerCase().includes('local'))
    : backends;
  // EAR99 banner only when a backend flag actually carries it — never static.
  const showEar99 = backends.some((b) => ((b.itar_ear || '') as string).toUpperCase().includes('EAR99'));
  const canRefreshCalibration = backends.some((b) => (b.provider || '').toLowerCase() === 'ibm-qiskit');
  async function onRefreshCalibration() {
    const backend = calibrationBackend.trim();
    if (!backend) {
      setCalibrationNote('Enter the IBM backend name (for example ibm_fez).');
      return;
    }
    setCalibrationBusy(true);
    setCalibrationNote(null);
    const result = await refreshCalibration(backend);
    setCalibrationBusy(false);
    if (!result.ok) {
      setCalibrationNote(result.error || 'Calibration refresh refused.');
      return;
    }
    const pending = result.queue?.pending_jobs;
    setCalibrationNote(`Calibration installed${typeof pending === 'number' ? `; vendor pending ${pending}` : ''}.`);
    refresh();
  }
  const queued = backends.filter((b) => typeof b.queue_depth === 'number' && Number.isFinite(b.queue_depth));
  const queueSpec: ChartSpec | undefined =
    queued.length > 0
      ? {
          kind: 'bars',
          title: 'Backend queue depth',
          series: [
            {
              name: 'Queued jobs',
              points: queued.map((backend) => ({
                x: backend.name,
                y: backend.queue_depth as number,
                display: `${backend.queue_depth} jobs`,
                evidence: [{ key: 'provenance', label: backend.telemetry || 'provenance not reported' }],
              })),
            },
          ],
          x: { label: 'Backend', kind: 'category' },
          y: { label: 'Queue', unit: 'jobs', format: 'number' },
          basis:
            'Vendor/daemon queue depth as served. Declared catalog values are not live canary measurements; provenance is shown per bar.',
          detail: 'full',
        }
      : undefined;

  const spendEntries = Object.entries(spend?.by_provider ?? {}).filter(([, value]) => Number.isFinite(value) && value > 0);
  const spendSpec: ChartSpec | undefined =
    spendEntries.length > 0
      ? {
          kind: 'bars',
          title: 'LLM spend by provider',
          series: [{ name: 'Spent', points: spendEntries.map(([provider, value]) => ({ x: provider, y: value, display: `$${value.toFixed(4)}` })) }],
          x: { label: 'Provider', kind: 'category' },
          y: { label: 'Spend', unit: 'USD', format: 'money' },
          basis: 'Server cost summary by provider. Unmetered calls are counted, not priced; see the status strip for blind-call count.',
          headline: { label: 'Total', value: `$${spend?.spent_usd.toFixed(2) ?? '0.00'}` },
          detail: 'full',
        }
      : undefined;

  const tiers = [
    { tier: 0, title: 'Tier 0 · Classical Physics Ground Truth', desc: 'PySCF FCI / RHF ab-initio exact anchor' },
    { tier: 1, title: 'Tier 1 · Sovereign Local Emulation', desc: 'Qiskit Aer GPU/MPS, ZNE, Wire-Cutting' },
    { tier: 2, title: 'Tier 2 · Managed Cloud Simulators', desc: 'AWS Braket SV1/TN1, CUDA-Q accelerated' },
    { tier: 3, title: 'Tier 3 · Physical QPUs', desc: 'IBM Heron (156q), Quantinuum H1 (20q), Rigetti' },
  ];

  return (
    <div className="relative rounded-sheet overflow-hidden bg-page border border-border shadow-[var(--shadow-float)] p-7 space-y-6 select-none">
      {/* Serene Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-border pb-5">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-card bg-info-wash border border-info-edge flex items-center justify-center shadow-inner">
            <Radio className="w-5 h-5 text-info-ink" />
          </div>
          <div>
            <h2 className="text-body-large font-semibold text-text tracking-tight flex items-center space-x-2">
              <span>Ambient Hardware Radar</span>
              <span className="w-2 h-2 rounded-capsule bg-success" />
            </h2>
            <p className="text-label text-muted mt-0.5">
              4-Tier classical-quantum continuum with real-time coherence calibration and sovereign routing
            </p>
          </div>
        </div>

        {/* Local-only list filter (display filter — enforcement lives in routing/admission) */}
        <div className="flex items-center space-x-3">
          <button
            onClick={() => {
              refresh();
            }}
            title="Refresh fleet + radar now (auto-poll also runs)"
            className="px-3.5 py-1.5 rounded-row text-label font-semibold flex items-center space-x-2 border transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] shadow-sm active:scale-95 bg-fill hover:bg-fill-hover border-border text-label-secondary"
          >
            <span>Refresh now</span>
          </button>
          <button
            onClick={() => setSovereignOnly(!sovereignOnly)}
            title="Filters this list to local backends; does not change enforcement"
            className={`px-3.5 py-1.5 rounded-row text-label font-semibold flex items-center space-x-2 border transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] shadow-sm active:scale-95 ${
              sovereignOnly
                ? 'bg-success-wash border-success-edge text-success-ink ring-1 ring-success-edge'
                : 'bg-fill hover:bg-fill-hover border-border text-label-secondary'
            }`}
          >
            {sovereignOnly ? (
              <>
                <Lock className="w-3.5 h-3.5 text-success-ink" />
                <span>Local backends only (filter)</span>
              </>
            ) : (
              <>
                <Globe className="w-3.5 h-3.5 text-muted" />
                <span>Global Fleet Available</span>
              </>
            )}
          </button>
        </div>
      </div>

      {/* Wave-3: honest first-load skeleton (gated on the fleet one-shot
          above — hidden the moment it settles, refused or not). */}
      {!loaded && !refused && (
        <div data-testid="fleet-radar-skeleton" className="space-y-2.5" aria-hidden="true">
          <div className="h-4 w-2/3 rounded-chip bg-fill animate-pulse" />
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3.5">
            <div className="h-20 rounded-card bg-fill border border-border animate-pulse" />
            <div className="h-20 rounded-card bg-fill border border-border animate-pulse" />
          </div>
          <div className="h-24 rounded-card bg-fill border border-border animate-pulse" />
        </div>
      )}

      {/* Continuum Radar Strip */}
      <div className="p-4 rounded-card bg-surface border border-border space-y-3">
        <div className="flex items-center justify-between text-label">
          <span className="font-semibold text-label-secondary uppercase tracking-wider font-mono text-[12px]" title="Vendor-declared continuum — not live canary data">
            Quantum Execution Continuum (declared catalog)
          </span>
          {showEar99 && (
            <span className="text-muted font-mono text-[12px] flex items-center space-x-1.5">
              <ShieldCheck className="w-3.5 h-3.5 text-success-ink" />
              <span>EAR99 Export-Controlled Cloud Filtering Active</span>
            </span>
          )}
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-2.5">
          {tiers.map((t) => (
            <div
              key={t.tier}
              className="p-3 rounded-row bg-raised border border-border space-y-1 hover:border-info-edge transition-[color,background-color,border-color,box-shadow,opacity,transform,filter]"
            >
              <div className="text-label font-bold text-text flex items-center space-x-1.5">
                <span className="w-1.5 h-1.5 rounded-capsule bg-info" />
                <span>Tier {t.tier}</span>
              </div>
              <p className="text-[12px] text-muted leading-tight font-sans">
                {t.desc}
              </p>
              <p className="text-[12px] font-mono text-faint" title="Tier list is vendor-declared until live canary data backs it">
                declared — not live canary data
              </p>
            </div>
          ))}
        </div>
      </div>

      {queueSpec && (
        <div className="rounded-card border border-border bg-surface p-4">
          <Chart spec={queueSpec} />
        </div>
      )}
      {spendSpec && (
        <div className="rounded-card border border-border bg-surface p-4">
          <Chart spec={spendSpec} />
        </div>
      )}

      {canRefreshCalibration && (
        <div className="flex flex-wrap items-center gap-2 rounded-card border border-border bg-surface p-3 text-label">
          <span className="text-muted">IBM calibration refresh</span>
          <input
            aria-label="IBM backend for calibration refresh"
            value={calibrationBackend}
            onChange={(event) => setCalibrationBackend(event.target.value)}
            placeholder="ibm_fez"
            className="w-36 rounded-chip border border-border bg-fill px-2 py-1 font-mono text-text"
          />
          <button
            type="button"
            onClick={onRefreshCalibration}
            disabled={calibrationBusy}
            className="rounded-chip border border-info-edge bg-info-wash px-2 py-1 font-semibold text-info-ink disabled:opacity-50"
          >
            {calibrationBusy ? 'Refreshing…' : 'Refresh calibration'}
          </button>
          {calibrationNote && <span role="status" className="text-muted">{calibrationNote}</span>}
        </div>
      )}

      {backends.length > 0 && (
        <div className="overflow-x-auto rounded-card border border-border bg-surface p-4">
          <table className="w-full text-left text-[12px]">
            <caption className="mb-2 text-left text-label font-semibold text-label-secondary">Fleet capability matrix — exact served values, provenance per row</caption>
            <thead className="font-mono uppercase tracking-wider text-faint">
              <tr>
                <th scope="col" className="py-1 pr-3">Backend</th>
                <th scope="col" className="py-1 pr-3">Qubits</th>
                <th scope="col" className="py-1 pr-3">Queue</th>
                <th scope="col" className="py-1 pr-3">Calibration</th>
                <th scope="col" className="py-1 pr-3">Cost/shot</th>
                <th scope="col" className="py-1 pr-3">Jurisdiction</th>
                <th scope="col" className="py-1 pr-3">Credential</th>
                <th scope="col" className="py-1">Telemetry</th>
              </tr>
            </thead>
            <tbody className="font-mono text-label-secondary">
              {filteredBackends.map((backend) => (
                <tr key={backend.name} className="border-t border-border">
                  <th scope="row" className="py-1.5 pr-3 font-sans font-medium text-text">{backend.name}</th>
                  <td className="py-1.5 pr-3">{typeof backend.qubits === 'number' ? backend.qubits : '—'}</td>
                  <td className="py-1.5 pr-3">
                    {typeof backend.queue_depth === 'number'
                      ? `${backend.queue_depth}${typeof backend.queue_eta_s === 'number' ? ` · ~${Math.round(backend.queue_eta_s)}s ETA` : backend.queue_eta_reason ? ' · ETA unknown' : ''}`
                      : '—'}
                  </td>
                  <td className="py-1.5 pr-3" title={backend.fidelity_source || undefined}>
                    {typeof backend.calibration_age_hours === 'number'
                      ? `${backend.calibration_age_hours.toFixed(1)}h${backend.calibration_fresh === false ? ' · STALE' : ''}`
                      : '—'}
                  </td>
                  <td className="py-1.5 pr-3">{typeof backend.cost_per_shot === 'number' ? `$${backend.cost_per_shot.toFixed(6)}` : '—'}</td>
                  <td className="py-1.5 pr-3">{backend.jurisdiction || '—'}</td>
                  <td className="py-1.5 pr-3">{backend.credential_state || '—'}</td>
                  <td className="py-1.5">{backend.telemetry || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Real-Time Host Daemon Telemetry (Ports 8080 & 50051) */}
      <div className="space-y-3">
        <div className="flex items-center justify-between text-label font-semibold text-label-secondary font-mono uppercase tracking-wider">
          <span>Host Daemon Telemetry &amp; Gateway Links</span>
          <span className="text-[12px] text-info-ink font-mono flex items-center space-x-1.5" title={`Current poll interval ${(pollIntervalMs / 1000).toFixed(1)}s — backs off to 15s on refusal/error`}>
            <span className="w-1.5 h-1.5 rounded-capsule bg-info" />
            <span>Adaptive radar · {(pollIntervalMs / 1000).toFixed(1)}s interval (1s nominal, backs off)</span>
          </span>
        </div>
        {/* Wave-3: stale-data badge when refresh lags (backoff grew past
            nominal). Gated on the existing pollIntervalMs mirror — no new
            timing logic. */}
        {loaded && pollIntervalMs > 1000 && (
          <div data-testid="fleet-radar-stale" className="px-2.5 py-1.5 rounded-row bg-warning-wash border border-warning-edge text-warning-ink text-[12px] font-mono">
            Stale — refresh delayed ({(pollIntervalMs / 1000).toFixed(1)}s interval). Data may be out of date.
          </div>
        )}

        {refused && (
          <div className="p-2.5 rounded-row bg-danger-wash border border-danger-edge text-danger-ink text-[12px]">
            Radar refused — {refused.slice(0, 200)}. Nothing discovered, nothing assumed.
          </div>
        )}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3.5">
          {/* Port 8080 Go Core Orchestrator Gateway */}
          {(() => {
            // Outage defaults unknown (not LISTENING): absent endpoint renders
            // UNKNOWN, never a confident LISTENING.
            const ep8080 = radarEndpoints.find((e) => e.port === 8080);
            const state: 'on' | 'off' | 'unknown' = ep8080 == null ? 'unknown' : ep8080.listening ? 'on' : 'off';
            return (
              <div
                data-testid="radar-endpoint-8080"
                className="p-4 rounded-card bg-surface border border-border flex items-center justify-between transition-[color,background-color,border-color,box-shadow,opacity,transform,filter]"
              >
                <div className="space-y-1">
                  <div className="flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-capsule bg-info" />
                    <span className="text-label font-mono font-bold text-text">Port 8080</span>
                    <span className="text-[12px] font-mono text-info-ink bg-info-wash px-2 py-0.5 rounded-chip border border-info-edge">
                      HTTP / REST
                    </span>
                  </div>
                  <p className="text-label text-label-secondary font-sans">
                    Kubona Go Core Gateway (Daemon: kubonad)
                  </p>
                </div>
                <div
                  data-testid="status-indicator-8080"
                  className={`px-3 py-1 rounded-capsule text-[12px] font-mono font-semibold flex items-center space-x-1.5 border transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] ${
                    state === 'on'
                      ? 'bg-success-wash text-success-ink border-success-edge'
                      : state === 'unknown'
                        ? 'bg-fill text-label-secondary border-border-strong'
                        : 'bg-danger-wash text-danger-ink border-danger-edge'
                  }`}
                >
                  <span
                    className={`w-2 h-2 rounded-capsule ${
                      state === 'on' ? 'bg-success' : state === 'unknown' ? 'bg-muted' : 'bg-danger'
                    }`}
                  />
                  <span>{state === 'on' ? 'LISTENING / SOVEREIGN' : state === 'unknown' ? 'UNKNOWN — no endpoint' : 'OFFLINE / DISCONNECTED'}</span>
                </div>
              </div>
            );
          })()}

          {/* Port 50051 Python Quantum Bridge */}
          {(() => {
            const ep50051 = radarEndpoints.find((e) => e.port === 50051);
            const state: 'on' | 'off' | 'unknown' = ep50051 == null ? 'unknown' : ep50051.listening ? 'on' : 'off';
            return (
              <div
                data-testid="radar-endpoint-50051"
                className="p-4 rounded-card bg-surface border border-border flex items-center justify-between transition-[color,background-color,border-color,box-shadow,opacity,transform,filter]"
              >
                <div className="space-y-1">
                  <div className="flex items-center space-x-2">
                    <span className="w-2 h-2 rounded-capsule bg-soft-lilac" />
                    <span className="text-label font-mono font-bold text-text">Port 50051</span>
                    <span className="text-[12px] font-mono text-soft-lilac-ink bg-soft-indigo-wash px-2 py-0.5 rounded-chip border border-soft-indigo-edge">
                      gRPC
                    </span>
                  </div>
                  <p className="text-label text-label-secondary font-sans">
                    Kubona Python Quantum Bridge (Daemon: server.py)
                  </p>
                </div>
                <div
                  data-testid="status-indicator-50051"
                  className={`px-3 py-1 rounded-capsule text-[12px] font-mono font-semibold flex items-center space-x-1.5 border transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] ${
                    state === 'on'
                      ? 'bg-success-wash text-success-ink border-success-edge'
                      : state === 'unknown'
                        ? 'bg-fill text-label-secondary border-border-strong'
                        : 'bg-danger-wash text-danger-ink border-danger-edge'
                  }`}
                >
                  <span
                    className={`w-2 h-2 rounded-capsule ${
                      state === 'on' ? 'bg-success' : state === 'unknown' ? 'bg-muted' : 'bg-danger'
                    }`}
                  />
                  <span>{state === 'on' ? 'LISTENING / SOVEREIGN' : state === 'unknown' ? 'UNKNOWN — no endpoint' : 'OFFLINE / DISCONNECTED'}</span>
                </div>
              </div>
            );
          })()}
        </div>
      </div>

      {/* Backend Devices: Clean Minimal Cards with Progressive Disclosure */}
      <div className="space-y-3">
        <div className="text-label font-semibold text-label-secondary font-mono uppercase tracking-wider">
          Available Devices &amp; Simulators ({filteredBackends.length})
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {filteredBackends.map((b) => {
            const isRecommended =
              campaign?.quantum_factors?.recommended_backend &&
              ((b.name.toLowerCase().includes('aer') && campaign.quantum_factors.recommended_backend.toLowerCase().includes('aer')) ||
                (b.name.toLowerCase().includes('heron') && campaign.quantum_factors.recommended_backend.toLowerCase().includes('heron')));

            const isExpanded = expandedBackend === b.name;
            const statusUp = (b.status || '').toUpperCase();
            const isOnline = statusUp === 'ONLINE' || statusUp === 'READY';
            // Credential dimension (fleet): credentialless is not down —
            // the device may be fine, this daemon lacks credentials.
            const credState = (b.credential_state || '').toLowerCase();
            const noCreds = credState === 'credentialless';
            // H6: queue/coherence/cost are only trustworthy when the daemon
            // says how it got them — otherwise the card says UNPROVENANCED.
            const unprovenanced = b.telemetry !== 'measured' && b.telemetry !== 'declared';

            return (
              <div
                key={b.name}
                className={`p-5 rounded-card bg-surface border transition-[color,background-color,border-color,box-shadow,opacity,transform,filter] space-y-3.5 ${
                  isRecommended
                    ? 'border-info-edge ring-1 ring-info-edge shadow-[var(--shadow-card)]'
                    : 'border-border hover:border-border-strong'
                }`}
              >
                {/* Device Title & Status Pill */}
                <div className="flex items-center justify-between">
                  <div className="flex items-center space-x-2.5">
                    <Cpu className={`w-4 h-4 ${isRecommended ? 'text-info-ink' : 'text-muted'}`} />
                    <span className="font-bold text-body-strong text-text font-mono tracking-tight">{b.name}</span>
                    {b.telemetry === 'declared' && (
                      <span
                        className="px-2 py-0.5 rounded-inset bg-fill text-label-secondary border border-border text-[12px] font-mono font-bold"
                        title="Vendor catalog, not a live measurement: queue depth and coherence figures are declared values until the live canary proxy lands. Credential state is measured."
                      >
                        CATALOG
                      </span>
                    )}
                    {isRecommended && (
                      <span className="px-2 py-0.5 rounded-inset bg-info-wash text-info-ink border border-info-edge text-[12px] font-mono font-bold flex items-center space-x-1">
                        <Star className="w-2.5 h-2.5 fill-info text-info-ink" />
                        <span>RECOMMENDED</span>
                      </span>
                    )}
                  </div>

                  <span
                    className={`px-2.5 py-0.5 rounded-capsule text-[12px] font-mono font-semibold flex items-center space-x-1.5 border ${
                      isOnline
                        ? 'bg-success-wash text-success-ink border-success-edge'
                        : 'bg-warning-wash text-warning-ink border-warning-edge'
                    }`}
                  >
                    <span className={`w-1.5 h-1.5 rounded-capsule ${isOnline ? 'bg-success' : 'bg-warning'}`} />
                    <span>{b.status}</span>
                  </span>
                  {noCreds && (
                    <span
                      className="px-2 py-0.5 rounded-capsule text-[12px] font-mono font-semibold bg-soft-sky-wash text-soft-sky-ink border border-soft-sky-edge"
                      title="This daemon holds no credentials for this backend — the device may be fine. Set the provider token to enable it."
                    >
                      NO CREDENTIALS
                    </span>
                  )}
                </div>

                <div className="text-label text-label-secondary font-sans">{b.type}</div>

                {/* Depth 1: Essential Metrics */}
                <div className="grid grid-cols-3 gap-2 text-label font-mono bg-raised p-3 rounded-row border border-border">
                  <div>
                    <span className="text-muted block text-[12px] font-medium">Capacity</span>
                    <strong className="text-text text-label font-bold">
                      {typeof b.qubits === 'number' && Number.isFinite(b.qubits) ? `${b.qubits} Qubits` : 'Qubits UNKNOWN'}
                    </strong>
                  </div>
                  <div>
                    <span className="text-muted block text-[12px] font-medium">Coherence T₁</span>
                    <strong className="text-info-ink text-label font-bold">
                      {b.t1_us && b.t1_us !== 'unknown' ? `${b.t1_us}${/^[0-9.]+$/.test(b.t1_us) ? ' µs' : ''}` : 'T1 UNKNOWN'}
                    </strong>
                  </div>
                  <div>
                    <span className="text-muted block text-[12px] font-medium">Queue</span>
                    <strong className="text-warning-ink text-label font-bold">
                      {typeof b.queue_depth === 'number' && Number.isFinite(b.queue_depth) ? `${b.queue_depth} jobs` : 'queue UNKNOWN'}
                    </strong>
                  </div>
                  {unprovenanced && (
                    <div className="col-span-3 text-[12px] font-mono text-muted border border-border rounded-chip px-1.5 py-0.5" title="Daemon reported no telemetry provenance — queue/coherence are neither measured nor declared">
                      UNPROVENANCED — queue/coherence unprovenanced
                    </div>
                  )}
                </div>

                {/* Progressive Disclosure Toggle */}
                <button
                  onClick={() => setExpandedBackend(isExpanded ? null : b.name)}
                  className="w-full flex items-center justify-between text-label text-label-secondary hover:text-text font-medium transition-colors pt-2 border-t border-border"
                >
                  <span className="flex items-center space-x-1.5">
                    <MapPin className="w-3.5 h-3.5 text-muted" />
                    <span>{b.jurisdiction || 'jurisdiction UNKNOWN'} ({b.itar_ear || 'export class UNKNOWN'})</span>
                  </span>
                  <span className="flex items-center space-x-1 font-mono text-info-ink">
                    <span>{isExpanded ? 'Hide Specs' : 'Deep Physics & Pricing'}</span>
                    {isExpanded ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
                  </span>
                </button>

                {/* Depth 2 & 3: Physics & Cost Specs */}
                {isExpanded && (
                  <div className="p-3.5 rounded-row bg-surface-elev border border-border space-y-2 text-label font-mono animate-fadeIn">
                    <div className="flex justify-between text-label-secondary font-medium">
                      <span>Cost per Shot:{unprovenanced ? ' (UNPROVENANCED)' : ''}</span>
                      <strong className="text-success-ink">${typeof b.cost_per_shot === 'number' && Number.isFinite(b.cost_per_shot) ? b.cost_per_shot.toFixed(4) : '—'}</strong>
                    </div>
                    {typeof b.avg_t2_us === 'number' && Number.isFinite(b.avg_t2_us) && b.avg_t2_us > 0 && (
                      <div className="flex justify-between text-label-secondary font-medium">
                        <span>Coherence T₂ (Dephasing):</span>
                        <strong className="text-info-ink">{b.avg_t2_us} µs</strong>
                      </div>
                    )}
                    <div className="flex justify-between text-label-secondary font-medium">
                      <span>Jurisdiction Policy:</span>
                      <span className="text-text font-semibold">{b.jurisdiction || 'jurisdiction UNKNOWN'} · {b.itar_ear || 'export class UNKNOWN'}</span>
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
