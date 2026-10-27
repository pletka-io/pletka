import type { FormSchema, FieldDef } from '../../types/form-schema.ts';

export interface FieldConflict { field: FieldDef; mine: unknown; latest: unknown }
export interface ConflictPlan {
  schema: FormSchema;
  latest: Record<string, any>;
  merged: Record<string, any>;
  conflicts: FieldConflict[];
}
function canonical(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonical).join(',')}]`;
  if (value && typeof value === 'object') {
    const record = value as Record<string, unknown>;
    return `{${Object.keys(record).sort().map(key => `${JSON.stringify(key)}:${canonical(record[key])}`).join(',')}}`;
  }
  return JSON.stringify(value) ?? 'undefined';
}
export function displayConflictValue(value: unknown): string {
  return typeof value === 'string' ? value : JSON.stringify(value, null, 2) ?? '';
}
/** Never advance a revision until the user reviews the plan. Inputs stay intact. */
export function planFormConflict(
  previous: FormSchema, baseline: Record<string, any>, mine: Record<string, any>, latestSchema: FormSchema,
  initialValue: (widget: string) => unknown = () => null,
): ConflictPlan {
  const config = previous.conflict;
  const latestConfig = latestSchema.conflict;
  if (!config || !latestConfig || config.identity !== latestConfig.identity || config.revision_field !== latestConfig.revision_field
    || !previous.endpoint || !latestSchema.endpoint || previous.endpoint.url !== latestSchema.endpoint.url
    || previous.endpoint.method !== latestSchema.endpoint.method) {
    throw new Error('The form structure changed. Your edits are preserved; open the latest draft in another tab to reconcile them.');
  }
  const oldFields = previous.sections.flatMap(section => section.fields);
  const fields = latestSchema.sections.flatMap(section => section.fields);
  const shape = (list: FieldDef[]) => list.map(f => [f.name, f.widget, !!f.readonly, !!f.item_add_url, !!f.item_remove_url_template]).sort((a, b) => String(a[0]).localeCompare(String(b[0])));
  if (canonical(shape(oldFields)) !== canonical(shape(fields))) {
    throw new Error('The form fields changed. Your unsaved edits are preserved; review the latest draft in another tab.');
  }
  const latest = Object.fromEntries(fields.map(field => [field.name, field.value ?? initialValue(field.widget)]));
  const revision = latest[config.revision_field];
  if (!Number.isSafeInteger(revision) || revision < 1 || revision < baseline[config.revision_field]) {
    throw new Error('The server did not return a valid current revision. Your unsaved edits are preserved.');
  }
  const merged = structuredClone(mine);
  const conflicts: FieldConflict[] = [];
  for (const field of fields) {
    const key = field.name;
    if (key === config.revision_field || canonical(mine[key]) === canonical(baseline[key])) {
      merged[key] = structuredClone(latest[key]);
    } else if (canonical(latest[key]) !== canonical(baseline[key]) && canonical(mine[key]) !== canonical(latest[key])) {
      conflicts.push({ field, mine: structuredClone(mine[key]), latest: structuredClone(latest[key]) });
    }
  }
  return { schema: latestSchema, latest, merged, conflicts };
}
export function resolveFormConflict(plan: ConflictPlan, choices: Record<string, 'mine' | 'latest'>): Record<string, any> {
  const result = structuredClone(plan.merged);
  for (const conflict of plan.conflicts) {
    const choice = choices[conflict.field.name];
    if (choice !== 'mine' && choice !== 'latest') throw new Error('Choose a value for each conflicting field.');
    result[conflict.field.name] = structuredClone(choice === 'mine' ? conflict.mine : conflict.latest);
  }
  return result;
}
