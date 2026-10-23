// Shared Morse potential-energy-surface physics (single source of truth).
//
// P0c remediation: equilibrium distances and dissociation energies were
// duplicated with DIVERGENT constants in SpatialCanvas.tsx (a=1.44,
// De=|FCI|*0.15, ad-hoc) vs SpatialCanvas3D.tsx (a=1.6, physical De table),
// so the same molecule showed two different energies in the 2D workbench
// and the 3D observatory. Both surfaces now import from here. The physical
// parameterization (experimental dissociation energies, Morse width a=1.6)
// is the grounded one; the ad-hoc |FCI|*0.15 scaling is retired.

/** Morse width parameter (1/Å). Shared by both surfaces. */
export const MORSE_WIDTH_A = 1.6;

/** Equilibrium bond distance in Ångströms per target system. */
export function equilibriumBondDistanceAngstrom(targetSystem?: string | null): number {
  const sys = (targetSystem || '').toUpperCase();
  if (sys.includes('LIH')) return 1.5957;
  if (sys.includes('BEH')) return 1.33;
  if (sys.includes('H2O')) return 0.958;
  if (sys.includes('N2')) return 1.098;
  if (sys.includes('CO')) return 1.128;
  return 0.7414; // H2 default
}

/**
 * Physical molecular bond dissociation energy De in Hartrees
 * (experimental values: LiH ~2.51 eV, H2 ~4.74 eV, CO ~11.1 eV,
 * N2 ~9.9 eV).
 */
export function dissociationEnergyHartree(targetSystem?: string | null): number {
  const sys = (targetSystem || '').toUpperCase();
  if (sys.includes('LIH')) return 0.092;
  if (sys.includes('BEH')) return 0.085;
  if (sys.includes('CO')) return 0.408;
  if (sys.includes('N2')) return 0.364;
  if (sys.includes('H2O')) return 0.18;
  return 0.174; // H2 default (~4.74 eV)
}

/**
 * Physical potential energy in Hartrees: Morse potential along bond
 * displacement deltaR (Å) plus harmonic confinement along the orthogonal
 * mode deltaZ — a localized physical 2D potential basin.
 */
export function morseEnergyHartree(
  deltaR_A: number,
  deltaZ_A: number,
  dissociationHa: number,
  groundHa: number,
  widthA: number = MORSE_WIDTH_A,
): number {
  const morseTerm = dissociationHa * Math.pow(1 - Math.exp(-widthA * deltaR_A), 2);
  const orthoHarmonic = 0.5 * dissociationHa * Math.pow(deltaZ_A, 2);
  return groundHa + morseTerm + orthoHarmonic;
}

/**
 * Visual elevation mapping: physical energy above ground state (deltaE)
 * to 3D mesh height without letting compressed nuclear-core repulsion
 * explode into vertical cliffs. Smooth C-infinity exponential saturation
 * gives a graceful, organic bowl without flat tabletop cuts.
 */
export function morseVisualHeight(energyHa: number, groundHa: number, dissociationHa: number): number {
  const deltaE = Math.max(0, energyHa - groundHa);
  const normalized = deltaE / (1.5 * dissociationHa);
  return 1.7 * (1.0 - Math.exp(-0.85 * normalized));
}
