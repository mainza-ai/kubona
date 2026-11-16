'use client';

import { useSyncExternalStore } from 'react';
import type { WorkflowPlanDraft } from '@/types';

/**
 * C8 — the latest dry-run plan, published by the workflow planner and
 * projected on the canvas. Ephemeral by design: the plan is a draft that has
 * not executed, so it is not server state. Creating tasks attaches their ids
 * (idempotently) so the card can flip to "Open in Tasks".
 */

const STORAGE_KEY = 'kubona.workflow-plan.v1';

function readStored(): WorkflowPlanDraft | null {
  if (typeof window === 'undefined') return null;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as WorkflowPlanDraft;
    if (!parsed || typeof parsed !== 'object' || !Array.isArray(parsed.steps)) return null;
    return parsed;
  } catch {
    return null;
  }
}

function writeStored(draft: WorkflowPlanDraft | null): void {
  if (typeof window === 'undefined') return;
  try {
    if (draft) window.localStorage.setItem(STORAGE_KEY, JSON.stringify(draft));
    else window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    // Storage unavailable: keep the in-memory draft; never fail the planner.
  }
}

let latest: WorkflowPlanDraft | null = readStored();
const listeners = new Set<() => void>();

function emit(): void {
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function getSnapshot(): WorkflowPlanDraft | null {
  return latest;
}

function getServerSnapshot(): WorkflowPlanDraft | null {
  return null;
}

export function publishWorkflowPlan(draft: Omit<WorkflowPlanDraft, 'createdAt'> & { createdAt?: number }): void {
  latest = { ...draft, createdAt: draft.createdAt ?? Date.now() };
  writeStored(latest);
  emit();
}

export function setWorkflowPlanTaskIds(taskIds: string[]): void {
  if (!latest) return;
  latest = { ...latest, taskIds };
  writeStored(latest);
  emit();
}

export function getWorkflowPlan(): WorkflowPlanDraft | null {
  return latest;
}

export function clearWorkflowPlan(): void {
  latest = null;
  writeStored(null);
  emit();
}

export function useWorkflowPlan(sessionId?: string): WorkflowPlanDraft | null {
  const draft = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
  if (!draft || !sessionId || !draft.sessionId) return draft;
  return draft.sessionId === sessionId ? draft : null;
}

export function resetWorkflowPlanForTests(): void {
  latest = null;
  listeners.clear();
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.removeItem(STORAGE_KEY);
    } catch {
      // ignore
    }
  }
}
