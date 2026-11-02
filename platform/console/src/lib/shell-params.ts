import type { ArenaView } from '@/components/sovereign/ArenaHud';

export type DockSurface = 'arena' | 'recorder' | 'manifold' | 'fleet' | 'proofs' | 'mesh';
export type ProofsTab = 'manuscripts' | 'tribunal' | 'gallery' | 'circuits';
export type MeshTab = 'fleet' | 'mesh' | 'guardrails' | 'models' | 'labs' | 'settings';

export interface ShellParams {
  surface: DockSurface;
  proofsTab: ProofsTab;
  meshTab: MeshTab;
  arenaView: ArenaView;
}

const SHELL_DEFAULTS: ShellParams = {
  surface: 'arena',
  proofsTab: 'manuscripts',
  meshTab: 'mesh',
  arenaView: 'workbench',
};

const VALID_SURFACES: DockSurface[] = ['arena', 'recorder', 'manifold', 'fleet', 'proofs', 'mesh'];
const VALID_PROOFS_TABS: ProofsTab[] = ['manuscripts', 'tribunal', 'gallery', 'circuits'];
const VALID_MESH_TABS: MeshTab[] = ['fleet', 'mesh', 'guardrails', 'models', 'labs', 'settings'];
const VALID_ARENA_VIEWS: ArenaView[] = ['workbench', 'orbital', 'proof-dag', 'hardware-qpu'];

export function parseShellParams(search: string): ShellParams {
  const out: ShellParams = { ...SHELL_DEFAULTS };
  try {
    const params = new URLSearchParams(search);
    const surface = params.get('surface');
    if (surface !== null && (VALID_SURFACES as string[]).includes(surface)) {
      out.surface = surface as DockSurface;
    }
    const tab = params.get('tab');
    if (tab === null) return out;
    if (out.surface === 'proofs' && (VALID_PROOFS_TABS as string[]).includes(tab)) {
      out.proofsTab = tab as ProofsTab;
    } else if (out.surface === 'mesh' && (VALID_MESH_TABS as string[]).includes(tab)) {
      out.meshTab = tab as MeshTab;
    } else if (out.surface === 'arena' && (VALID_ARENA_VIEWS as string[]).includes(tab)) {
      out.arenaView = tab as ArenaView;
    }
    return out;
  } catch {
    return { ...SHELL_DEFAULTS };
  }
}

export function tabForSurface(
  surface: DockSurface,
  tabs: Pick<ShellParams, 'proofsTab' | 'meshTab' | 'arenaView'>,
): string | null {
  switch (surface) {
    case 'proofs':
      return tabs.proofsTab;
    case 'mesh':
      return tabs.meshTab;
    case 'arena':
      return tabs.arenaView;
    default:
      return null;
  }
}

export function buildShellSearch(surface: DockSurface, tab: string | null): string {
  const params = new URLSearchParams();
  params.set('surface', surface);
  if (tab !== null && tab !== '') params.set('tab', tab);
  const s = params.toString();
  return s ? `?${s}` : '';
}
