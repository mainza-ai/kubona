'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import type { CampaignState, ChatMessage, Proposal, RunPhase } from '@/types';
import { useTranscript } from '@/domain/transcript-store';
import { useWriteSettlement } from '@/domain/use-write-settlement';
import { newWriteRequestId, writeSettlementLabel } from '@/domain/write-settlement';
import { useAuthPhase } from '@/domain/auth-state';
import {
  clearMessages,
  createSession,
  fetchCampaignState,
  fetchJobs,
  fetchMessages,
  isTerminalJobStatus,
  jobStatusLabel,
  launchProposal,
  regenerateChat,
  rewriteChat,
  runTribunal,
  sendChatMessage,
  sendFeedback,
  streamChatMessage,
  wasRecentAuthFailure,
} from '@/lib/api';
import type { CanvasWidgetKind } from '@/components/sovereign/WidgetStage';

export type ChatAttachment = { name: string; text: string };
export type FollowUpSpec = { label: string; tone: 'accent' | 'success' | 'warning'; key: string };

type ActiveSession = { id: string } | { needsLogin: true };
const isNeedsLogin = (r: ActiveSession | null): r is { needsLogin: true } =>
  r !== null && 'needsLogin' in r;

export function formatCopilotTargetName(rawTarget: string): string {
  const raw = rawTarget || 'No campaign';
  const upper = raw.toUpperCase();
  if (upper === 'H2' || raw === 'H₂') return 'Hydrogen Molecule (H₂)';
  if (upper.includes('LIH')) return 'Lithium Hydride (LiH)';
  if (upper.includes('BEH2')) return 'Beryllium Hydride (BeH₂)';
  if (upper.includes('H2O')) return 'Water Molecule (H₂O)';
  if (upper.includes('N2')) return 'Dinitrogen (N₂)';
  if (upper.includes('CO')) return 'Carbon Monoxide (CO)';
  return raw;
}

export interface UseCopilotChatOptions {
  sessionId: string;
  campaign?: CampaignState | null;
  requiresSignIn?: boolean;
  disabled?: boolean;
  onSessionChange?: (newSessionId: string) => void;
  onCampaignUpdated?: (campaign: CampaignState) => void;
  onExperimentLaunched?: () => void;
  onRunStateChange?: (phase: RunPhase, detail: string) => void;
  onMaterialize?: (kind: CanvasWidgetKind, title: string, jobId?: string, sessionId?: string) => void;
}

