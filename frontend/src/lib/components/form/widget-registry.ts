import type { Component } from 'svelte';
import type { FieldDef, LanguageInfo } from '$lib/types/form-schema';

import MultilingualText from './widgets/MultilingualText.svelte';
import MultilingualTextarea from './widgets/MultilingualTextarea.svelte';
import SystemNamePreview from './widgets/SystemNamePreview.svelte';
import SlugInput from './widgets/SlugInput.svelte';
import SelectWidget from './widgets/SelectWidget.svelte';
import SearchSelectWidget from './widgets/SearchSelectWidget.svelte';
import VocabularyEntryPicker from './widgets/VocabularyEntryPicker.svelte';
import PillMultiSelect from './widgets/PillMultiSelect.svelte';
import OntologyPathBuilder from './widgets/OntologyPathBuilder.svelte';
import SubfieldPathsDisplay from './widgets/SubfieldPathsDisplay.svelte';
import TextInput from './widgets/TextInput.svelte';
import PrefixInput from './widgets/PrefixInput.svelte';
import RadioGroup from './widgets/RadioGroup.svelte';
import ReadonlyStat from './widgets/ReadonlyStat.svelte';
import ReadonlyTable from './widgets/ReadonlyTable.svelte';
import Checkbox from './widgets/Checkbox.svelte';
import MultiSelect from './widgets/MultiSelect.svelte';
import OntologyTreeSelect from './widgets/OntologyTreeSelect.svelte';

export type FormWidgetProps = {
  field: FieldDef;
  value: any;
  formValues: Record<string, any>;
  lang: string;
  languages: LanguageInfo[];
  errors: string[];
};

export type FormWidgetComponent = Component<FormWidgetProps>;

export type FormWidgetMetadata = {
  emptyValue?: () => any;
};

type FormWidgetEntry = FormWidgetMetadata & {
  component: FormWidgetComponent;
};

const registry = new Map<string, FormWidgetEntry>();

export function registerWidget(kind: string, component: FormWidgetComponent, metadata: FormWidgetMetadata = {}) {
  if (!kind) return;
  registry.set(kind, { component, ...metadata });
}

export function registerWidgets(
  entries: Record<string, FormWidgetComponent>,
  metadata: Record<string, FormWidgetMetadata> = {},
) {
  for (const [kind, component] of Object.entries(entries)) {
    registerWidget(kind, component, metadata[kind]);
  }
}

export function resolveWidget(kind: string): FormWidgetComponent | undefined {
  return registry.get(kind)?.component;
}

export function initialValueForWidget(kind: string): any {
  return registry.get(kind)?.emptyValue?.() ?? null;
}

registerWidgets(
  {
    'multilingual-text': MultilingualText,
    'multilingual-textarea': MultilingualTextarea,
    'system-name-preview': SystemNamePreview,
    'slug-input': SlugInput,
    select: SelectWidget,
    'search-select': SearchSelectWidget,
    'vocabulary-entry-picker': VocabularyEntryPicker,
    'pill-multi-select': PillMultiSelect,
    'ontology-path': OntologyPathBuilder,
    'subfield-paths': SubfieldPathsDisplay,
    'prefix-input': PrefixInput,
    text: TextInput,
    number: TextInput,
    date: TextInput,
    url: TextInput,
    textarea: TextInput,
    password: TextInput,
    checkbox: Checkbox,
    'multi-select': MultiSelect,
    'ontology-tree': OntologyTreeSelect,
    'radio-group': RadioGroup,
    'readonly-stat': ReadonlyStat,
    'readonly-table': ReadonlyTable,
  },
  {
    'multilingual-text': { emptyValue: () => ({}) },
    'multilingual-textarea': { emptyValue: () => ({}) },
  },
);
