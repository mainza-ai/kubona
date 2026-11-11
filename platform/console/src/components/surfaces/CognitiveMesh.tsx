'use client';

import React, { useState, useEffect, useCallback, useRef } from 'react';
import {
  Network,
  RefreshCw,
  Terminal,
  Users,
  Plus,
  Trash2,
  Zap,
  X,
} from 'lucide-react';
import { CampaignState, CustomAgentManifest, RoutingDecision, ModalityType, RadarEndpoint } from '@/types';
import {
  fetchRadar,
  fetchLLMSettings,
  fetchCostSummary,
  fetchCustomAgents,
  fetchSystemAgents,
  createCustomAgent,
  deleteCustomAgent,
  routeMultimodalPayload,
} from '@/lib/api';
import { fmtFixed } from '@/lib/guards';
import { useFocusTrap } from '@/lib/useFocusTrap';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { writeSettlementLabel } from '@/domain/write-settlement';
import { AirGapToggleButton, useAirGapControl } from '@/components/common/AirGapControl';
import { BudgetSummaryStrip } from '@/components/common/BudgetSummaryStrip';
import { subscribeInvalidation } from '@/domain/invalidation';
import type { CostSummary } from '@/lib/api';

interface CognitiveMeshProps {
  campaign?: CampaignState | null;
}

