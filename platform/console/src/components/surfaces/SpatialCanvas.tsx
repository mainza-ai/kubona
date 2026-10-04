'use client';

import React, { useState } from 'react';
import {
  ReactFlow,
  Controls,
  Background,
  BackgroundVariant,
  Node,
  Edge,
  Handle,
  Position,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { Sparkles, Scissors, Users, Play, Cpu, ShieldCheck, Eye, Layers, ZoomIn } from 'lucide-react';
import { MathText } from '../common/MathText';

// Custom Universal Post-QIR Gate Node
function QuantumGateNode({ data }: { data: any }) {
  return (
    <div className="px-3 py-2 rounded-xl bg-[#0A0D14] border border-cyan-500/50 shadow-lg text-xs font-mono text-cyan-300 min-w-[120px] text-center relative group hover:border-cyan-400 transition-all">
      <Handle type="target" position={Position.Left} className="w-2 h-2 bg-cyan-400" />
      <div className="text-[10px] text-slate-400 uppercase font-semibold">{data.type || 'Gate'}</div>
      <div className="text-sm font-bold text-white mt-0.5">{data.label}</div>
      {data.angle !== undefined && (
        <div className="text-[10px] text-amber-400 mt-1">
          θ = {Number(data.angle).toFixed(3)}
        </div>
      )}
      <Handle type="source" position={Position.Right} className="w-2 h-2 bg-cyan-400" />
    </div>
  );
}

// Custom LOCC Wire Cut Node
function WireCutNode({ data }: { data: any }) {
  return (
    <div className="px-3.5 py-2.5 rounded-xl bg-[#451A03]/90 border border-dashed border-amber-500 shadow-xl text-xs font-mono text-amber-200 min-w-[160px] text-center">
      <Handle type="target" position={Position.Left} className="w-2 h-2 bg-amber-400" />
      <div className="flex items-center justify-center space-x-1.5 font-bold text-amber-300">
        <Scissors className="w-3.5 h-3.5 text-amber-400" />
        <span>LOCC Wire Cut</span>
      </div>
      <div className="text-[10px] text-amber-400/80 mt-1">QPD Overhead: γ² = 4.0x</div>
      <Handle type="source" position={Position.Right} className="w-2 h-2 bg-amber-400" />
    </div>
  );
}

const nodeTypes = {
  quantumGate: QuantumGateNode,
  wireCut: WireCutNode,
};

const initialNodes: Node[] = [
  {
    id: 'q0-in',
    type: 'quantumGate',
    position: { x: 50, y: 100 },
    data: { type: 'Qubit Wire', label: 'q[0]: |0⟩' },
  },
  {
    id: 'h-gate',
    type: 'quantumGate',
    position: { x: 220, y: 100 },
    data: { type: 'Single Qubit', label: '[ H ]' },
  },
  {
    id: 'rz-gate',
    type: 'quantumGate',
    position: { x: 380, y: 100 },
    data: { type: 'Param Rotation', label: '[ Rz(θ₁) ]', angle: 0.785 },
  },
  {
    id: 'q1-in',
    type: 'quantumGate',
    position: { x: 50, y: 220 },
    data: { type: 'Qubit Wire', label: 'q[1]: |0⟩' },
  },
  {
    id: 'ry-gate',
    type: 'quantumGate',
    position: { x: 220, y: 220 },
    data: { type: 'Param Rotation', label: '[ Ry(θ₂) ]', angle: 1.571 },
  },
  {
    id: 'cut-1',
    type: 'wireCut',
    position: { x: 550, y: 160 },
    data: { label: 'KaHyPar QPD Wire Cut' },
  },
  {
    id: 'sub-1',
    type: 'quantumGate',
    position: { x: 780, y: 80 },
    data: { type: 'Subcircuit A', label: 'Qiskit Aer Sim' },
  },
  {
    id: 'sub-2',
    type: 'quantumGate',
    position: { x: 780, y: 240 },
    data: { type: 'Subcircuit B', label: 'IBM Heron QPU' },
  },
];

const initialEdges: Edge[] = [
  { id: 'e-q0-h', source: 'q0-in', target: 'h-gate', animated: true, style: { stroke: '#00f0ff' } },
  { id: 'e-h-rz', source: 'h-gate', target: 'rz-gate', animated: true, style: { stroke: '#00f0ff' } },
  { id: 'e-q1-ry', source: 'q1-in', target: 'ry-gate', animated: true, style: { stroke: '#00f0ff' } },
  { id: 'e-rz-cut', source: 'rz-gate', target: 'cut-1', animated: true, style: { stroke: '#f59e0b' } },
  { id: 'e-ry-cut', source: 'ry-gate', target: 'cut-1', animated: true, style: { stroke: '#f59e0b' } },
  { id: 'e-cut-sub1', source: 'cut-1', target: 'sub-1', animated: true, style: { stroke: '#10b981' } },
  { id: 'e-cut-sub2', source: 'cut-1', target: 'sub-2', animated: true, style: { stroke: '#8a2be2' } },
];

export function SpatialCanvas() {
  const [nodes, setNodes] = useState<Node[]>(initialNodes);
  const [edges, setEdges] = useState<Edge[]>(initialEdges);
  const [viewMode, setViewMode] = useState<'circuit' | 'galaxy' | 'qpu'>('circuit');
  const [executing, setExecuting] = useState(false);
  const [lastExecution, setLastExecution] = useState<{ id: string; status: string; shots: number } | null>(null);

  const handleExecute = async () => {
    setExecuting(true);
    try {
      const res = await fetch('http://localhost:8080/api/jobs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          session_id: 'spatial-canvas-session',
          qasm: 'OPENQASM 2.0;\ninclude "qelib1.inc";\nqreg q[2];\ncreg c[2];\nh q[0];\ncx q[0], q[1];\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];\n',
          backend: 'local_simulator',
          shots: 1024,
          budget_usd: 0.0,
        }),
      });
      if (res.ok) {
        const job = await res.json();
        setLastExecution({ id: job.id, status: job.status || 'SUBMITTED', shots: 1024 });
      }
    } catch (err) {
      console.error('Failed to run cut partition:', err);
    } finally {
      setExecuting(false);
    }
  };

  return (
    <div className="flex flex-col h-[740px] bg-[#05070B] border border-slate-800 rounded-xl overflow-hidden shadow-2xl relative">
      {/* Top Controls Bar */}
      <div className="h-12 bg-[#0A0D14] border-b border-slate-800 px-4 flex items-center justify-between text-xs">
        <div className="flex items-center space-x-3">
          <div className="flex items-center space-x-1.5 font-bold text-white">
            <Sparkles className="w-4 h-4 text-cyan-400" />
            <span>Surface 1: Spatial Living Lab Canvas</span>
          </div>
          <span className="px-2 py-0.5 rounded bg-cyan-950 text-cyan-300 border border-cyan-800 text-[10px] font-mono">
            React Flow @xyflow/react
          </span>
          {lastExecution && (
            <span className="px-2 py-0.5 rounded bg-emerald-950 text-emerald-300 border border-emerald-800 text-[10px] font-mono">
              Job #{lastExecution.id.slice(0, 8)} ({lastExecution.status})
            </span>
          )}
        </div>

        {/* View Mode Switcher & Execution */}
        <div className="flex items-center space-x-2">
          <div className="flex rounded-lg bg-slate-900 p-0.5 border border-slate-800 text-[11px]">
            <button
              onClick={() => setViewMode('galaxy')}
              className={`px-2.5 py-1 rounded-md transition-colors ${
                viewMode === 'galaxy' ? 'bg-indigo-600 text-white font-bold' : 'text-slate-400 hover:text-white'
              }`}
            >
              Galaxy (10⁻²x)
            </button>
            <button
              onClick={() => setViewMode('circuit')}
              className={`px-2.5 py-1 rounded-md transition-colors ${
                viewMode === 'circuit' ? 'bg-indigo-600 text-white font-bold' : 'text-slate-400 hover:text-white'
              }`}
            >
              Circuit (1x)
            </button>
            <button
              onClick={() => setViewMode('qpu')}
              className={`px-2.5 py-1 rounded-md transition-colors ${
                viewMode === 'qpu' ? 'bg-indigo-600 text-white font-bold' : 'text-slate-400 hover:text-white'
              }`}
            >
              QPU Lattice (10²x)
            </button>
          </div>

          <button
            onClick={handleExecute}
            disabled={executing}
            className="px-3 py-1.5 bg-gradient-to-r from-indigo-600 to-cyan-600 hover:from-indigo-500 hover:to-cyan-500 text-white rounded-lg font-bold flex items-center space-x-1.5 shadow transition-all disabled:opacity-50 text-[11px]"
          >
            <Play className="w-3 h-3" />
            <span>{executing ? 'Executing...' : 'Execute on Local Aer'}</span>
          </button>
        </div>
      </div>

      {/* Main View Area */}
      {viewMode === 'circuit' && (
        <div className="flex-1 w-full h-full relative">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            fitView
            colorMode="dark"
          >
            <Background color="#1e293b" gap={20} variant={BackgroundVariant.Dots} />
            <Controls className="bg-[#0A0D14] border border-slate-800 text-slate-300 rounded-lg shadow-xl" />
          </ReactFlow>
        </div>
      )}

      {viewMode === 'galaxy' && (
        <div className="flex-1 flex flex-col items-center justify-center p-8 bg-[#05070B] text-center space-y-4">
          <Layers className="w-12 h-12 text-indigo-400 animate-pulse" />
          <h3 className="text-base font-bold text-white">Galaxy Research Campaign View (10⁻²x)</h3>
          <p className="text-xs text-slate-400 max-w-md leading-relaxed">
            Displays concurrent multi-agent campaigns, meta-loop branches, and cross-session active space explorations across the entire laboratory.
          </p>
          <div className="grid grid-cols-3 gap-4 text-xs font-mono">
            <div className="p-3 bg-slate-900 border border-slate-800 rounded-lg">
              <div className="text-slate-500">Campaign 1</div>
              <div className="text-cyan-300 font-bold">H2 Dissociation</div>
            </div>
            <div className="p-3 bg-slate-900 border border-slate-800 rounded-lg">
              <div className="text-slate-500">Campaign 2</div>
              <div className="text-emerald-300 font-bold">LiH Ground State</div>
            </div>
            <div className="p-3 bg-slate-900 border border-slate-800 rounded-lg">
              <div className="text-slate-500">Campaign 3</div>
              <div className="text-purple-300 font-bold">BeH2 Linear PES</div>
            </div>
          </div>
        </div>
      )}

      {viewMode === 'qpu' && (
        <div className="flex-1 flex flex-col p-6 bg-[#05070B] space-y-4 overflow-y-auto">
          <div className="flex items-center justify-between border-b border-slate-800 pb-3">
            <div>
              <h3 className="text-sm font-bold text-white flex items-center space-x-2">
                <Cpu className="w-4 h-4 text-cyan-400" />
                <span>Physical QPU Coupling Lattice &amp; Calibration Heatmap (10²x)</span>
              </h3>
              <p className="text-[11px] text-slate-400">
                133-Qubit Heavy-Hex IBM Heron Topology with T₁/T₂ decay and readout error rates
              </p>
            </div>
            <span className="px-2.5 py-1 rounded bg-cyan-950 text-cyan-300 border border-cyan-800 text-[10px] font-mono">
              Average T₁ = 180 µs | T₂ = 145 µs
            </span>
          </div>

          {/* Qubit Coupling Lattice Grid */}
          <div className="grid grid-cols-8 gap-3 p-4 bg-[#0A0D14] border border-slate-800 rounded-xl">
            {Array.from({ length: 16 }).map((_, i) => (
              <div
                key={i}
                className="p-3 rounded-lg bg-slate-900/90 border border-cyan-500/30 text-center space-y-1 group hover:border-cyan-400 transition-all cursor-pointer"
              >
                <div className="text-[10px] font-mono text-cyan-400 font-bold">Q[{i}]</div>
                <div className="text-[9px] font-mono text-slate-400">T₁: 175µs</div>
                <div className="text-[9px] font-mono text-emerald-400">ε: 0.12%</div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Floating Telemetry Badge Overlay */}
      <div className="absolute bottom-4 left-4 glass-panel p-3 rounded-xl text-xs space-y-1.5 pointer-events-none">
        <div className="flex items-center space-x-2 text-slate-300">
          <Scissors className="w-3.5 h-3.5 text-amber-400" />
          <span>PennyLane QPD Cut Overhead: <strong className="text-white font-mono">4.0x</strong></span>
        </div>
        <div className="flex items-center space-x-2 text-slate-300">
          <Cpu className="w-3.5 h-3.5 text-emerald-400" />
          <span>Subcircuits partitioned across: <strong className="text-emerald-300">2 Qubits Max</strong></span>
        </div>
        <div className="flex items-center space-x-2 text-slate-300">
          <ShieldCheck className="w-3.5 h-3.5 text-indigo-400" />
          <span>Post-QIR Universal Glyphs Verified</span>
        </div>
      </div>
    </div>
  );
}
