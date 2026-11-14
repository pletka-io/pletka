// Required and Min are one concept: Required ⇔ Min ≥ 1. The server keeps
// them in agreement on save; the editor mirrors that so whichever control
// the user changed decides the other (unticking Required sets Min to 0).
type RequiredMin = { is_required?: boolean; min_occurs?: number };

export function syncRequiredMin(prev: RequiredMin, next: RequiredMin): { is_required: boolean; min_occurs: number } {
  const required = !!next.is_required;
  const min = next.min_occurs ?? 0;
  if (required !== !!prev.is_required) {
    return { is_required: required, min_occurs: required ? Math.max(min, 1) : 0 };
  }
  return { is_required: min >= 1, min_occurs: min };
}