export function CognitiveMesh({ campaign }: CognitiveMeshProps) {
  const [endpoints, setEndpoints] = useState<RadarEndpoint[]>([]);
  const [radarRefused, setRadarRefused] = useState<string | null>(null);
  // Unknown until the daemon answers; never assume ENGAGED from a default.
  const {
    airGap: airGapped,
    setAirGap: setAirGapped,
    error: airGapError,
    settlementLabel: airGapSettlementLabel,
    toggle: toggleAirGap,
  } = useAirGapControl();
  const [loading, setLoading] = useState(false);
  const [agents, setAgents] = useState<CustomAgentManifest[]>([]);
  const [systemAgents, setSystemAgents] = useState<Array<{ role: string; description: string }>>([]);
  const [isCreatingAgent, setIsCreatingAgent] = useState(false);
  const createAgentPanelRef = useRef<HTMLDivElement | null>(null);
  useFocusTrap(isCreatingAgent, createAgentPanelRef);
  const [meshError, setMeshError] = useState<string | null>(null);
  const agentWrite = useWriteSettlement();
  const [costSummary, setCostSummary] = useState<CostSummary | null>(null);
  const [costRefused, setCostRefused] = useState<string | null>(null);
  const [routeError, setRouteError] = useState<string | null>(null);

  // New Agent Form
  const [newAgentName, setNewAgentName] = useState('');
  const [newAgentRole, setNewAgentRole] = useState('custom_theorist');
  const [newAgentDesc, setNewAgentDesc] = useState('');
  const [newAgentPrompt, setNewAgentPrompt] = useState('');
  const [newAgentModel, setNewAgentModel] = useState('');
  const [newAgentBudget, setNewAgentBudget] = useState('5.0');
  const [newAgentEmoji, setNewAgentEmoji] = useState('🧪');

  // Multimodal Router Test
  const [testPrompt, setTestPrompt] = useState('Evaluate Kohn-Sham failure for FeMo-cofactor');
  const [testModality, setTestModality] = useState<ModalityType>('molecule_3d');
  const [testModel, setTestModel] = useState('');
  const [routingDecision, setRoutingDecision] = useState<RoutingDecision | null>(null);
  const [routingLoading, setRoutingLoading] = useState(false);

  const scanRadar = async () => {
    setLoading(true);
    setRadarRefused(null);
    try {
      const data = await fetchRadar();
      if ((data as { refused?: boolean }).refused) {
        setRadarRefused((data as { refuse_reason?: string }).refuse_reason || 'radar refused');
        setEndpoints([]);
      } else {
        setEndpoints((data.radar_endpoints as RadarEndpoint[]) || []);
      }
    } catch (err) {
      setRadarRefused(err instanceof Error ? err.message : String(err));
      setEndpoints([]);
    } finally {
      setLoading(false);
    }
  };

  const loadAgents = useCallback(async () => {
    try {
      const list = await fetchCustomAgents();
      setAgents(list);
      setMeshError(null);
    } catch (err) {
      setMeshError(err instanceof Error ? err.message : String(err));
    }
  }, []);

  useEffect(() => {
    let mounted = true;
    fetchRadar()
      .then((data) => {
        if (!mounted) return;
        if ((data as { refused?: boolean })?.refused) {
          setRadarRefused((data as { refuse_reason?: string })?.refuse_reason || 'radar refused');
          setEndpoints([]);
        } else if (data?.radar_endpoints) {
          setEndpoints(data.radar_endpoints as RadarEndpoint[]);
        }
      })
      .catch((err) => {
        if (mounted) setRadarRefused(err instanceof Error ? err.message : String(err));
      });

    fetchLLMSettings()
      .then((data) => {
        if (mounted && data?.air_gapped !== undefined) {
          setAirGapped(Boolean(data.air_gapped));
        }
      })
      .catch((err) => {
        if (mounted) setMeshError(`LLM settings unreachable: ${err instanceof Error ? err.message : String(err)}`);
      });

    fetchCustomAgents()
      .then((list) => {
        if (mounted) {
          setAgents(list);
        }
      })
      .catch((err) => {
        if (mounted) setMeshError(`Agents unreachable: ${err instanceof Error ? err.message : String(err)}`);
      });

    fetchSystemAgents().then((roles) => {
      if (mounted) setSystemAgents(roles);
    });

    fetchCostSummary()
      .then((out) => {
        if (!mounted) return;
        if (out.ok) {
          setCostSummary(out.result);
          setCostRefused(null);
        } else {
          setCostSummary(null);
          setCostRefused(out.error);
        }
      })
      .catch((err) => {
        if (mounted) {
          setCostSummary(null);
          setCostRefused(err instanceof Error ? err.message : String(err));
        }
      });

    return () => {
      mounted = false;
    };
  }, [setAirGapped]);

  useEffect(() => {
    return subscribeInvalidation(['llm'], () => {
      void fetchLLMSettings()
        .then((data) => {
          if (data?.air_gapped !== undefined) {
            setAirGapped(Boolean(data.air_gapped));
          }
        })
        .catch(() => {});
    });
  }, [setAirGapped]);

  const handleSaveCustomAgent = async () => {
    if (!newAgentName.trim() || !newAgentPrompt.trim()) return;
    if (!newAgentModel.trim()) {
      setMeshError('Refused: preferred model is required — pick an explicit model, never a baked-in default. Nothing registered.');
      return;
    }
    const parsedBudget = parseFloat(newAgentBudget);
    if (!Number.isFinite(parsedBudget) || parsedBudget < 0) {
      setMeshError(`Refused: spend budget must be a finite number ≥ 0 (got "${newAgentBudget}"). Nothing registered.`);
      return;
    }
    if (parsedBudget > 100) {
      setMeshError(`Warning: $${parsedBudget.toFixed(2)} exceeds the $100 planning ceiling — confirm this is intentional.`);
    }
    const id = `agent_${newAgentName.toLowerCase().replace(/[^a-z0-9]/g, '_')}_${Date.now().toString(36)}`;
    const manifest: CustomAgentManifest = {
      id,
      name: newAgentName,
      role: newAgentRole,
      description: newAgentDesc,
      system_prompt: newAgentPrompt,
      avatar_emoji: newAgentEmoji,
      badge_color: 'cyan',
      capabilities: ['custom_reasoning'],
      modalities: ['text', 'molecule_3d'],
      preferred_model: newAgentModel,
      mcp_tools: ['pyscf', 'lean4'],
      spend_budget_usd: parsedBudget,
      memory_scopes: ['default'],
    };
    const requestId = agentWrite.begin('register custom agent');
    try {
      await createCustomAgent(manifest);
      setIsCreatingAgent(false);
      setNewAgentName('');
      setNewAgentDesc('');
      setNewAgentPrompt('');
      agentWrite.applied(requestId, 'custom agent registered');
      setMeshError(null);
      loadAgents();
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      agentWrite[message.includes('refused (HTTP') ? 'refused' : 'unknown'](requestId, message);
      setMeshError(`Failed to create agent — nothing registered (${message}).`);
    }
  };

  const handleDeleteAgent = async (agentId: string) => {
    const requestId = agentWrite.begin('delete custom agent');
    try {
      // Audit 2026-10-21: the boolean was ignored, so a refused delete
      // reloaded the roster in false confirmation. Refusals surface.
      const removed = await deleteCustomAgent(agentId);
      if (!removed.ok) {
        agentWrite[removed.unknown ? 'unknown' : 'refused'](requestId, removed.error);
        setMeshError(`Failed to delete agent — nothing removed (${removed.error}).`);
        return;
      }
      agentWrite.applied(requestId, 'custom agent deleted');
      loadAgents();
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      agentWrite.unknown(requestId, message);
      setMeshError(`Failed to delete agent — nothing removed (${message}).`);
    }
  };

  const handleTestRoute = async () => {
    setRoutingLoading(true);
    setRouteError(null);
    try {
      const decision = await routeMultimodalPayload({
        prompt: testPrompt,
        role_hint: 'lead_theorist',
        preferred_model: testModel.trim() || undefined,
        items: [
          {
            modality: testModality,
            data: testModality === 'molecule_3d' ? 'Fe 0.0 0.0 0.0\nMo 1.2 0.0 0.0' : 'H 0 0 0',
            mime_type: 'chemical/x-xyz',
          },
        ],
        spend_budget_usd: 1.0,
      });
      if (!decision) {
        setRouteError('Router refused — no decision returned. Nothing routed.');
        setRoutingDecision(null);
      } else if (decision.ok === false) {
        setRouteError(`Router refused: ${decision.explanation || 'no reason given'}. Nothing routed.`);
        setRoutingDecision(null);
      } else {
        setRoutingDecision(decision);
      }
    } catch (err) {
      setRouteError(`Router failed — nothing routed (${err instanceof Error ? err.message : String(err)}).`);
      setRoutingDecision(null);
    } finally {
      setRoutingLoading(false);
    }
  };

  return (
    <div className="relative rounded-3xl overflow-hidden bg-[var(--color-page)] border border-[var(--color-border)] shadow-xl p-7 space-y-6">
      {meshError && (
        <div className="p-3 rounded-xl bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-xs">
          Mesh error — {meshError} Nothing was changed.
        </div>
      )}
      {writeSettlementLabel(agentWrite.write) && (
        <p role="status" className="text-[12px] font-mono text-[var(--color-muted)]">{writeSettlementLabel(agentWrite.write)}</p>
      )}
      {airGapError && (
        <div className="p-3 rounded-xl bg-[var(--color-warning-wash)] border border-[var(--color-warning-edge)] text-[var(--color-warning-ink)] text-xs">
          {airGapError}
        </div>
      )}
      {airGapSettlementLabel && (
        <p role="status" className="text-[12px] font-mono text-[var(--color-muted)]">
          {airGapSettlementLabel}
        </p>
      )}
      {/* Serene Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-[var(--color-border)] pb-5">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-2xl bg-[var(--color-soft-indigo-wash)] border border-[var(--color-soft-indigo-edge)] flex items-center justify-center">
            <Network className="w-5 h-5 text-[var(--color-accent-ink)]" />
          </div>
          <div>
            <h2 className="text-base font-semibold text-[var(--color-text)] tracking-tight flex items-center space-x-2">
              <span>Sovereign Cognitive Mesh &amp; Mission Control</span>
              <span className="w-2 h-2 rounded-full bg-[var(--color-success)]" />
            </h2>
            <p className="text-xs text-[var(--color-muted)] mt-0.5">
              Multimodal model routing, local zero-config discovery, and declarative agent orchestration
              {campaign?.target_system ? ` — active campaign: ${campaign.target_system}` : ''}
            </p>
          </div>
        </div>

        <div className="flex items-center space-x-3">
          <button
            onClick={scanRadar}
            disabled={loading}
            className="px-3 py-1.5 bg-[var(--color-tile)] hover:bg-[var(--color-fill-hover)] text-[var(--color-text)] text-xs font-semibold rounded-xl flex items-center space-x-1.5 border border-[var(--color-border)] transition-colors shadow-sm"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            <span>Scan Ports</span>
          </button>

          {/* Sovereign Air-Gap Toggle */}
          <AirGapToggleButton value={airGapped} onToggle={() => void toggleAirGap()} />
        </div>
      </div>

      {/* LLM spend (server-sealed aggregates, heuristic estimates) */}
      <BudgetSummaryStrip summary={costSummary} refused={costRefused} />

      {/* Host Port Radar Discovery */}
      <div className="p-5 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] space-y-3">
        <div className="flex items-center justify-between text-xs">
          <span className="font-semibold text-[var(--color-label-secondary)] uppercase tracking-wider font-mono text-[12px] flex items-center space-x-2">
            <Terminal className="w-4 h-4 text-[var(--color-info-ink)]" />
            <span>Local Host Port Radar (Zero-Config Discovery)</span>
          </span>
          <span className="text-[var(--color-muted)] font-mono text-[12px]">
            Target: 127.0.0.1 (Loopback Only)
          </span>
        </div>
        {radarRefused && (
          <div className="p-2.5 rounded-xl bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-[12px]">
            Radar refused — {radarRefused.slice(0, 200)}. Nothing discovered, nothing assumed.
          </div>
        )}
        {!radarRefused && endpoints.length === 0 && (
          <div className="p-2.5 rounded-xl bg-[var(--color-tile)] border border-[var(--color-border)] text-[var(--color-muted)] text-[12px] font-code">
            No endpoints discovered — radar reachable but empty (not a refusal).
          </div>
        )}

        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {endpoints.map((ep, idx) => (
            <div
              key={ep.port ?? idx}
              className="p-3.5 rounded-xl bg-[var(--color-tile)] border border-[var(--color-border)] flex items-center justify-between text-xs font-mono hover:border-[var(--color-info-edge)] transition-all"
            >
              <div>
                <div className="font-semibold text-[var(--color-text)]">{ep.service}</div>
                <div className="text-[var(--color-label-secondary)] text-xs font-mono">127.0.0.1:{ep.port}</div>
              </div>
              <span
                className={`px-2.5 py-0.5 rounded-full text-[12px] font-semibold flex items-center space-x-1.5 border ${
                  ep.listening
                    ? 'bg-[var(--color-success-wash)] text-[var(--color-success-ink)] border-[var(--color-success-edge)]'
                    : 'bg-[var(--color-surface)] text-[var(--color-label-secondary)] border-[var(--color-border)]'
                }`}
              >
                <span className={`w-1.5 h-1.5 rounded-full ${ep.listening ? 'bg-[var(--color-success)]' : 'bg-[var(--color-faint)]'}`} />
                <span>{ep.listening ? 'ONLINE' : 'IDLE'}</span>
              </span>
            </div>
          ))}
        </div>
      </div>

      {/* Multimodal Mission Control Route Simulator */}
      <div className="p-5 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] space-y-3">
        <div className="flex items-center justify-between text-xs">
          <span className="font-semibold text-[var(--color-label-secondary)] uppercase tracking-wider font-mono text-[12px] flex items-center space-x-2">
            <Zap className="w-4 h-4 text-[var(--color-warning-ink)]" />
            <span>Mission Control Multimodal Router Evaluator</span>
          </span>
          <span className="text-[var(--color-muted)] font-mono text-[12px]">
            Local-First &amp; Air-Gap Policy Enforcement
          </span>
        </div>

        <div className="flex flex-col md:flex-row items-stretch gap-3">
          <input
            type="text"
            value={testPrompt}
            onChange={(e) => setTestPrompt(e.target.value)}
            placeholder="Scientific task prompt..."
            className="flex-1 px-4 py-2 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] text-[var(--color-text)] text-xs placeholder:text-[var(--color-faint)] focus:outline-none focus:border-[var(--color-info-edge)]"
          />
          <select
            value={testModality}
            onChange={(e) => setTestModality(e.target.value as ModalityType)}
            className="px-3 py-2 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] text-[var(--color-text)] text-xs font-mono focus:outline-none"
          >
            <option value="text">Modality: Text / Paper</option>
            <option value="molecule_3d">Modality: 3D Molecule (CIF/PDB)</option>
            <option value="circuit_qir">Modality: Quantum Circuit (QASM/QIR)</option>
            <option value="spectroscopy">Modality: Spectroscopy / Vision</option>
            <option value="lean4_ast">Modality: Lean 4 AST</option>
          </select>
          <input
            type="text"
            value={testModel}
            onChange={(e) => setTestModel(e.target.value)}
            placeholder="Model (optional — server default applies)"
            title="Explicit model to route; empty uses the server's configured default, never a baked-in ID"
            className="px-4 py-2 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] text-[var(--color-text)] text-xs font-mono placeholder:text-[var(--color-faint)] focus:outline-none focus:border-[var(--color-info-edge)]"
          />
          <button
            onClick={handleTestRoute}
            disabled={routingLoading}
            className="px-4 py-2 rounded-xl bg-[var(--color-warning-wash)] hover:opacity-90 border border-[var(--color-warning-edge)] text-[var(--color-warning-ink)] text-xs font-semibold flex items-center space-x-1.5 transition-all shadow-sm shrink-0"
          >
            {routingLoading ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Zap className="w-3.5 h-3.5" />}
            <span>Evaluate Route</span>
          </button>
        </div>

        {routingDecision && (
          <div className="p-3.5 rounded-xl bg-[var(--color-tile)] border border-[var(--color-border)] text-xs space-y-1.5 font-mono animate-fade-in">
            <div className="flex items-center justify-between">
              <span className="text-[var(--color-text)] font-semibold">
                Selected Model: <span className="text-[var(--color-info-ink)]">{routingDecision.model}</span>
              </span>
              <span className={`px-2 py-0.5 rounded text-[12px] ${routingDecision.air_gap_enforced ? 'bg-[var(--color-success-wash)] text-[var(--color-success-ink)] border border-[var(--color-success-edge)]' : 'bg-[var(--color-warning-wash)] text-[var(--color-warning-ink)] border border-[var(--color-warning-edge)]'}`}>
                {routingDecision.air_gap_enforced ? 'AIR-GAP ENFORCED' : 'CLOUD ESCALATION'}
              </span>
            </div>
            <p className="text-[var(--color-muted)] font-sans">{routingDecision.explanation}</p>
            <div className="flex items-center space-x-4 text-[12px] text-[var(--color-muted)]">
              <span>Provider: {routingDecision.provider}</span>
              <span>Estimated Cost: ${fmtFixed(routingDecision.estimated_cost_usd, 2)}</span>
              <span>Modalities: {(routingDecision.modalities_supported || []).join(', ') || 'general'}</span>
            </div>
          </div>
        )}
        {routeError && (
          <div className="p-2.5 rounded-xl bg-[var(--color-danger-wash)] border border-[var(--color-danger-edge)] text-[var(--color-danger-ink)] text-[12px]">
            {routeError}
          </div>
        )}
      </div>

      {systemAgents.length > 0 && (
        <div className="p-5 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] space-y-2">
          <div className="text-[12px] font-semibold text-[var(--color-label-secondary)] uppercase tracking-wider font-mono">
            Tribunal system roles ({systemAgents.length})
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
            {systemAgents.map((role) => (
              <div key={role.role} className="p-2.5 rounded-xl bg-[var(--color-tile)] border border-[var(--color-border)] text-[12px]">
                <div className="font-code text-[var(--color-info-ink)]">{role.role}</div>
                <p className="text-[var(--color-muted)] font-sans mt-0.5">{role.description}</p>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Agents Matrix: Standard System + User-Defined */}
      <div className="p-5 rounded-2xl bg-[var(--color-surface)] border border-[var(--color-border)] space-y-4">
        <div className="flex items-center justify-between text-xs">
          <div className="flex items-center space-x-2">
            <Users className="w-4 h-4 text-[var(--color-accent-ink)]" />
            <span className="font-semibold text-[var(--color-label-secondary)] uppercase tracking-wider font-mono text-[12px]">
              Scientific Agent Roster ({agents.length})
            </span>
          </div>

          <button
            onClick={() => { setMeshError(null); setIsCreatingAgent(true); }}
            className="px-3 py-1.5 rounded-xl bg-[var(--color-soft-indigo-wash)] hover:opacity-90 border border-[var(--color-soft-indigo-edge)] text-[var(--color-accent-ink)] text-xs font-semibold flex items-center space-x-1.5 transition-all shadow-sm"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>Create Custom Agent</span>
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {agents.map((agent) => (
            <div
              key={agent.id}
              className="p-4 rounded-xl bg-[var(--color-tile)] border border-[var(--color-border)] hover:border-[var(--color-accent-edge)] transition-all flex flex-col justify-between space-y-3"
            >
              <div className="space-y-1">
                <div className="flex items-center justify-between">
                  <div className="flex items-center space-x-2">
                    <span className="text-base">{agent.avatar_emoji}</span>
                    <span className="font-semibold text-[var(--color-text)] text-xs">{agent.name}</span>
                  </div>
                  <span
                    className={`text-[12px] font-mono px-2 py-0.5 rounded ${
                      agent.is_system_agent
                        ? 'bg-[var(--color-soft-indigo-wash)] text-[var(--color-accent-ink)] border border-[var(--color-soft-indigo-edge)]'
                        : 'bg-[var(--color-success-wash)] text-[var(--color-success-ink)] border border-[var(--color-success-edge)]'
                    }`}
                  >
                    {agent.is_system_agent ? 'SYSTEM' : 'USER-DEFINED'}
                  </span>
                </div>
                <p className="text-xs text-[var(--color-muted)] leading-relaxed font-sans">{agent.description}</p>
              </div>

              <div className="space-y-1.5 pt-2 border-t border-[var(--color-border)] text-[12px] font-mono">
                <div className="flex items-center justify-between text-[var(--color-muted)]">
                  <span>Model:</span>
                  <span className="text-[var(--color-info-ink)] truncate max-w-[200px]">{agent.preferred_model}</span>
                </div>
                <div className="flex items-center justify-between text-[var(--color-muted)]">
                  <span>Modalities:</span>
                  <span className="text-[var(--color-label-secondary)]">{agent.modalities.join(', ')}</span>
                </div>
                {!agent.is_system_agent && (
                  <div className="pt-2 flex justify-end">
                    <button
                      onClick={() => handleDeleteAgent(agent.id)}
                      className="text-[var(--color-danger-ink)] hover:opacity-80 text-xs flex items-center space-x-1"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                      <span>Remove</span>
                    </button>
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Modal: Create User-Defined Agent */}
      {isCreatingAgent && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 animate-fade-in">
          <div
            ref={createAgentPanelRef}
            role="dialog"
            aria-modal="true"
            aria-labelledby="create-agent-title"
            onKeyDown={(e) => {
              if (e.key === 'Escape') setIsCreatingAgent(false);
            }}
            className="bg-[var(--color-float)] border border-[var(--color-border-strong)] rounded-3xl max-w-xl w-full p-6 space-y-4 shadow-2xl"
          >
            <div className="flex items-center justify-between border-b border-[var(--color-border)] pb-3">
              <h3 id="create-agent-title" className="text-base font-bold text-[var(--color-text)] flex items-center space-x-2">
                <span>Define Custom Scientific Agent</span>
              </h3>
              <button
                onClick={() => setIsCreatingAgent(false)}
                aria-label="Close create agent dialog"
                className="p-1.5 rounded-lg text-[var(--color-muted)] hover:text-[var(--color-text)]"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <div className="space-y-3 text-xs">
              <div>
                <label className="text-[var(--color-label-secondary)] font-semibold block mb-1">Agent Name &amp; Emoji</label>
                <div className="flex space-x-2">
                  <input
                    type="text"
                    value={newAgentEmoji}
                    aria-label="Agent emoji"
                    onChange={(e) => setNewAgentEmoji(e.target.value)}
                    className="w-12 text-center p-2 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] text-[var(--color-text)]"
                  />
                  <input
                    type="text"
                    value={newAgentName}
                    aria-label="Agent name"
                    onChange={(e) => setNewAgentName(e.target.value)}
                    placeholder="e.g. Battery Crystallographer"
                    className="flex-1 p-2 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] text-[var(--color-text)] placeholder:text-[var(--color-faint)]"
                  />
                </div>
              </div>

              <div>
                <label className="text-[var(--color-label-secondary)] font-semibold block mb-1">Role Identifier</label>
                <input
                  type="text"
                  value={newAgentRole}
                  aria-label="Role identifier"
                  onChange={(e) => setNewAgentRole(e.target.value)}
                  placeholder="e.g. solid_state_crystallographer"
                  className="w-full p-2 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] text-[var(--color-text)] font-mono"
                />
              </div>

              <div>
                <label className="text-[var(--color-label-secondary)] font-semibold block mb-1">Short Description</label>
                <input
                  type="text"
                  value={newAgentDesc}
                  aria-label="Short description"
                  onChange={(e) => setNewAgentDesc(e.target.value)}
                  placeholder="e.g. Investigates halide perovskite crystal structures"
                  className="w-full p-2 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] text-[var(--color-text)]"
                />
              </div>

              <div>
                <label className="text-[var(--color-label-secondary)] font-semibold block mb-1">System Prompt / Instructions</label>
                <textarea
                  rows={4}
                  value={newAgentPrompt}
                  aria-label="System prompt"
                  onChange={(e) => setNewAgentPrompt(e.target.value)}
                  placeholder="Detailed scientific system prompt guiding the agent's reasoning..."
                  className="w-full p-2 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] text-[var(--color-text)] placeholder:text-[var(--color-faint)] font-mono text-[12px]"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-[var(--color-label-secondary)] font-semibold block mb-1">Preferred Model</label>
                  <input
                    type="text"
                    value={newAgentModel}
                    aria-label="Preferred model"
                    placeholder="e.g. local/qwen2.5-coder:32b (required — never assumed)"
                    onChange={(e) => setNewAgentModel(e.target.value)}
                    className="w-full p-2 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] text-[var(--color-text)] font-mono text-[12px]"
                  />
                </div>
                <div>
                  <label className="text-[var(--color-label-secondary)] font-semibold block mb-1">Spend Budget (USD)</label>
                  <input
                    type="number"
                    step="0.5"
                    value={newAgentBudget}
                    aria-label="Spend budget in USD"
                    onChange={(e) => setNewAgentBudget(e.target.value)}
                    className="w-full p-2 rounded-xl bg-[var(--color-field)] border border-[var(--color-border)] text-[var(--color-text)] font-mono"
                  />
                </div>
              </div>
            </div>

            <div className="pt-3 border-t border-[var(--color-border)] flex justify-end space-x-2">
              {/* Audit 2026-10-21: registration refusals set meshError,
              which renders on the page behind this modal's backdrop.
              The dialog carries its own copy so the refusal is visible
              where the user acted — the modal stays open with the form
              intact for retry. */}
              {meshError && (
                <p role="alert" className="flex-1 text-left text-[12px] text-[var(--color-danger-ink)] self-center pr-2">
                  {meshError}
                </p>
              )}
              <button
                onClick={() => setIsCreatingAgent(false)}
                className="px-4 py-1.5 rounded-xl bg-[var(--color-tile)] hover:bg-[var(--color-fill-hover)] border border-[var(--color-border)] text-[var(--color-label-secondary)] text-xs font-semibold"
              >
                Cancel
              </button>
              <button
                onClick={handleSaveCustomAgent}
                className="px-4 py-1.5 rounded-xl bg-[var(--color-accent)] hover:bg-[var(--color-lit)] text-[var(--color-on-accent)] text-xs font-semibold shadow-sm"
              >
                Save &amp; Register Agent
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
