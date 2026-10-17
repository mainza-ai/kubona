'use client';

import React, { useState, useEffect, useCallback } from 'react';
import {
  Network,
  RefreshCw,
  Lock,
  Globe,
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
  saveLLMSettings,
  fetchCustomAgents,
  createCustomAgent,
  deleteCustomAgent,
  routeMultimodalPayload,
} from '@/lib/api';
import { fmtFixed } from '@/lib/guards';

interface CognitiveMeshProps {
  campaign?: CampaignState | null;
}

export function CognitiveMesh({ campaign }: CognitiveMeshProps) {
  const [endpoints, setEndpoints] = useState<RadarEndpoint[]>([]);
  const [radarRefused, setRadarRefused] = useState<string | null>(null);
  const [airGapped, setAirGapped] = useState(true);
  const [loading, setLoading] = useState(false);
  const [agents, setAgents] = useState<CustomAgentManifest[]>([]);
  const [isCreatingAgent, setIsCreatingAgent] = useState(false);
  const [meshError, setMeshError] = useState<string | null>(null);
  const [airGapError, setAirGapError] = useState<string | null>(null);
  const [routeError, setRouteError] = useState<string | null>(null);

  // New Agent Form
  const [newAgentName, setNewAgentName] = useState('');
  const [newAgentRole, setNewAgentRole] = useState('custom_theorist');
  const [newAgentDesc, setNewAgentDesc] = useState('');
  const [newAgentPrompt, setNewAgentPrompt] = useState('');
  const [newAgentModel, setNewAgentModel] = useState('local/qwen2.5-coder:32b');
  const [newAgentBudget, setNewAgentBudget] = useState('5.0');
  const [newAgentEmoji, setNewAgentEmoji] = useState('🧪');

  // Multimodal Router Test
  const [testPrompt, setTestPrompt] = useState('Evaluate Kohn-Sham failure for FeMo-cofactor');
  const [testModality, setTestModality] = useState<ModalityType>('molecule_3d');
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

  // Fetch-merge-save: never wipe unrelated settings with a partial toggle
  // write. Revert + banner on failure so the switch cannot lie.
  const toggleAirGap = async () => {
    const prev = airGapped;
    const nextVal = !airGapped;
    setAirGapped(nextVal);
    setAirGapError(null);
    try {
      const current = await fetchLLMSettings();
      if (!current) throw new Error('settings unreachable — nothing changed');
      const res = await saveLLMSettings({ ...(current as Record<string, unknown>), air_gapped: nextVal });
      if (!res.ok) {
        setAirGapped(prev);
        setAirGapError(`Air-gap save refused — reverted (${res.error || 'no reason'}).`);
      }
    } catch (err) {
      setAirGapped(prev);
      setAirGapError(`Air-gap save failed — reverted (${err instanceof Error ? err.message : String(err)}).`);
    }
  };

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

    return () => {
      mounted = false;
    };
  }, []);

  const handleSaveCustomAgent = async () => {
    if (!newAgentName.trim() || !newAgentPrompt.trim()) return;
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
    try {
      await createCustomAgent(manifest);
      setIsCreatingAgent(false);
      setNewAgentName('');
      setNewAgentDesc('');
      setNewAgentPrompt('');
      setMeshError(null);
      loadAgents();
    } catch (err) {
      setMeshError(`Failed to create agent — nothing registered (${err instanceof Error ? err.message : String(err)}).`);
    }
  };

  const handleDeleteAgent = async (agentId: string) => {
    try {
      await deleteCustomAgent(agentId);
      loadAgents();
    } catch (err) {
      setMeshError(`Failed to delete agent — nothing removed (${err instanceof Error ? err.message : String(err)}).`);
    }
  };

  const handleTestRoute = async () => {
    setRoutingLoading(true);
    setRouteError(null);
    try {
      const decision = await routeMultimodalPayload({
        prompt: testPrompt,
        role_hint: 'lead_theorist',
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
    <div className="relative rounded-3xl overflow-hidden bg-gradient-to-b from-[#090C13] via-[#0B0E17] to-[#06080D] border border-white/[0.08] shadow-2xl p-7 space-y-6">
      {meshError && (
        <div className="p-3 rounded-xl bg-rose-950/60 border border-rose-500/40 text-rose-200 text-xs">
          Mesh error — {meshError} Nothing was changed.
        </div>
      )}
      {airGapError && (
        <div className="p-3 rounded-xl bg-amber-950/60 border border-amber-500/40 text-amber-200 text-xs">
          {airGapError}
        </div>
      )}
      {/* Serene Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-white/[0.08] pb-5">
        <div className="flex items-center space-x-3.5">
          <div className="w-10 h-10 rounded-2xl bg-indigo-500/10 border border-indigo-500/20 backdrop-blur-xl flex items-center justify-center shadow-inner">
            <Network className="w-5 h-5 text-indigo-400" />
          </div>
          <div>
            <h2 className="text-base font-semibold text-white tracking-tight flex items-center space-x-2">
              <span>Sovereign Cognitive Mesh &amp; Mission Control</span>
              <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
            </h2>
            <p className="text-xs text-slate-400 mt-0.5">
              Multimodal model routing, local zero-config discovery, and declarative agent orchestration
              {campaign?.target_system ? ` — active campaign: ${campaign.target_system}` : ''}
            </p>
          </div>
        </div>

        <div className="flex items-center space-x-3">
          <button
            onClick={scanRadar}
            disabled={loading}
            className="px-3 py-1.5 bg-white/[0.04] hover:bg-white/[0.08] text-slate-200 text-xs font-semibold rounded-xl flex items-center space-x-1.5 border border-white/10 transition-colors shadow-sm"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
            <span>Scan Ports</span>
          </button>

          {/* Sovereign Air-Gap Toggle */}
          <button
            onClick={toggleAirGap}
            className={`px-3.5 py-1.5 rounded-xl text-xs font-semibold flex items-center space-x-2 border transition-all shadow-sm active:scale-95 ${
              airGapped
                ? 'bg-emerald-950/70 border-emerald-500/40 text-emerald-300 ring-1 ring-emerald-500/20'
                : 'bg-white/[0.04] hover:bg-white/[0.08] border-white/10 text-slate-300'
            }`}
          >
            {airGapped ? (
              <>
                <Lock className="w-3.5 h-3.5 text-emerald-400" />
                <span>Air-Gap: ENGAGED</span>
              </>
            ) : (
              <>
                <Globe className="w-3.5 h-3.5 text-slate-400" />
                <span>Air-Gap: OFF</span>
              </>
            )}
          </button>
        </div>
      </div>

      {/* Host Port Radar Discovery */}
      <div className="p-5 rounded-2xl bg-black/40 border border-white/[0.06] backdrop-blur-xl space-y-3">
        <div className="flex items-center justify-between text-xs">
          <span className="font-semibold text-slate-300 uppercase tracking-wider font-mono text-[11px] flex items-center space-x-2">
            <Terminal className="w-4 h-4 text-cyan-400" />
            <span>Local Host Port Radar (Zero-Config Discovery)</span>
          </span>
          <span className="text-slate-400 font-mono text-[10px]">
            Target: 127.0.0.1 (Loopback Only)
          </span>
        </div>
        {radarRefused && (
          <div className="p-2.5 rounded-xl bg-rose-950/60 border border-rose-500/40 text-rose-200 text-[11px]">
            Radar refused — {radarRefused.slice(0, 200)}. Nothing discovered, nothing assumed.
          </div>
        )}
        {!radarRefused && endpoints.length === 0 && (
          <div className="p-2.5 rounded-xl bg-white/[0.03] border border-white/10 text-slate-400 text-[11px] font-code">
            No endpoints discovered — radar reachable but empty (not a refusal).
          </div>
        )}

        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {endpoints.map((ep) => (
            <div
              key={ep.port}
              className="p-3.5 rounded-xl bg-white/[0.02] border border-white/[0.05] flex items-center justify-between text-xs font-mono hover:border-cyan-500/30 transition-all"
            >
              <div>
                <div className="font-semibold text-white">{ep.service}</div>
                <div className="text-slate-300 text-xs font-mono">127.0.0.1:{ep.port}</div>
              </div>
              <span
                className={`px-2.5 py-0.5 rounded-full text-[10px] font-semibold flex items-center space-x-1.5 border ${
                  ep.listening
                    ? 'bg-emerald-950/60 text-emerald-300 border-emerald-800/60'
                    : 'bg-white/[0.04] text-slate-300 border-white/10'
                }`}
              >
                <span className={`w-1.5 h-1.5 rounded-full ${ep.listening ? 'bg-emerald-400 animate-pulse' : 'bg-slate-400'}`} />
                <span>{ep.listening ? 'ONLINE' : 'IDLE'}</span>
              </span>
            </div>
          ))}
        </div>
      </div>

      {/* Multimodal Mission Control Route Simulator */}
      <div className="p-5 rounded-2xl bg-black/40 border border-white/[0.06] backdrop-blur-xl space-y-3">
        <div className="flex items-center justify-between text-xs">
          <span className="font-semibold text-slate-300 uppercase tracking-wider font-mono text-[11px] flex items-center space-x-2">
            <Zap className="w-4 h-4 text-amber-400" />
            <span>Mission Control Multimodal Router Evaluator</span>
          </span>
          <span className="text-slate-400 font-mono text-[10px]">
            Local-First &amp; Air-Gap Policy Enforcement
          </span>
        </div>

        <div className="flex flex-col md:flex-row items-stretch gap-3">
          <input
            type="text"
            value={testPrompt}
            onChange={(e) => setTestPrompt(e.target.value)}
            placeholder="Scientific task prompt..."
            className="flex-1 px-4 py-2 rounded-xl bg-black/60 border border-white/10 text-white text-xs placeholder:text-slate-500 focus:outline-none focus:border-cyan-400"
          />
          <select
            value={testModality}
            onChange={(e) => setTestModality(e.target.value as ModalityType)}
            className="px-3 py-2 rounded-xl bg-black/60 border border-white/10 text-white text-xs font-mono focus:outline-none"
          >
            <option value="text">Modality: Text / Paper</option>
            <option value="molecule_3d">Modality: 3D Molecule (CIF/PDB)</option>
            <option value="circuit_qir">Modality: Quantum Circuit (QASM/QIR)</option>
            <option value="spectroscopy">Modality: Spectroscopy / Vision</option>
            <option value="lean4_ast">Modality: Lean 4 AST</option>
          </select>
          <button
            onClick={handleTestRoute}
            disabled={routingLoading}
            className="px-4 py-2 rounded-xl bg-amber-500/20 hover:bg-amber-500/30 border border-amber-500/40 text-amber-200 text-xs font-semibold flex items-center space-x-1.5 transition-all shadow-md shrink-0"
          >
            {routingLoading ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Zap className="w-3.5 h-3.5" />}
            <span>Evaluate Route</span>
          </button>
        </div>

        {routingDecision && (
          <div className="p-3.5 rounded-xl bg-white/[0.02] border border-white/10 text-xs space-y-1.5 font-mono animate-fade-in">
            <div className="flex items-center justify-between">
              <span className="text-white font-semibold">
                Selected Model: <span className="text-cyan-300">{routingDecision.model}</span>
              </span>
              <span className={`px-2 py-0.5 rounded text-[10px] ${routingDecision.air_gap_enforced ? 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/30' : 'bg-amber-500/20 text-amber-300'}`}>
                {routingDecision.air_gap_enforced ? 'AIR-GAP ENFORCED' : 'CLOUD ESCALATION'}
              </span>
            </div>
            <p className="text-slate-400 font-sans">{routingDecision.explanation}</p>
            <div className="flex items-center space-x-4 text-[10px] text-slate-400">
              <span>Provider: {routingDecision.provider}</span>
              <span>Estimated Cost: ${fmtFixed(routingDecision.estimated_cost_usd, 2)}</span>
              <span>Modalities: {routingDecision.modalities_supported.join(', ')}</span>
            </div>
          </div>
        )}
        {routeError && (
          <div className="p-2.5 rounded-xl bg-rose-950/60 border border-rose-500/40 text-rose-200 text-[11px]">
            {routeError}
          </div>
        )}
      </div>

      {/* Agents Matrix: Standard System + User-Defined */}
      <div className="p-5 rounded-2xl bg-black/40 border border-white/[0.06] backdrop-blur-xl space-y-4">
        <div className="flex items-center justify-between text-xs">
          <div className="flex items-center space-x-2">
            <Users className="w-4 h-4 text-indigo-400" />
            <span className="font-semibold text-slate-300 uppercase tracking-wider font-mono text-[11px]">
              Scientific Agent Roster ({agents.length})
            </span>
          </div>

          <button
            onClick={() => setIsCreatingAgent(true)}
            className="px-3 py-1.5 rounded-xl bg-indigo-600/30 hover:bg-indigo-600/50 border border-indigo-500/40 text-indigo-200 text-xs font-semibold flex items-center space-x-1.5 transition-all shadow-sm"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>Create Custom Agent</span>
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {agents.map((agent) => (
            <div
              key={agent.id}
              className="p-4 rounded-xl bg-white/[0.02] border border-white/[0.05] hover:border-indigo-500/30 transition-all flex flex-col justify-between space-y-3"
            >
              <div className="space-y-1">
                <div className="flex items-center justify-between">
                  <div className="flex items-center space-x-2">
                    <span className="text-base">{agent.avatar_emoji}</span>
                    <span className="font-semibold text-white text-xs">{agent.name}</span>
                  </div>
                  <span
                    className={`text-[9px] font-mono px-2 py-0.5 rounded ${
                      agent.is_system_agent
                        ? 'bg-indigo-500/20 text-indigo-300 border border-indigo-500/30'
                        : 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/30'
                    }`}
                  >
                    {agent.is_system_agent ? 'SYSTEM' : 'USER-DEFINED'}
                  </span>
                </div>
                <p className="text-xs text-slate-400 leading-relaxed font-sans">{agent.description}</p>
              </div>

              <div className="space-y-1.5 pt-2 border-t border-white/[0.04] text-[10px] font-mono">
                <div className="flex items-center justify-between text-slate-400">
                  <span>Model:</span>
                  <span className="text-cyan-300 truncate max-w-[200px]">{agent.preferred_model}</span>
                </div>
                <div className="flex items-center justify-between text-slate-400">
                  <span>Modalities:</span>
                  <span className="text-slate-300">{agent.modalities.join(', ')}</span>
                </div>
                {!agent.is_system_agent && (
                  <div className="pt-2 flex justify-end">
                    <button
                      onClick={() => handleDeleteAgent(agent.id)}
                      className="text-rose-400 hover:text-rose-300 text-xs flex items-center space-x-1"
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
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-md animate-fade-in">
          <div className="bg-[#0D111A] border border-white/20 rounded-3xl max-w-xl w-full p-6 space-y-4 shadow-2xl">
            <div className="flex items-center justify-between border-b border-white/10 pb-3">
              <h3 className="text-base font-bold text-white flex items-center space-x-2">
                <span>Define Custom Scientific Agent</span>
              </h3>
              <button
                onClick={() => setIsCreatingAgent(false)}
                className="p-1.5 rounded-lg text-slate-400 hover:text-white"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <div className="space-y-3 text-xs">
              <div>
                <label className="text-slate-300 font-semibold block mb-1">Agent Name &amp; Emoji</label>
                <div className="flex space-x-2">
                  <input
                    type="text"
                    value={newAgentEmoji}
                    onChange={(e) => setNewAgentEmoji(e.target.value)}
                    className="w-12 text-center p-2 rounded-xl bg-black/40 border border-white/10 text-white"
                  />
                  <input
                    type="text"
                    value={newAgentName}
                    onChange={(e) => setNewAgentName(e.target.value)}
                    placeholder="e.g. Battery Crystallographer"
                    className="flex-1 p-2 rounded-xl bg-black/40 border border-white/10 text-white placeholder:text-slate-500"
                  />
                </div>
              </div>

              <div>
                <label className="text-slate-300 font-semibold block mb-1">Role Identifier</label>
                <input
                  type="text"
                  value={newAgentRole}
                  onChange={(e) => setNewAgentRole(e.target.value)}
                  placeholder="e.g. solid_state_crystallographer"
                  className="w-full p-2 rounded-xl bg-black/40 border border-white/10 text-white font-mono"
                />
              </div>

              <div>
                <label className="text-slate-300 font-semibold block mb-1">Short Description</label>
                <input
                  type="text"
                  value={newAgentDesc}
                  onChange={(e) => setNewAgentDesc(e.target.value)}
                  placeholder="e.g. Investigates halide perovskite crystal structures"
                  className="w-full p-2 rounded-xl bg-black/40 border border-white/10 text-white"
                />
              </div>

              <div>
                <label className="text-slate-300 font-semibold block mb-1">System Prompt / Instructions</label>
                <textarea
                  rows={4}
                  value={newAgentPrompt}
                  onChange={(e) => setNewAgentPrompt(e.target.value)}
                  placeholder="Detailed scientific system prompt guiding the agent's reasoning..."
                  className="w-full p-2 rounded-xl bg-black/40 border border-white/10 text-white placeholder:text-slate-500 font-mono text-[11px]"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-slate-300 font-semibold block mb-1">Preferred Model</label>
                  <input
                    type="text"
                    value={newAgentModel}
                    onChange={(e) => setNewAgentModel(e.target.value)}
                    className="w-full p-2 rounded-xl bg-black/40 border border-white/10 text-white font-mono text-[11px]"
                  />
                </div>
                <div>
                  <label className="text-slate-300 font-semibold block mb-1">Spend Budget (USD)</label>
                  <input
                    type="number"
                    step="0.5"
                    value={newAgentBudget}
                    onChange={(e) => setNewAgentBudget(e.target.value)}
                    className="w-full p-2 rounded-xl bg-black/40 border border-white/10 text-white font-mono"
                  />
                </div>
              </div>
            </div>

            <div className="pt-3 border-t border-white/10 flex justify-end space-x-2">
              <button
                onClick={() => setIsCreatingAgent(false)}
                className="px-4 py-1.5 rounded-xl bg-white/[0.06] hover:bg-white/[0.1] text-slate-300 text-xs font-semibold"
              >
                Cancel
              </button>
              <button
                onClick={handleSaveCustomAgent}
                className="px-4 py-1.5 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold shadow-md"
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
