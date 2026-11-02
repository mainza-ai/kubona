import { describe, expect, it } from 'vitest';
import { shouldRenderFrame } from './render-policy';

describe('3D render policy', () => {
  it('renders only while the page is visible and the canvas is in view', () => {
    expect(shouldRenderFrame(true, true)).toBe(true);
    expect(shouldRenderFrame(false, true)).toBe(false);
    expect(shouldRenderFrame(true, false)).toBe(false);
    expect(shouldRenderFrame(false, false)).toBe(false);
  });
});
