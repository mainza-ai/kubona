'use client';

import React, { useState } from 'react';
import { KeyRound, ShieldCheck, DollarSign, Clock, Check, CheckCircle2, AlertOctagon, GitMerge, ArrowDownRight, RefreshCw, XCircle, Terminal } from 'lucide-react';
import { WebCryptoKeyring, signSpendMandate } from '../common/WebCryptoKeyring';

export function AutonomyCockpit() {
  const [signed, setSigned] = useState(false);
  const [signing, setSigning] = useState(false);
  const [verificationResult, setVerificationResult] = useState<any>(null);
  const [budgetUSD, setBudgetUSD] = useState(25.0);
  const [actionVerdict, setActionVerdict] = useState<string>('');
  const [actionDetails, setActionDetails] = useState<string>('');
  const [executingAction, setExecutingAction] = useState<string | null>(null);

  const tenantId = 'tenant-quantum-lab-1';
  const jobHash = 'sha256:88fa29810bce328a994781dfa98e21';

  const handleClientSign = async () => {
    setSigning(true);
    setActionDetails('');
    try {
      const expiry = new Date(Date.now() + 7200 * 1000).toISOString().replace(/\.\d{3}Z$/, 'Z');
      
      // Sign the mandate client-side in browser memory with RFC 8032 Ed25519
      const mandatePayload = signSpendMandate(tenantId, jobHash, budgetUSD, expiry);

      // Verify the client's signature with Go Core backend
      const res = await fetch('http://localhost:8080/api/mandates/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(mandatePayload),
      });

      const data = await res.json();
      if (data.valid) {
        setSigned(true);
        setActionVerdict('MANDATE_AUTHORIZED_CLIENT_SIGNED');
        setActionDetails(`Ed25519 signature verified by Go Core backend. Envelope recorded in SQLite WAL ledger at ${data.verified_at}`);
        setVerificationResult({
          tenant_id: tenantId,
          job_hash: jobHash,
          max_budget_usd: budgetUSD,
          public_key_hex: mandatePayload.public_key_hex,
          signature_hex: mandatePayload.signature_hex,
          signed_at: data.verified_at,
          verified: true,
        });
      } else {
        setActionVerdict('MANDATE_VERIFICATION_REJECTED');
        setActionDetails(`Verification error: ${data.error}`);
      }
    } catch (err: any) {
      console.error('Failed to verify client mandate:', err);
      setActionVerdict('MANDATE_DISPATCH_FAILED');
      setActionDetails(`Connection error: ${err?.message || err}`);
    } finally {
      setSigning(false);
    }
  };

  const executeActionGate = async (action: 'downgrade' | 'rescope' | 'kill', label: string) => {
    setExecutingAction(action);
    setActionDetails('');
    try {
      const res = await fetch(`http://localhost:8080/api/loops/loop-vqe-active/${action}`, {
        method: 'POST',
      });
      const data = await res.json();
      setActionVerdict(label);
      if (action === 'downgrade') {
        setActionDetails('Action Gate 2 executed: Quantum circuit re-routed to sovereign local Aer statevector simulator ($0.00 spend). Spend ceiling bypassed.');
      } else if (action === 'rescope') {
        setActionDetails('Action Gate 3 executed: Ring 0 Supervisor re-scoped variational ansatz. 2-qubit entangling layer depth halved to 3 layers, step budget clamped to 50.');
      } else if (action === 'kill') {
        setActionDetails('Action Gate 4 executed: Ring 0 emergency abort sequence initiated. Loop killed and QPU queue reservation cancelled.');
      }
    } catch (err: any) {
      setActionVerdict(`${label}_ERROR`);
      setActionDetails(`Failed to dispatch loop action: ${err?.message || err}`);
    } finally {
      setExecutingAction(null);
    }
  };

  return (
    <div className="bg-[#0A0D14] border border-slate-800 rounded-xl p-6 space-y-6 shadow-2xl">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-slate-800 pb-4">
        <div>
          <h2 className="text-base font-bold text-white flex items-center space-x-2">
            <KeyRound className="w-5 h-5 text-amber-400" />
            <span>Surface 4: Autonomy Cockpit &amp; WebCrypto AP2 Keyring</span>
          </h2>
          <p className="text-xs text-slate-400 mt-0.5">
            Browser client-side Ed25519 signing, four 1-glance action gate cards, and A2A delegation trees
          </p>
        </div>
        <div className="flex items-center space-x-2">
          <span className="px-2.5 py-1 rounded-lg bg-amber-950 border border-amber-800 text-amber-300 text-xs font-mono font-bold glow-amber">
            AP2 PROTOCOL ACTIVE
          </span>
        </div>
      </div>

      {/* Embedded WebCrypto Keyring Manager */}
      <WebCryptoKeyring />

      {/* Spend Authorization Card with Four 1-Glance Action Gate Cards */}
      <div className="bg-[#05070B] border border-slate-800 rounded-xl p-5 space-y-5">
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <div className="w-10 h-10 rounded-xl bg-indigo-950 border border-indigo-800 flex items-center justify-center">
              <DollarSign className="w-5 h-5 text-cyan-400" />
            </div>
            <div>
              <div className="text-sm font-bold text-white">Cloud QPU Spend Mandate Envelope</div>
              <div className="text-xs text-slate-400 font-mono">
                Tenant: {tenantId} | Target: IBM Heron (133 Qubits)
              </div>
            </div>
          </div>
          <div className="text-right">
            <div className="text-[10px] text-slate-500 font-bold uppercase">Authorized Ceiling</div>
            <div className="text-2xl font-bold font-mono text-emerald-400">${budgetUSD.toFixed(2)} USD</div>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-3 text-xs bg-[#0A0D14] p-3.5 rounded-lg border border-slate-800 font-mono">
          <div>
            <span className="text-slate-500">Predicted Shots:</span> <strong className="text-slate-200">2048</strong>
          </div>
          <div>
            <span className="text-slate-500">Clifford Fidelity:</span> <strong className="text-emerald-400">98.4%</strong>
          </div>
          <div>
            <span className="text-slate-500">Lease Expiry:</span> <strong className="text-cyan-400">2 Hours (7200s)</strong>
          </div>
        </div>

        {/* Four 1-Glance Ergonomic Action Gate Cards */}
        <div className="space-y-2">
          <div className="text-[11px] font-bold text-slate-400 uppercase tracking-wider">
            One-Glance Pre-Flight Action Gates:
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 pt-1">
            <button
              onClick={handleClientSign}
              disabled={signed || signing}
              className={`p-3 rounded-xl border text-left flex flex-col justify-between transition-all shadow ${
                signed
                  ? 'bg-emerald-950/40 border-emerald-800 text-emerald-300'
                  : 'bg-indigo-600/20 hover:bg-indigo-600/30 border-indigo-500/50 text-white'
              }`}
            >
              <div className="flex items-center justify-between">
                <span className="font-bold text-xs">[1] Authorize (Sign)</span>
                <KeyRound className="w-3.5 h-3.5 text-cyan-400" />
              </div>
              <span className="text-[10px] text-slate-400 mt-2">
                {signing ? 'Signing RFC 8032...' : signed ? 'Signed & Verified' : 'Sign mandate & dispatch QPU'}
              </span>
            </button>

            <button
              onClick={() => executeActionGate('downgrade', 'DOWNGRADED_TO_LOCAL_AER_SIM')}
              disabled={executingAction !== null}
              className="p-3 rounded-xl bg-slate-900 hover:bg-slate-800 border border-slate-800 text-left flex flex-col justify-between transition-all text-slate-200"
            >
              <div className="flex items-center justify-between">
                <span className="font-bold text-xs">[2] Downgrade</span>
                <ArrowDownRight className="w-3.5 h-3.5 text-amber-400" />
              </div>
              <span className="text-[10px] text-slate-400 mt-2">Re-route to local Aer ($0.00)</span>
            </button>

            <button
              onClick={() => executeActionGate('rescope', 'ANSATZ_RESCOPED_SHALLOW')}
              disabled={executingAction !== null}
              className="p-3 rounded-xl bg-slate-900 hover:bg-slate-800 border border-slate-800 text-left flex flex-col justify-between transition-all text-slate-200"
            >
              <div className="flex items-center justify-between">
                <span className="font-bold text-xs">[3] Re-Scope Ansatz</span>
                <RefreshCw className="w-3.5 h-3.5 text-indigo-400" />
              </div>
              <span className="text-[10px] text-slate-400 mt-2">Truncate entanglers (depth -50%)</span>
            </button>

            <button
              onClick={() => executeActionGate('kill', 'RING0_ABORT_TRIGGERED')}
              disabled={executingAction !== null}
              className="p-3 rounded-xl bg-red-950/20 hover:bg-red-950/30 border border-red-800/40 text-left flex flex-col justify-between transition-all text-red-300"
            >
              <div className="flex items-center justify-between">
                <span className="font-bold text-xs">[4] Abort Execution</span>
                <XCircle className="w-3.5 h-3.5 text-red-400" />
              </div>
              <span className="text-[10px] text-red-400/80 mt-2">Kill under Ring 0 authority</span>
            </button>
          </div>
        </div>

        {actionVerdict && (
          <div className="p-3 bg-emerald-950/20 border border-emerald-800/50 rounded-xl space-y-1">
            <div className="text-xs font-mono text-emerald-300 flex items-center space-x-2">
              <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0" />
              <span>Action Verdict: <strong>{actionVerdict}</strong></span>
            </div>
            {actionDetails && (
              <div className="text-[11px] text-slate-400 font-mono pl-6">
                {actionDetails}
              </div>
            )}
          </div>
        )}

        {verificationResult && (
          <div className="p-3.5 bg-[#0A0D14] border border-cyan-900/40 rounded-xl space-y-2 text-xs font-mono">
            <div className="flex items-center justify-between text-cyan-300 font-bold border-b border-cyan-900/30 pb-1.5">
              <span className="flex items-center space-x-1.5">
                <ShieldCheck className="w-4 h-4 text-cyan-400" />
                <span>Cryptographic Mandate Proof (Go Verified)</span>
              </span>
              <span className="text-[10px] text-emerald-400">STATUS: ED25519_VALID</span>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-[11px] text-slate-400 pt-1">
              <div>
                <span className="text-slate-500">Signer PubKey:</span>
                <div className="truncate text-slate-300">{verificationResult.public_key_hex}</div>
              </div>
              <div>
                <span className="text-slate-500">Signature:</span>
                <div className="truncate text-indigo-300">{verificationResult.signature_hex}</div>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* A2A Delegation Hierarchy Tree */}
      <div className="bg-[#05070B] border border-slate-800 rounded-xl p-5 space-y-3">
        <h3 className="text-xs font-bold text-slate-300 uppercase tracking-wider flex items-center space-x-2">
          <GitMerge className="w-4 h-4 text-indigo-400" />
          <span>A2A (Agent-to-Agent) Delegation Hierarchy &amp; Spend Leases</span>
        </h3>
        <div className="p-4 bg-[#0A0D14] border border-slate-800 rounded-lg text-xs font-mono space-y-2">
          <div className="text-indigo-400 font-bold">
            Human Reviewer (Root WebCrypto Keyholder: $150.00 Spend Ceiling)
          </div>
          <div className="pl-4 border-l border-slate-700 space-y-2">
            <div className="text-slate-300">
              └── Ansatz Scout (Delegated Child Key: $25.00 Ceiling | Burn: $0.00)
            </div>
            <div className="pl-4 border-l border-slate-700 space-y-1.5">
              <div className="text-slate-400">
                └── Device Auditor (Clifford Canary Probe Only: $0.00 Ceiling, Read-Only)
              </div>
              <div className="text-slate-400">
                └── Disconfirmation Officer (PySCF FCI Local Anchor: $0.00 Ceiling, Read-Only)
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
