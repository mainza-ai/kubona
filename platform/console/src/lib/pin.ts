// Probe-pin payload builders (R-24). Pure functions extracted from the
// SpatialCanvas3D click handler so the probe_pin contract is unit-tested
// without WebGL: every pin is a human touch (touched:true, actor:'human')
// and the amber stain halo keys off the same coordinates. The payload key
// set lives HERE only — the component must not duplicate these literals.

export interface PinValues {
  r: number;
  energy: number;
}

// pinValuesFromHit derives ledger coordinates from a terrain hit point.
// ptX/ptZ are terrain-local units; equilibriumR recenters the bond axis and
// computeEnergy maps radial offset to energy. Rounding matches the display
// contract (r: 4dp, energy: 5dp).
export function pinValuesFromHit(
  ptX: number,
  ptZ: number,
  equilibriumR: number,
  computeEnergy: (deltaR: number, z: number) => number,
): PinValues {
  const deltaR = ptX * 0.4;
  return {
    r: Number((equilibriumR + deltaR).toFixed(4)),
    energy: Number(computeEnergy(deltaR, ptZ).toFixed(5)),
  };
}

// buildPinPayload returns the annotation payload for a probe_pin ledger
// record. touched:true is unconditional: a pin cannot exist without a
// human click, so there is no code path that mints an untouched pin.
export function buildPinPayload(values: PinValues): Record<string, unknown> {
  return {
    world_tool: 'probe_pin',
    touched: true,
    actor: 'human',
    bond_distance: values.r,
    energy: values.energy,
  };
}
