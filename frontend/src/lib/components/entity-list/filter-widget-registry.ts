import type { Component } from 'svelte';
import type { FilterConfig, FilterOption } from '$lib/types/entity-list-schema';

import SelectFilter from './filter-widgets/SelectFilter.svelte';
import MultiFilter from './filter-widgets/MultiFilter.svelte';
import TypeaheadFilter from './filter-widgets/TypeaheadFilter.svelte';

export interface FilterWidgetProps {
  filter: FilterConfig;
  lang: string;
  controlPrefix: string;
  currentValue: string;
  selectedValues: string[];
  options: FilterOption[];
  filteredOptions: FilterOption[];
  typeaheadQuery: string;
  onfilterchange: (paramName: string, value: string) => void;
  ontogglevalue: (filter: FilterConfig, value: string) => void;
  onquerychange: (paramName: string, value: string) => void;
}

export type FilterWidgetComponent = Component<FilterWidgetProps>;

const registry = new Map<string, FilterWidgetComponent>();

export function registerFilterWidget(kind: string, component: FilterWidgetComponent) {
  if (!kind) return;
  registry.set(kind, component);
}

export function getFilterWidget(kind: string | undefined): FilterWidgetComponent | null {
  if (!kind) return null;
  return registry.get(kind) ?? null;
}

export function filterWidgetName(filter: FilterConfig): string {
  if (filter.options_type === 'typeahead') return 'typeahead';
  if (filter.multi) return 'multi';
  return 'select';
}

registerFilterWidget('select', SelectFilter);
registerFilterWidget('multi', MultiFilter);
registerFilterWidget('typeahead', TypeaheadFilter);
