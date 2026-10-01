import rongqing from '../../docs/rp-runtime-r1/rongqing-product-world-spec-2026-10-01.json' with { type: 'json' }

export type StudioWorldPreset = 'custom' | 'rongqing'

// This is an explicitly authored declaration, submitted to the same Studio
// owner as custom worlds. It neither creates a world nor supplies live data.
export function authoredRongqingSpec(): Record<string, unknown> {
  return structuredClone(rongqing)
}