export function useCopilotChat({
  sessionId,
  campaign,
  requiresSignIn = false,
  disabled = false,
  onSessionChange,
  onCampaignUpdated,
  onExperimentLaunched,
  onRunStateChange,
  onMaterialize,
}: UseCopilotChatOptions) {
  const {
    messages,
    loading,
    streamingId,
    errorMessage,
    setMessages,
    setLoading,
    setStreamingId,
    setErrorMessage,
  } = useTranscript(sessionId);

  const chatWrite = useWriteSettlement();
  const feedbackWrite = useWriteSettlement();
  const authPhase = useAuthPhase();
  const [elapsedSeconds, setElapsedSeconds] = useState(0);
  const [runningTribunal, setRunningTribunal] = useState(false);
  const [tribunalStage, setTribunalStage] = useState('Convening multi-agent scientific tribunal...');
  const [launchingId, setLaunchingId] = useState<string | null>(null);
  const [launchedNote, setLaunchedNote] = useState<Record<string, string>>({});
  const [launchedJobs, setLaunchedJobs] = useState<Record<string, string>>({});
  const [infoMessage, setInfoMessage] = useState<string | null>(null);
  const [needsLogin, setNeedsLogin] = useState(false);
  const [attachments, setAttachments] = useState<ChatAttachment[]>([]);
  const attachmentsRef = useRef<ChatAttachment[]>([]);
  const [clearing, setClearing] = useState(false);
  const [regenerating, setRegenerating] = useState(false);
  const [votes, setVotes] = useState<Record<string, 'up' | 'down'>>({});
  const [rewriting, setRewriting] = useState(false);

  const lastRequestIdRef = useRef<string | null>(null);
  const lastPromptRef = useRef<string>('');
  const mountedRef = useRef(true);
  const abortRef = useRef<AbortController | null>(null);
  const idCounter = useRef(0);
  const nextTempId = (prefix: string) => `${prefix}-${++idCounter.current}`;

  const authExpired =
    authPhase === 'expired' ||
    (authPhase !== 'anonymous' && typeof wasRecentAuthFailure === 'function' && wasRecentAuthFailure());

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      abortRef.current?.abort();
      abortRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (disabled) return;
    abortRef.current?.abort();
    abortRef.current = null;
    let live = true;
    if (sessionId) {
      fetchMessages(sessionId).then((msgs) => {
        if (!live || !mountedRef.current) return;
        if (msgs && msgs.length > 0) setMessages(msgs);
      });
    }
    return () => {
      live = false;
    };
  }, [disabled, sessionId, setMessages]);

  useEffect(() => {
    if (disabled || (!loading && !runningTribunal)) return;
    const initTimer = setTimeout(() => setElapsedSeconds(0), 0);
    const interval = setInterval(() => {
      setElapsedSeconds((prev) => prev + 1);
    }, 1000);
    return () => {
      clearTimeout(initTimer);
      clearInterval(interval);
    };
  }, [disabled, loading, runningTribunal]);

  const attach = useCallback((name: string, text: string) => {
    setAttachments((prev) => {
      const next = [...prev.filter((item) => item.name !== name), { name, text }];
      attachmentsRef.current = next;
      return next;
    });
  }, []);

  const detach = useCallback((name: string) => {
    setAttachments((prev) => {
      const next = prev.filter((item) => item.name !== name);
      attachmentsRef.current = next;
      return next;
    });
  }, []);

  const clearAttachments = useCallback(() => {
    attachmentsRef.current = [];
    setAttachments([]);
  }, []);

  const getActiveSessionId = async (): Promise<ActiveSession | null> => {
    if (sessionId) return { id: sessionId };
    if (requiresSignIn || authPhase === 'expired' || authPhase === 'forbidden') {
      return { needsLogin: true };
    }
    const newSess = await createSession('Quantum Research Campaign #1', 150.0);
    if (newSess?.id) {
      onSessionChange?.(newSess.id);
      return { id: newSess.id };
    }
    if (wasRecentAuthFailure()) return { needsLogin: true };
    return null;
  };

  const targetSystem = campaign?.target_system || 'No campaign';
  const targetDisplayName = campaign ? formatCopilotTargetName(campaign.target_system) : 'No campaign selected';
  const rawDiscrepancy = campaign?.classical_anchor?.discrepancy_mha;
  const discrepancy =
    campaign?.classical_anchor?.anchor_computed === true &&
    typeof rawDiscrepancy === 'number' &&
    Number.isFinite(rawDiscrepancy)
      ? rawDiscrepancy
      : undefined;
  const isChemicalAccuracyMet = discrepancy != null && discrepancy <= 1.6;

  const canRegenerate = messages.length > 0 && messages[messages.length - 1].role !== 'user';

  const runLaunchProposal = async (messageId: string, proposal: Proposal) => {
    if (launchingId) return;
    const active = await getActiveSessionId();
    if (active === null) return;
    if (isNeedsLogin(active)) {
      setNeedsLogin(true);
      return;
    }
    const activeSessionId = active.id;
    setLaunchingId(messageId);
    try {
      const res = await launchProposal(activeSessionId, proposal);
      if (res && 'job' in res && res.job) {
        const jobId = String(res.job.id);
        setLaunchedJobs((prev) => ({ ...prev, [messageId]: jobId }));
        setLaunchedNote((prev) => ({ ...prev, [messageId]: `Launched: job ${jobId.slice(0, 8)} · tracking…` }));
        onExperimentLaunched?.();
        let rounds = 0;
        const track = async () => {
          rounds += 1;
          const jobs = await fetchJobs(activeSessionId);
          const hit = jobs.find((j) => String(j.id) === jobId);
          if (hit?.status != null) {
            const label = jobStatusLabel(hit.status);
            const result = (hit as { result_json?: { error?: string; error_message?: string } }).result_json;
            const reasonText = result?.error || result?.error_message || '';
            const reason = reasonText ? ` · ${String(reasonText).slice(0, 160)}` : '';
            setLaunchedNote((prev) => ({
              ...prev,
              [messageId]: `Job ${jobId.slice(0, 8)} · ${label}${hit.completed_at ? ` · done ${new Date(hit.completed_at).toLocaleTimeString()}` : ''}${reason}`,
            }));
            if (isTerminalJobStatus(hit.status) || rounds >= 8) return;
          } else if (rounds >= 8) {
            setLaunchedNote((prev) => ({ ...prev, [messageId]: `Launched: job ${jobId.slice(0, 8)} · status unavailable` }));
            return;
          }
          setTimeout(track, 3000);
        };
        setTimeout(track, 3000);
      } else if (res && 'error' in res) {
        setLaunchedNote((prev) => ({ ...prev, [messageId]: `Launch refused: ${res.error}` }));
      } else {
        setLaunchedNote((prev) => ({ ...prev, [messageId]: 'Launch refused by backend — nothing dispatched.' }));
      }
    } catch {
      setLaunchedNote((prev) => ({ ...prev, [messageId]: 'Launch failed — nothing dispatched.' }));
    } finally {
      setLaunchingId(null);
    }
  };

  const launchLast = async () => {
    for (let i = messages.length - 1; i >= 0; i--) {
      const m = messages[i];
      if (m.role !== 'user' && m.proposal_json) {
        await runLaunchProposal(m.id, m.proposal_json);
        return;
      }
    }
    setInfoMessage('No proposal in this conversation yet — ask for a simulation first.');
    setTimeout(() => setInfoMessage(null), 4000);
  };

  const conveneTribunal = async (topic: string) => {
    if (runningTribunal || loading) return;
    setRunningTribunal(true);
    setTribunalStage('Ansatz Scout: Proposing low-depth quantum circuit parameterization...');
    onRunStateChange?.('tribunal', 'Ansatz Scout: proposing parameterization…');
    setErrorMessage(null);

    const active = await getActiveSessionId();
    if (active === null) {
      setRunningTribunal(false);
      onRunStateChange?.('idle', '');
      setErrorMessage('Could not create or resolve an active campaign session.');
      return;
    }
    if (isNeedsLogin(active)) {
      setRunningTribunal(false);
      onRunStateChange?.('idle', '');
      setNeedsLogin(true);
      return;
    }
    const activeSessionId = active.id;

    const tempUserMsg: ChatMessage = {
      id: nextTempId('temp-tribunal'),
      session_id: activeSessionId,
      role: 'user',
      content: `Convene Research Tribunal for: ${topic}`,
      created_at: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, tempUserMsg]);

    const runStartEpoch = new Date(tempUserMsg.created_at).getTime() - 1000;
    const turnPoll = setInterval(async () => {
      if (typeof document !== 'undefined' && document.hidden) return;
      if (!mountedRef.current) {
        clearInterval(turnPoll);
        return;
      }
      try {
        const fresh = await fetchMessages(activeSessionId);
        if (!mountedRef.current) return;
        const arrived = fresh.filter(
          (m) => m.role !== 'user' && new Date(m.created_at).getTime() >= runStartEpoch,
        );
        const lastTurn = arrived[arrived.length - 1];
        if (lastTurn) {
          const roleLabel = String(lastTurn.role).replace(/_/g, ' ');
          setTribunalStage(`${lastTurn.role}: turn recorded (${arrived.length} so far)…`);
          onRunStateChange?.('tribunal', `${roleLabel} turn recorded`);
          setMessages((prev) => {
            const ids = new Set(prev.map((m) => m.id));
            const missing = arrived.filter((m) => !ids.has(m.id));
            return missing.length > 0 ? [...prev, ...missing] : prev;
          });
        }
      } catch {
        /* transient poll failure */
      }
    }, 2500);

    try {
      const res = await runTribunal(activeSessionId, topic);
      if (!mountedRef.current) return;
      if (res && res.assistant_message) {
        const assistantWithTribunal: ChatMessage = {
          ...res.assistant_message,
          tribunal: res.tribunal,
        };
        setMessages((prev) => {
          const filtered = prev.filter((m) => m.id !== tempUserMsg.id);
          return [...filtered, tempUserMsg, assistantWithTribunal];
        });
        if (res.campaign && onCampaignUpdated) {
          onCampaignUpdated(res.campaign);
        } else if (onCampaignUpdated) {
          fetchCampaignState(activeSessionId).then((c) => {
            if (mountedRef.current && c) onCampaignUpdated(c);
          });
        }
        onExperimentLaunched?.();
      } else {
        setErrorMessage('Failed to convene tribunal. Check the model in Sovereign settings and the bridge status in Fleet Radar.');
      }
    } catch (err: unknown) {
      if (!mountedRef.current) return;
      setErrorMessage(err instanceof Error ? err.message : 'Tribunal connection error.');
    } finally {
      clearInterval(turnPoll);
      if (!mountedRef.current) return;
      setRunningTribunal(false);
      onRunStateChange?.('idle', '');
    }
  };

  const handleSlash = (raw: string) => {
    const [cmd, ...rest] = raw.slice(1).split(/\s+/);
    const arg = rest.join(' ').trim();
    if (cmd === 'tribunal') {
      void conveneTribunal(arg || `${targetSystem} ground state electronic structure`);
    } else if (cmd === 'launch') {
      void launchLast();
    } else if (cmd === 'help') {
      setInfoMessage('Commands: /tribunal <topic> · /launch (most recent proposal) · /help');
      setTimeout(() => setInfoMessage(null), 5000);
    } else {
      setInfoMessage(`Unknown command /${cmd}. Try /help.`);
      setTimeout(() => setInfoMessage(null), 4000);
    }
  };

  const send = async (rawTextInput: string, retryRequestId?: string) => {
    const rawText = rawTextInput.trim();
    if (!rawText || loading || runningTribunal) return;

    if (rawText.startsWith('/')) {
      handleSlash(rawText);
      return;
    }

    const currentAttachments = attachmentsRef.current;
    const attached = currentAttachments.map((a) => `[attached ${a.name}]\n${a.text}`).join('\n');
    const textToSend = [attached, rawText].filter(Boolean).join('\n');
    lastPromptRef.current = textToSend;
    if (currentAttachments.length > 0) clearAttachments();

    setErrorMessage(null);
    setInfoMessage(null);

    const active = await getActiveSessionId();
    if (active === null) {
      setErrorMessage('Could not create or resolve an active campaign session.');
      return;
    }
    if (isNeedsLogin(active)) {
      setNeedsLogin(true);
      return;
    }
    setNeedsLogin(false);
    const activeSessionId = active.id;
    const requestId = retryRequestId ?? newWriteRequestId();
    lastRequestIdRef.current = requestId;
    const writeId = chatWrite.begin('chat send');

    const tempUserMsg: ChatMessage = {
      id: nextTempId('temp-user'),
      session_id: activeSessionId,
      role: 'user',
      content: textToSend,
      created_at: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, tempUserMsg]);
    setLoading(true);
    onRunStateChange?.('thinking', textToSend);

    const liveId = nextTempId('live-assistant');
    setMessages((prev) => [
      ...prev,
      {
        id: liveId,
        session_id: activeSessionId,
        role: 'assistant',
        content: '',
        created_at: new Date().toISOString(),
      },
    ]);
    setStreamingId(liveId);
    const controller = new AbortController();
    abortRef.current = controller;

    const settleStream = (done: {
      user: ChatMessage;
      assistant: ChatMessage;
      proposal?: unknown;
      campaign?: CampaignState;
      duration_ms?: number;
      usage?: Record<string, number>;
    }) => {
      if (!mountedRef.current) return;
      setMessages((prev) => {
        const filtered = prev.filter((m) => m.id !== tempUserMsg.id && m.id !== liveId);
        const assistant: ChatMessage = { ...done.assistant };
        if (!assistant.proposal_json && done.proposal) {
          assistant.proposal_json = done.proposal as Proposal;
        }
        if (assistant.duration_ms == null && done.duration_ms != null) {
          assistant.duration_ms = done.duration_ms;
        }
        if (!assistant.usage && done.usage) {
          assistant.usage = {
            completion_tokens: done.usage.completion_tokens,
            prompt_tokens: done.usage.prompt_tokens,
            total_tokens: done.usage.total_tokens,
          };
        }
        return [...filtered, done.user, assistant];
      });
      if (done.campaign && onCampaignUpdated) {
        onCampaignUpdated(done.campaign);
      } else if (onCampaignUpdated) {
        fetchCampaignState(activeSessionId).then((c) => {
          if (mountedRef.current && c) onCampaignUpdated(c);
        });
      }
      onExperimentLaunched?.();
      chatWrite.applied(writeId, 'assistant reply stored');
    };

    try {
      const done = await streamChatMessage(
        activeSessionId,
        textToSend,
        (tok) => {
          if (!mountedRef.current) return;
          setMessages((prev) =>
            prev.map((m) => (m.id === liveId ? { ...m, content: m.content + tok } : m)),
          );
        },
        controller.signal,
        requestId,
      );
      settleStream(done);
    } catch (streamErr) {
      if (!mountedRef.current) return;
      if (wasRecentAuthFailure()) {
        setMessages((prev) => prev.filter((m) => m.id !== tempUserMsg.id && m.id !== liveId));
        setNeedsLogin(true);
        setLoading(false);
        setStreamingId(null);
        abortRef.current = null;
        onRunStateChange?.('idle', '');
        return;
      }
      if (streamErr instanceof DOMException && streamErr.name === 'AbortError') {
        setMessages((prev) => prev.filter((m) => m.id !== liveId));
        chatWrite.unknown(writeId, 'operator stopped; backend may still finish');
        setInfoMessage('Stopped. The backend run may still finish — its result will appear in history.');
        setTimeout(() => { if (mountedRef.current) setInfoMessage(null); }, 6000);
        setLoading(false);
        setStreamingId(null);
        abortRef.current = null;
        onRunStateChange?.('idle', '');
        return;
      }
      if (streamErr instanceof Error && /stream stalled/i.test(streamErr.message)) {
        setMessages((prev) => prev.filter((m) => m.id !== liveId));
        chatWrite.unknown(writeId, 'stream stalled; reply not confirmed');
        setErrorMessage(`${streamErr.message} — the backend went quiet. Retry re-sends the same prompt.`);
        return;
      }
      console.warn('Stream failed, falling back to unary chat:', streamErr);
      try {
        const res = await sendChatMessage(activeSessionId, textToSend, requestId);
        if (!mountedRef.current) return;
        if (res) {
          setMessages((prev) => {
            const filtered = prev.filter((m) => m.id !== tempUserMsg.id && m.id !== liveId);
            return [...filtered, res.user, res.assistant];
          });
          if (onCampaignUpdated) {
            fetchCampaignState(activeSessionId).then((c) => {
              if (mountedRef.current && c) onCampaignUpdated(c);
            });
          }
          onExperimentLaunched?.();
          chatWrite.applied(writeId, 'assistant reply stored (unary fallback)');
        } else {
          setMessages((prev) => prev.filter((m) => m.id !== liveId));
          chatWrite.unknown(writeId, 'unary reply not confirmed');
          setErrorMessage('AI engine did not respond. Check the model in Sovereign settings (dock → mesh → settings).');
        }
      } catch (err: unknown) {
        if (!mountedRef.current) return;
        setMessages((prev) => prev.filter((m) => m.id !== liveId));
        chatWrite.unknown(writeId, err instanceof Error ? err.message : 'connection error');
        setErrorMessage(err instanceof Error ? err.message : 'Connection error to backend.');
      }
    } finally {
      if (!mountedRef.current) return;
      setLoading(false);
      setStreamingId(null);
      onRunStateChange?.('idle', '');
    }
  };

  const stop = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  const retryStalled = () => {
    setErrorMessage(null);
    if (lastPromptRef.current) {
      void send(lastPromptRef.current, lastRequestIdRef.current || undefined);
    }
  };

  const confirmClearChat = async () => {
    if (clearing || loading || runningTribunal) return false;
    const active = await getActiveSessionId();
    if (active === null) return true;
    if (isNeedsLogin(active)) {
      setNeedsLogin(true);
      return true;
    }
    const activeSessionId = active.id;
    setClearing(true);
    try {
      const deleted = await clearMessages(activeSessionId);
      if (deleted != null) {
        setMessages([]);
        setVotes({});
        setInfoMessage(`Cleared ${deleted} message${deleted === 1 ? '' : 's'}.`);
        setTimeout(() => setInfoMessage(null), 4000);
      } else {
        setErrorMessage('Clear refused by backend — nothing was deleted.');
      }
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Clear failed.');
    } finally {
      setClearing(false);
    }
    return true;
  };

  const regenerate = async () => {
    if (regenerating || loading || runningTribunal || !canRegenerate) return;
    const active = await getActiveSessionId();
    if (active === null) return;
    if (isNeedsLogin(active)) {
      setNeedsLogin(true);
      return;
    }
    const activeSessionId = active.id;
    setRegenerating(true);
    setErrorMessage(null);
    onRunStateChange?.('thinking', 'regenerating last answer');
    try {
      const res = await regenerateChat(activeSessionId);
      if (res?.assistant) {
        setMessages((prev) => {
          const trimmed: ChatMessage[] = [...prev];
          while (trimmed.length > 0 && trimmed[trimmed.length - 1].role !== 'user') {
            trimmed.pop();
          }
          if (!trimmed.some((m) => m.id === res.assistant.id)) {
            trimmed.push(res.assistant);
          }
          return trimmed;
        });
        onExperimentLaunched?.();
      } else {
        setErrorMessage('Regenerate refused — no user message to answer, or backend error.');
      }
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Regenerate failed.');
    } finally {
      setRegenerating(false);
      onRunStateChange?.('idle', '');
    }
  };

  const vote = async (messageId: string, nextVote: 'up' | 'down') => {
    const active = await getActiveSessionId();
    if (active === null) return;
    if (isNeedsLogin(active)) {
      setNeedsLogin(true);
      return;
    }
    const activeSessionId = active.id;
    setVotes((prev) => ({ ...prev, [messageId]: nextVote }));
    const requestId = feedbackWrite.begin('chat feedback');
    const out = await sendFeedback(activeSessionId, messageId, nextVote);
    if (!out.ok) {
      feedbackWrite[out.unknown ? 'unknown' : 'refused'](requestId, out.error);
      setVotes((prev) => {
        const next = { ...prev };
        delete next[messageId];
        return next;
      });
      setErrorMessage(`Feedback not recorded — ${out.unknown ? 'backend unreachable' : 'backend refused'} (${out.error}).`);
    } else {
      feedbackWrite.applied(requestId, 'feedback recorded');
    }
  };

  const rewriteSave = async (messageId: string, draft: string): Promise<boolean> => {
    const trimmed = draft.trim();
    if (!trimmed || rewriting || loading || runningTribunal) return false;
    const active = await getActiveSessionId();
    if (active === null) return false;
    if (isNeedsLogin(active)) {
      setNeedsLogin(true);
      return false;
    }
    const activeSessionId = active.id;
    setRewriting(true);
    setErrorMessage(null);
    try {
      const rewritten = await rewriteChat(activeSessionId, messageId, trimmed);
      if (!rewritten) {
        setErrorMessage('Rewrite refused — only your own messages can be edited.');
        return false;
      }
      setMessages((prev) => {
        const idx = prev.findIndex((m) => m.id === messageId);
        const cut = idx === -1 ? prev : prev.slice(0, idx);
        return [...cut, rewritten];
      });
      onRunStateChange?.('thinking', 'answering edited prompt');
      const res = await regenerateChat(activeSessionId);
      if (res?.assistant) {
        setMessages((prev) => {
          if (prev.some((m) => m.id === res.assistant.id)) return prev;
          return [...prev, res.assistant];
        });
        onExperimentLaunched?.();
      } else {
        setErrorMessage('Regenerate after rewrite refused — branch is clean, retry manually.');
      }
      return true;
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Rewrite failed.');
      return false;
    } finally {
      setRewriting(false);
      onRunStateChange?.('idle', '');
    }
  };

  const lastAssistantForFollowUps = [...messages].reverse().find((m) => m.role !== 'user');
  const followUps: FollowUpSpec[] = lastAssistantForFollowUps?.proposal_json
    ? [
        { label: 'Launch it', tone: 'accent', key: 'launch-last' },
        { label: 'Inspect circuit', tone: 'accent', key: 'show-circuit' },
        { label: 'Tribunal review', tone: 'success', key: 'tribunal-topic' },
      ]
    : lastAssistantForFollowUps?.tribunal
      ? [
          { label: 'Inspect circuit', tone: 'accent', key: 'show-circuit' },
          { label: 'Shadow estimates', tone: 'accent', key: 'show-shadows' },
          { label: 'Active space', tone: 'warning', key: 'show-molecule' },
        ]
      : [
          { label: 'H₂ Bond PES', tone: 'warning', key: 'send-pes' },
          { label: `${targetSystem} Tribunal`, tone: 'success', key: 'tribunal-topic' },
          { label: 'Canary Fidelity', tone: 'accent', key: 'send-canary' },
          { label: 'Classical FCI Anchor', tone: 'warning', key: 'send-anchor' },
        ];

  const runFollowUp = (key: string) => {
    if (key === 'launch-last') {
      void launchLast();
    } else if (key === 'show-circuit') {
      onMaterialize?.('circuit', 'Circuit · live');
    } else if (key === 'show-shadows') {
      onMaterialize?.('shadows', 'Shadow estimates');
    } else if (key === 'show-molecule') {
      onMaterialize?.('molecule', 'Active space');
    } else if (key === 'tribunal-topic') {
      void conveneTribunal(`${targetDisplayName} equilibrium state`);
    } else if (key === 'send-pes') {
      void send('Calculate potential energy surface for H2');
    } else if (key === 'send-canary') {
      void send('Simulate Clifford RB canary on IBM Heron');
    } else if (key === 'send-anchor') {
      void send('Verify classical PySCF STO-3G ground truth');
    }
  };

  return {
    sessionId,
    messages,
    loading,
    streamingId,
    runningTribunal,
    tribunalStage,
    elapsedSeconds,
    errorMessage,
    infoMessage,
    needsLogin,
    authExpired,
    attachments,
    chatWriteLabel: writeSettlementLabel(chatWrite.write),
    feedbackWriteLabel: writeSettlementLabel(feedbackWrite.write),
    launchingId,
    launchedNote,
    launchedJobs,
    clearing,
    regenerating,
    rewriting,
    votes,
    canRegenerate,
    targetSystem,
    targetDisplayName,
    discrepancy,
    isChemicalAccuracyMet,
    followUps,
    send,
    stop,
    retryStalled,
    conveneTribunal,
    launchLast,
    launchProposal: runLaunchProposal,
    runFollowUp,
    attach,
    detach,
    clearAttachments,
    confirmClearChat,
    regenerate,
    vote,
    rewriteSave,
    setErrorMessage,
    onMaterialize,
  };
}

export type CopilotChatController = ReturnType<typeof useCopilotChat>;
