<script lang="ts">
  import WidgetDispatcher from '$lib/components/form/WidgetDispatcher.svelte';
  import ConceptPicker from '$lib/components/example/ConceptPicker.svelte';
  import type { FieldDef, SelectOption, Translations } from '$lib/types/form-schema';
  import type { ExampleFormField, ExampleFormGroup, ExampleFormSchema, ExampleFormSection, ExampleIssue } from '$lib/types/example-form-schema';
  import { tr } from '$lib/types/form-schema';
  import { addToast } from '$lib/stores/toast';

  /** Schema-provided API endpoints (from EntityListSchema.editor.endpoints).
   *  The workspace fills {id} into url templates client-side; it never
   *  constructs an API path itself. */
  type WorkspaceEndpoints = {
    list_url: string;
    detail_url_template: string;
    form_schema_url: string;
    /** Browser URL of one example ({id} substituted client-side); '' when the schema omits it. */
    page_url_template: string;
  };

  let {
    projectId,
    endpoints,
    mode = 'create',
    exampleId = '',
    initialTargetEntityType = '',
    initialTargetEntityId = '',
    lang = 'en',
    oncancel,
    onsuccess,
  }: {
    projectId: string;
    endpoints: WorkspaceEndpoints;
    mode?: 'create' | 'edit';
    exampleId?: string;
    initialTargetEntityType?: string;
    initialTargetEntityId?: string;
    lang?: string;
    oncancel?: () => void;
    onsuccess?: () => void;
  } = $props();

  type ModelResult = {
    id: string;
    ui_name?: Translations;
    description?: Translations;
    system_name?: string;
  };

  type ExampleOption = {
    id: string;
    entity_type: string;
    entity_id: string;
    title?: Translations;
    status?: string;
  };

  type OccurrenceState = {
    occurrence_index: number;
    value: string;
    // Stub creation for example_ref fields: a typed label (and, when the
    // field allows several models, the chosen target) instead of `value`.
    stub_label?: string;
    stub_model?: string;
  };

  type FilterMode = 'all' | 'required' | 'issues';
  type WorkspaceMode = 'edit' | 'overview';

  type SummaryCounts = {
    required_total: number;
    required_filled: number;
    required_remaining: number;
    filled_fields: number;
    errors: number;
    warnings: number;
    has_value: boolean;
  };

  let loading = $state(true);
  let saving = $state(false);
  let error = $state('');
  let schema = $state<ExampleFormSchema | null>(null);
  let selectedModelId = $state('');
  let selectedModelLabel = $state('');
  let modelQuery = $state('');
  let modelResults = $state<ModelResult[]>([]);
  let initialModelResults = $state<ModelResult[]>([]);
  let loadingModels = $state(false);
  let availableExamples = $state<ExampleOption[]>([]);
  let titleValues = $state<Record<string, string>>({});
  let descriptionValues = $state<Record<string, string>>({});
  let valuesByOverride = $state<Record<string, OccurrenceState[]>>({});
  let conceptLabelsByURI = $state<Record<string, string>>({});
  let currentExampleId = $state('');
  let currentMode = $state<'create' | 'edit'>('create');
  let activeFilter = $state<FilterMode>('all');
  let expandedSections = $state<Record<string, boolean>>({});
  let expandedGroups = $state<Record<string, boolean>>({});
  let workspaceMode = $state<WorkspaceMode>('edit');
  let searchQuery = $state('');
  let searchIndex = $state(0);

  let modelAbort: AbortController | null = null;

  const primaryLang = $derived(schema?.ui.primary_language || lang || 'en');

  $effect(() => {
    init();
  });

  async function init() {
    loading = true;
    error = '';
    currentExampleId = exampleId;
    currentMode = mode;
    valuesByOverride = {};
    conceptLabelsByURI = {};
    schema = null;
    activeFilter = 'all';
    expandedSections = {};
    expandedGroups = {};
    workspaceMode = mode === 'edit' ? 'overview' : 'edit';
    searchQuery = '';
    searchIndex = 0;
    try {
      await loadAvailableExamples();
      if (mode === 'create') {
        if (initialTargetEntityType === 'model' && initialTargetEntityId) {
          selectedModelId = initialTargetEntityId;
          selectedModelLabel = initialTargetEntityId;
          modelQuery = initialTargetEntityId;
          await loadSchemaForCreate(initialTargetEntityId);
        } else {
          await loadInitialModels();
        }
      }
      if (mode === 'edit' && exampleId) {
        await loadExistingExample(exampleId);
      }
    } catch (e: any) {
      error = e?.message || 'Failed to load example workspace';
    } finally {
      loading = false;
    }
  }

  async function loadAvailableExamples() {
    const res = await fetch(`${endpoints.list_url}?per_page=500`);
    if (!res.ok) throw new Error(`Failed to load examples: ${res.status}`);
    const data = await res.json();
    availableExamples = data.items ?? [];
  }

  async function loadExistingExample(id: string) {
    const res = await fetch(endpoints.detail_url_template.replace('{id}', id));
    if (!res.ok) throw new Error(`Failed to load example: ${res.status}`);
    const data = await res.json();
    selectedModelId = data.example?.entity_id ?? '';
    selectedModelLabel = data.example?.entity_id ?? '';
    titleValues = data.example?.title ?? {};
    descriptionValues = data.example?.description ?? {};
    await loadSchemaForEdit(id);
  }

  async function searchModels(query: string) {
    modelAbort?.abort();
    error = '';
    if (query.trim().length === 0) {
      modelResults = initialModelResults;
      return;
    }
    modelAbort = new AbortController();
    loadingModels = true;
    try {
      const res = await fetch(`/projects/${projectId}/models?search=${encodeURIComponent(query)}&per_page=20`, {
        signal: modelAbort.signal,
      });
      if (!res.ok) throw new Error(`Failed to search models: ${res.status}`);
      const data = await res.json();
      modelResults = data.models ?? [];
    } catch (e: any) {
      if (e?.name !== 'AbortError') {
        error = e?.message || 'Failed to search models';
      }
    } finally {
      loadingModels = false;
    }
  }

  async function loadInitialModels() {
    loadingModels = true;
    try {
      const res = await fetch(`/projects/${projectId}/models?per_page=20`);
      if (!res.ok) throw new Error(`Failed to load models: ${res.status}`);
      const data = await res.json();
      initialModelResults = data.models ?? [];
      modelResults = initialModelResults;
    } catch (e: any) {
      error = e?.message || 'Failed to load models';
    } finally {
      loadingModels = false;
    }
  }

  function handleModelInput(event: Event) {
    const target = event.target as HTMLInputElement;
    modelQuery = target.value;
    void searchModels(target.value);
  }

  async function chooseModel(model: ModelResult) {
    selectedModelId = model.id;
    selectedModelLabel = translated(model.ui_name, model.id);
    modelResults = [];
    modelQuery = selectedModelLabel;
    await loadSchemaForCreate(model.id);
  }

  async function loadSchemaForCreate(modelId: string) {
    loading = true;
    error = '';
    try {
      const res = await fetch(`${endpoints.form_schema_url}?mode=create&target_type=model&target_id=${encodeURIComponent(modelId)}`);
      if (!res.ok) throw new Error(`Failed to load form schema: ${res.status}`);
      const nextSchema: ExampleFormSchema = await res.json();
      schema = nextSchema;
      selectedModelLabel = translated(nextSchema.target?.name, selectedModelLabel || modelId);
      initValueState(nextSchema);
      void hydrateConceptLabels(nextSchema);
      initStructureState(nextSchema);
    } catch (e: any) {
      error = e?.message || 'Failed to load example schema';
    } finally {
      loading = false;
    }
  }

  async function loadSchemaForEdit(id: string) {
    loading = true;
    error = '';
    try {
      const res = await fetch(`${endpoints.form_schema_url}?mode=edit&example_id=${encodeURIComponent(id)}`);
      if (!res.ok) throw new Error(`Failed to load form schema: ${res.status}`);
      const nextSchema: ExampleFormSchema = await res.json();
      schema = nextSchema;
      // The create path does this after the model pick; edit mode needs it
      // too, else the header shows the model id instead of its name.
      selectedModelLabel = translated(nextSchema.target?.name, selectedModelLabel || nextSchema.target?.entity_id || '');
      initValueState(nextSchema);
      void hydrateConceptLabels(nextSchema);
      initStructureState(nextSchema);
    } catch (e: any) {
      error = e?.message || 'Failed to load example schema';
    } finally {
      loading = false;
    }
  }

  function initValueState(nextSchema: ExampleFormSchema) {
    const next: Record<string, OccurrenceState[]> = {};
    const nextConceptLabels: Record<string, string> = {};
    for (const section of nextSchema.sections ?? []) {
      for (const field of [...(section.direct_fields ?? []), ...(section.groups ?? []).flatMap((group) => group.fields ?? [])]) {
        const key = fieldKey(field);
        const occurrences = (field.occurrences?.length ? field.occurrences : [{ occurrence_index: 0, value: undefined }]).map((occ) => ({
          occurrence_index: occ.occurrence_index,
          value: payloadToInput(field, occ.value),
        }));
        if (field.value_kind === 'concept') {
          for (const occ of field.occurrences ?? []) {
            const uri = occ.value?.concept_uri;
            const label = occ.value?.concept_label;
            if (uri && label) nextConceptLabels[uri] = label;
          }
        }
        next[key] = occurrences;
      }
    }
    valuesByOverride = next;
    conceptLabelsByURI = nextConceptLabels;
  }

  async function hydrateConceptLabels(nextSchema: ExampleFormSchema) {
    const uris = new Set<string>();
    for (const section of nextSchema.sections ?? []) {
      for (const field of [...(section.direct_fields ?? []), ...(section.groups ?? []).flatMap((group) => group.fields ?? [])]) {
        if (field.value_kind !== 'concept') continue;
        for (const occ of field.occurrences ?? []) {
          const uri = occ.value?.concept_uri?.trim();
          if (!uri || (conceptLabelsByURI[uri] && conceptLabelsByURI[uri] !== uri)) continue;
          uris.add(uri);
        }
      }
    }
    if (uris.size === 0) return;
    const resolved: Record<string, string> = {};
    await Promise.all(
      [...uris].map(async (uri) => {
        try {
          const params = new URLSearchParams({ uri, lang: primaryLang });
          const res = await fetch(`/api/v2/vocabulary-entries/resolve?${params.toString()}`);
          if (!res.ok) return;
          const data = await res.json();
          const label = translated(data.label, uri);
          if (label && label !== uri) resolved[uri] = label;
        } catch {
          // Keep the URI fallback when a vocabulary entry cannot be resolved.
        }
      }),
    );
    if (Object.keys(resolved).length > 0) {
      conceptLabelsByURI = { ...conceptLabelsByURI, ...resolved };
    }
  }

  function initStructureState(nextSchema: ExampleFormSchema) {
    const nextSections: Record<string, boolean> = {};
    const nextGroups: Record<string, boolean> = {};
    for (const [index, section] of (nextSchema.sections ?? []).entries()) {
      const summary = sectionSummary(section);
      nextSections[section.id] = index === 0 || summary.required_total > 0 || summary.errors > 0 || summary.warnings > 0;
      for (const group of section.groups ?? []) {
        const groupCounts = groupSummary(group);
        nextGroups[group.id] = groupCounts.required_total > 0 || groupCounts.errors > 0 || groupCounts.warnings > 0;
      }
    }
    expandedSections = nextSections;
    expandedGroups = nextGroups;
  }

  function fieldKey(field: ExampleFormField): string {
    return String(field.override_id);
  }

  function translated(value: any, fallback = ''): string {
    if (!value) return fallback;
    if (typeof value === 'string') return value;
    return value?.[primaryLang] || value?.en || Object.values(value)[0] || fallback;
  }

  function requiredMinimum(field: ExampleFormField): number {
    return Math.max(field.required ? 1 : 0, field.min_occurs ?? 0);
  }

  function fieldFilledCount(field: ExampleFormField): number {
    return fieldOccurrences(field).filter((occ) => !occurrenceIsBlank(field, occ)).length;
  }

  function fieldHasAnyValue(field: ExampleFormField): boolean {
    return fieldFilledCount(field) > 0;
  }

  function issueCounts(issues: ExampleIssue[] | undefined): { errors: number; warnings: number } {
    let errors = 0;
    let warnings = 0;
    for (const issue of issues ?? []) {
      if (issue.severity === 'warning') {
        warnings += 1;
      } else {
        errors += 1;
      }
    }
    return { errors, warnings };
  }

  function fieldIssueCounts(field: ExampleFormField): { errors: number; warnings: number } {
    const counts = issueCounts(field.issues);
    for (const occurrence of field.occurrences ?? []) {
      const next = issueCounts(occurrence.issues);
      counts.errors += next.errors;
      counts.warnings += next.warnings;
    }
    return counts;
  }

  function fieldHasIssues(field: ExampleFormField): boolean {
    const counts = fieldIssueCounts(field);
    return counts.errors > 0 || counts.warnings > 0;
  }

  function renderableField(field: ExampleFormField): boolean {
    return !field.hidden || fieldHasAnyValue(field) || fieldHasIssues(field);
  }

  function fieldMatchesFilter(field: ExampleFormField): boolean {
    if (!renderableField(field)) return false;
    if (activeFilter === 'required') return requiredMinimum(field) > 0;
    if (activeFilter === 'issues') return fieldHasIssues(field);
    return true;
  }

  function fieldHasOverviewContent(field: ExampleFormField): boolean {
    return fieldHasAnyValue(field);
  }

  function fieldMatchesSearch(field: ExampleFormField): boolean {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return true;
    const haystack = [
      translated(field.label, field.field_id),
      field.field_semantic_id,
      translated(field.help),
      field.expected_value_type || '',
      ...fieldContextNotes(field),
    ]
      .join(' ')
      .toLowerCase();
    return haystack.includes(q);
  }

  function fieldVisibleInEdit(field: ExampleFormField): boolean {
    return fieldMatchesFilter(field) && fieldMatchesSearch(field);
  }

  function allSectionFields(section: ExampleFormSection): ExampleFormField[] {
    return [...(section.direct_fields ?? []), ...(section.groups ?? []).flatMap((group) => group.fields ?? [])];
  }

  function searchMatchOrder(): number[] {
    if (workspaceMode !== 'edit' || !searchQuery.trim()) return [];
    const order: number[] = [];
    for (const section of schema?.sections ?? []) {
      for (const field of section.direct_fields ?? []) {
        if (fieldVisibleInEdit(field)) order.push(field.override_id);
      }
      for (const group of section.groups ?? []) {
        for (const field of group.fields ?? []) {
          if (fieldVisibleInEdit(field)) order.push(field.override_id);
        }
      }
    }
    return order;
  }

  function activeMatchID(): number | null {
    const order = searchMatchOrder();
    if (order.length === 0) return null;
    const safeIndex = Math.min(searchIndex, order.length - 1);
    return order[safeIndex] ?? null;
  }

  function sectionSummary(section: ExampleFormSection): SummaryCounts {
    const counts: SummaryCounts = {
      required_total: 0,
      required_filled: 0,
      required_remaining: 0,
      filled_fields: 0,
      errors: 0,
      warnings: 0,
      has_value: false,
    };
    for (const field of allSectionFields(section)) {
      if (!renderableField(field)) continue;
      const requiredCount = requiredMinimum(field);
      const filledCount = fieldFilledCount(field);
      const fieldIssues = fieldIssueCounts(field);
      if (requiredCount > 0) {
        counts.required_total += 1;
        if (filledCount >= requiredCount) {
          counts.required_filled += 1;
        } else {
          counts.required_remaining += 1;
        }
      }
      if (filledCount > 0) {
        counts.filled_fields += 1;
        counts.has_value = true;
      }
      counts.errors += fieldIssues.errors;
      counts.warnings += fieldIssues.warnings;
    }
    return counts;
  }

  function groupSummary(group: ExampleFormGroup): SummaryCounts {
    const counts: SummaryCounts = {
      required_total: 0,
      required_filled: 0,
      required_remaining: 0,
      filled_fields: 0,
      errors: 0,
      warnings: 0,
      has_value: false,
    };
    for (const field of group.fields ?? []) {
      if (!renderableField(field)) continue;
      const requiredCount = requiredMinimum(field);
      const filledCount = fieldFilledCount(field);
      const fieldIssues = fieldIssueCounts(field);
      if (requiredCount > 0) {
        counts.required_total += 1;
        if (filledCount >= requiredCount) {
          counts.required_filled += 1;
        } else {
          counts.required_remaining += 1;
        }
      }
      if (filledCount > 0) {
        counts.filled_fields += 1;
        counts.has_value = true;
      }
      counts.errors += fieldIssues.errors;
      counts.warnings += fieldIssues.warnings;
    }
    return counts;
  }

  function overallSummary(): SummaryCounts {
    const counts: SummaryCounts = {
      required_total: 0,
      required_filled: 0,
      required_remaining: 0,
      filled_fields: 0,
      errors: 0,
      warnings: 0,
      has_value: false,
    };
    for (const section of schema?.sections ?? []) {
      const next = sectionSummary(section);
      counts.required_total += next.required_total;
      counts.required_filled += next.required_filled;
      counts.required_remaining += next.required_remaining;
      counts.filled_fields += next.filled_fields;
      counts.errors += next.errors;
      counts.warnings += next.warnings;
      counts.has_value = counts.has_value || next.has_value;
    }
    const schemaIssueCounts = issueCounts(schema?.issues);
    counts.errors += schemaIssueCounts.errors;
    counts.warnings += schemaIssueCounts.warnings;
    return counts;
  }

  function sectionVisible(section: ExampleFormSection): boolean {
    return allSectionFields(section).some((field) => fieldVisibleInEdit(field));
  }

  function groupVisible(group: ExampleFormGroup): boolean {
    return (group.fields ?? []).some((field) => fieldVisibleInEdit(field));
  }

  function sectionVisibleInOverview(section: ExampleFormSection): boolean {
    return allSectionFields(section).some((field) => fieldHasOverviewContent(field));
  }

  function groupVisibleInOverview(group: ExampleFormGroup): boolean {
    return (group.fields ?? []).some((field) => fieldHasOverviewContent(field));
  }

  function toggleSection(sectionID: string) {
    expandedSections = { ...expandedSections, [sectionID]: !expandedSections[sectionID] };
  }

  function toggleGroup(groupID: string) {
    expandedGroups = { ...expandedGroups, [groupID]: !expandedGroups[groupID] };
  }

  function setFilter(mode: FilterMode) {
    activeFilter = mode;
    if (mode === 'required') {
      openRequiredSections();
    } else if (mode === 'issues') {
      openIssueSections();
    }
  }

  function expandAllStructure() {
    const nextSections: Record<string, boolean> = {};
    const nextGroups: Record<string, boolean> = {};
    for (const section of schema?.sections ?? []) {
      nextSections[section.id] = true;
      for (const group of section.groups ?? []) {
        nextGroups[group.id] = true;
      }
    }
    expandedSections = nextSections;
    expandedGroups = nextGroups;
  }

  function collapseOptionalStructure() {
    const nextSections: Record<string, boolean> = {};
    const nextGroups: Record<string, boolean> = {};
    for (const [index, section] of (schema?.sections ?? []).entries()) {
      const summary = sectionSummary(section);
      nextSections[section.id] = index === 0 || summary.required_total > 0 || summary.errors > 0 || summary.warnings > 0;
      for (const group of section.groups ?? []) {
        const groupCounts = groupSummary(group);
        nextGroups[group.id] = groupCounts.required_total > 0 || groupCounts.errors > 0 || groupCounts.warnings > 0;
      }
    }
    expandedSections = nextSections;
    expandedGroups = nextGroups;
  }

  function openRequiredSections() {
    const nextSections = { ...expandedSections };
    const nextGroups = { ...expandedGroups };
    for (const section of schema?.sections ?? []) {
      if (allSectionFields(section).some((field) => renderableField(field) && requiredMinimum(field) > 0)) {
        nextSections[section.id] = true;
      }
      for (const group of section.groups ?? []) {
        if ((group.fields ?? []).some((field) => renderableField(field) && requiredMinimum(field) > 0)) {
          nextGroups[group.id] = true;
        }
      }
    }
    expandedSections = nextSections;
    expandedGroups = nextGroups;
  }

  function openIssueSections() {
    const nextSections = { ...expandedSections };
    const nextGroups = { ...expandedGroups };
    for (const section of schema?.sections ?? []) {
      if (allSectionFields(section).some((field) => fieldHasIssues(field))) {
        nextSections[section.id] = true;
      }
      for (const group of section.groups ?? []) {
        if ((group.fields ?? []).some((field) => fieldHasIssues(field))) {
          nextGroups[group.id] = true;
        }
      }
    }
    expandedSections = nextSections;
    expandedGroups = nextGroups;
  }

  function completedSectionsCount(): number {
    let completed = 0;
    for (const section of schema?.sections ?? []) {
      const summary = sectionSummary(section);
      if (summary.required_remaining === 0 && summary.errors === 0 && summary.has_value) {
        completed += 1;
      }
    }
    return completed;
  }

  function currentStatus(summary: SummaryCounts): 'draft' | 'valid' | 'has_issues' {
    if (summary.errors > 0 || summary.warnings > 0) return 'has_issues';
    if (summary.required_remaining === 0 && summary.has_value) return 'valid';
    return 'draft';
  }

  function statusClasses(status: 'draft' | 'valid' | 'has_issues'): string {
    if (status === 'valid') return 'bg-emerald-100 text-emerald-700 ring-emerald-200';
    if (status === 'has_issues') return 'bg-amber-100 text-amber-800 ring-amber-200';
    return 'bg-slate-100 text-slate-700 ring-slate-200';
  }

  function jumpToSection(sectionID: string) {
    document.getElementById(`example-section-${sectionID}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  function setWorkspaceMode(mode: WorkspaceMode) {
    workspaceMode = mode;
  }

  function handleSearchInput(event: Event) {
    const target = event.target as HTMLInputElement;
    searchQuery = target.value;
    searchIndex = 0;
    if (target.value.trim()) {
      openSearchMatches();
      queueMicrotask(scrollToActiveMatch);
    }
  }

  function handleSearchKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      event.preventDefault();
      if (event.shiftKey) {
        prevSearchMatch();
      } else {
        nextSearchMatch();
      }
      return;
    }
    if (event.key === 'Escape') {
      event.preventDefault();
      clearSearch();
    }
  }

  function clearSearch() {
    searchQuery = '';
    searchIndex = 0;
  }

  function nextSearchMatch() {
    const order = searchMatchOrder();
    if (order.length === 0) return;
    searchIndex = (searchIndex + 1) % order.length;
    scrollToActiveMatch();
  }

  function prevSearchMatch() {
    const order = searchMatchOrder();
    if (order.length === 0) return;
    searchIndex = (searchIndex - 1 + order.length) % order.length;
    scrollToActiveMatch();
  }

  function openSearchMatches() {
    const q = searchQuery.trim();
    if (!q) return;
    const nextSections = { ...expandedSections };
    const nextGroups = { ...expandedGroups };
    for (const section of schema?.sections ?? []) {
      if (allSectionFields(section).some((field) => fieldVisibleInEdit(field))) {
        nextSections[section.id] = true;
      }
      for (const group of section.groups ?? []) {
        if ((group.fields ?? []).some((field) => fieldVisibleInEdit(field))) {
          nextGroups[group.id] = true;
        }
      }
    }
    expandedSections = nextSections;
    expandedGroups = nextGroups;
  }

  function scrollToActiveMatch() {
    const activeID = activeMatchID();
    if (!activeID) return;
    document.getElementById(`example-field-${activeID}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }

  function payloadToInput(field: ExampleFormField, value: Record<string, any> | undefined): string {
    if (!value) {
      if (field.value_kind === 'concept' && field.set_value) return field.set_value;
      return '';
    }
    switch (field.value_kind) {
      case 'integer':
        return value.number_value != null ? String(value.number_value) : '';
      case 'date':
        return value.date_value || '';
      case 'uri':
        return value.uri_value || '';
      case 'concept':
        return value.concept_uri || field.set_value || '';
      case 'example_ref':
        return value.example_id || '';
      default:
        return value.string_value || '';
    }
  }

  function inputToPayload(field: ExampleFormField, raw: string): Record<string, any> {
    switch (field.value_kind) {
      case 'integer': {
        const parsed = Number(raw);
        return { kind: field.value_kind, number_value: Number.isFinite(parsed) ? parsed : null };
      }
      case 'date':
        return { kind: field.value_kind, date_value: raw };
      case 'uri':
        return { kind: field.value_kind, uri_value: raw };
      case 'concept':
        return { kind: field.value_kind, concept_uri: raw, concept_label: conceptLabelsByURI[raw] || raw };
      case 'example_ref':
        return { kind: field.value_kind, example_id: raw };
      default:
        return { kind: field.value_kind, string_value: raw };
    }
  }

  function isBlank(field: ExampleFormField, raw: string): boolean {
    if (field.value_kind === 'concept' && field.set_value) return raw.trim() === '';
    return raw.trim() === '';
  }

  function soleResourceModel(field: ExampleFormField): string {
    const models = field.resource_models ?? [];
    return models.length === 1 ? models[0].id : '';
  }

  function occurrenceIsStub(field: ExampleFormField, occurrence: OccurrenceState): boolean {
    return (
      field.value_kind === 'example_ref' &&
      (field.expected_value_type || '').trim() === 'Model' &&
      (field.resource_models ?? []).length > 0 &&
      !occurrence.value.trim() &&
      Boolean(occurrence.stub_label?.trim())
    );
  }

  function occurrenceIsBlank(field: ExampleFormField, occurrence: OccurrenceState): boolean {
    if (occurrenceIsStub(field, occurrence)) return false;
    return isBlank(field, occurrence.value);
  }

  function refPayload(field: ExampleFormField, occurrence: OccurrenceState): Record<string, any> {
    if (occurrenceIsStub(field, occurrence)) {
      const target = occurrence.stub_model || soleResourceModel(field);
      return {
        kind: 'example_ref',
        target_label: occurrence.stub_label!.trim(),
        ...(target ? { target_entity_id: target } : {}),
      };
    }
    return { kind: 'example_ref', example_id: occurrence.value };
  }

  function occurrenceFieldDef(field: ExampleFormField, occurrenceIndex: number): FieldDef {
    const fixedConcept = field.value_kind === 'concept' && Boolean(field.set_value);
    return {
      name: `${field.override_id}:${occurrenceIndex}`,
      widget: renderWidget(field),
      required: Boolean(field.required && occurrenceIndex === 0),
      readonly: fixedConcept,
      label: field.label,
      help: field.help,
      value: '',
      options: field.value_kind === 'example_ref' ? linkedExampleOptions(field) : undefined,
    };
  }

  function renderWidget(field: ExampleFormField): string {
    if (field.value_kind !== 'concept' && field.widget) {
      return field.widget;
    }
    switch (field.value_kind) {
      case 'integer':
        return 'number';
      case 'date':
        return 'date';
      case 'uri':
        return 'url';
      case 'example_ref':
        return 'search-select';
      default:
        return 'text';
    }
  }

  function linkedExampleOptions(field: ExampleFormField): SelectOption[] {
    const allowAllModels = (field.expected_value_type || '').trim() === 'Model' && (!field.resource_models || field.resource_models.length === 0);
    const allowedModelIDs = new Set((field.resource_models ?? []).map((model) => model.id));
    return availableExamples
      .filter((example) => example.id !== currentExampleId)
      .filter((example) => example.entity_type === 'model')
      .filter((example) => allowAllModels || allowedModelIDs.has(example.entity_id))
      .sort((a, b) => translated(a.title, a.id).localeCompare(translated(b.title, b.id)))
      .map((example) => ({
        value: example.id,
        label: { en: `${translated(example.title, example.id)} (${example.entity_id})` },
        description: { en: example.status ? `Example status: ${example.status.replace('_', ' ')}` : 'Example' },
        semantic_id: example.entity_id,
        status: example.status,
      }));
  }

  function eligibleLinkedExamples(field: ExampleFormField): ExampleOption[] {
    const allowAllModels = (field.expected_value_type || '').trim() === 'Model' && (!field.resource_models || field.resource_models.length === 0);
    const allowedModelIDs = new Set((field.resource_models ?? []).map((model) => model.id));
    return availableExamples
      .filter((example) => example.id !== currentExampleId)
      .filter((example) => example.entity_type === 'model')
      .filter((example) => allowAllModels || allowedModelIDs.has(example.entity_id))
      .sort((a, b) => translated(a.title, a.id).localeCompare(translated(b.title, b.id)));
  }

  function linkedExampleByID(id: string): ExampleOption | null {
    return availableExamples.find((example) => example.id === id) ?? null;
  }

  /** Page URL of the example an occurrence links to, or '' for stubs, blanks and non-ref fields. */
  function occurrenceLinkURL(field: ExampleFormField, occurrence: OccurrenceState): string {
    if (field.value_kind !== 'example_ref' || occurrenceIsStub(field, occurrence)) return '';
    const id = occurrence.value?.trim() ?? '';
    return id ? examplePageURL(id) : '';
  }

  function examplePageURL(id: string): string {
    return endpoints.page_url_template ? endpoints.page_url_template.replace('{id}', encodeURIComponent(id)) : '';
  }

  function linkedStatusClasses(status: string | undefined): string {
    switch (status) {
      case 'valid':
        return 'bg-emerald-100 text-emerald-700';
      case 'has_issues':
        return 'bg-amber-100 text-amber-800';
      default:
        return 'bg-slate-100 text-slate-700';
    }
  }

  function fieldOccurrences(field: ExampleFormField): OccurrenceState[] {
    return valuesByOverride[fieldKey(field)] ?? [{ occurrence_index: 0, value: '' }];
  }

  function addOccurrence(field: ExampleFormField) {
    const key = fieldKey(field);
    const current = [...fieldOccurrences(field)];
    const nextIndex = current.reduce((max, occ) => Math.max(max, occ.occurrence_index), -1) + 1;
    current.push({
      occurrence_index: nextIndex,
      value: field.value_kind === 'concept' && field.set_value ? field.set_value : '',
    });
    valuesByOverride = { ...valuesByOverride, [key]: current };
  }

  function removeOccurrence(field: ExampleFormField, occurrenceIndex: number) {
    const key = fieldKey(field);
    const current = fieldOccurrences(field).filter((occ) => occ.occurrence_index !== occurrenceIndex);
    valuesByOverride = {
      ...valuesByOverride,
      [key]: current.length > 0 ? current : [{ occurrence_index: 0, value: '' }],
    };
  }

  function errorMessages(issues: ExampleIssue[] | undefined): string[] {
    return (issues ?? []).filter((issue) => issue.severity === 'error').map((issue) => translated(issue.message, issue.code));
  }

  function warningMessages(issues: ExampleIssue[] | undefined): string[] {
    return (issues ?? []).filter((issue) => issue.severity === 'warning').map((issue) => translated(issue.message, issue.code));
  }

  function occurrenceIssues(field: ExampleFormField, occurrenceIndex: number): string[] {
    const occurrence = field.occurrences?.find((occ) => occ.occurrence_index === occurrenceIndex);
    return errorMessages(occurrence?.issues);
  }

  function occurrenceWarnings(field: ExampleFormField, occurrenceIndex: number): string[] {
    const occurrence = field.occurrences?.find((occ) => occ.occurrence_index === occurrenceIndex);
    return warningMessages(occurrence?.issues);
  }

  function occurrenceDisplayValue(field: ExampleFormField, occurrence: OccurrenceState): string {
    if (occurrenceIsStub(field, occurrence)) {
      return `${occurrence.stub_label!.trim()} (new draft)`;
    }
    const raw = occurrence.value?.trim() ?? '';
    if (!raw) return '';
    if (field.value_kind === 'example_ref') {
      const linked = availableExamples.find((example) => example.id === raw);
      if (linked) {
        const linkedTitle = translated(linked.title, linked.id);
        return `${linkedTitle} (${linked.entity_id})`;
      }
    }
    if (field.value_kind === 'concept') {
      return conceptLabelsByURI[raw] || raw;
    }
    return raw;
  }

  function occurrenceTechnicalValue(field: ExampleFormField, occurrence: OccurrenceState): string {
    const raw = occurrence.value?.trim() ?? '';
    if (!raw) return '';
    if (field.value_kind === 'concept' && conceptLabelsByURI[raw] && conceptLabelsByURI[raw] !== raw) {
      return raw;
    }
    return '';
  }

  function rememberConceptLabel(selection: { uri: string; label: string }) {
    if (!selection.uri || !selection.label) return;
    conceptLabelsByURI = { ...conceptLabelsByURI, [selection.uri]: selection.label };
  }

  function fieldContextNotes(field: ExampleFormField): string[] {
    const notes: string[] = [];
    if (field.value_kind === 'concept' && field.set_value) {
      notes.push(`Fixed concept: ${field.set_value}`);
    }
    if (field.value_kind === 'concept' && field.concept_lists?.length) {
      const labels = field.concept_lists.map((list) => list.semantic_id || list.id);
      notes.push(`Allowed concepts: ${labels.join(', ')}`);
    }
    if (field.value_kind === 'example_ref') {
      const labels = (field.resource_models ?? []).map((model) => model.semantic_id || model.id);
      if (labels.length > 0) {
        notes.push(`Allowed models: ${labels.join(', ')}`);
      } else if ((field.expected_value_type || '').trim() === 'Model') {
        notes.push('Links to another example');
      }
    }
    if (field.repeatable && field.max_occurs != null) {
      notes.push(`Up to ${field.max_occurs} value(s)`);
    }
    if (field.min_occurs && field.min_occurs > 1) {
      notes.push(`At least ${field.min_occurs} value(s) required`);
    }
    return notes;
  }

  function serializeValues(): Array<Record<string, any>> {
    if (!schema) return [];
    const out: Array<Record<string, any>> = [];
    for (const section of schema.sections ?? []) {
      const fields = [...(section.direct_fields ?? []), ...(section.groups ?? []).flatMap((group) => group.fields ?? [])];
      for (const field of fields) {
        for (const occurrence of fieldOccurrences(field)) {
          if (occurrenceIsBlank(field, occurrence)) continue;
          out.push({
            override_id: field.override_id,
            field_id: field.field_id,
            occurrence_index: occurrence.occurrence_index,
            value_kind: field.value_kind,
            value_payload:
              field.value_kind === 'example_ref' ? refPayload(field, occurrence) : inputToPayload(field, occurrence.value),
          });
        }
      }
    }
    return out;
  }

  function firstStubWithoutTarget(): string | null {
    if (!schema) return null;
    for (const section of schema.sections ?? []) {
      const fields = [...(section.direct_fields ?? []), ...(section.groups ?? []).flatMap((group) => group.fields ?? [])];
      for (const field of fields) {
        for (const occurrence of fieldOccurrences(field)) {
          if (!occurrenceIsStub(field, occurrence)) continue;
          const models = field.resource_models ?? [];
          const missingTarget = models.length > 1 && !occurrence.stub_model;
          if (missingTarget) {
            return `Choose a model for the new draft "${occurrence.stub_label!.trim()}".`;
          }
        }
      }
    }
    return null;
  }

  async function submit() {
    if (!schema?.endpoint) return;
    if (!selectedModelId) {
      error = 'Select a model first.';
      return;
    }
    const stubIssue = firstStubWithoutTarget();
    if (stubIssue) {
      error = stubIssue;
      return;
    }
    saving = true;
    error = '';
    try {
      const payload = {
        entity_type: 'model',
        entity_id: selectedModelId,
        title: titleValues,
        description: descriptionValues,
        values: serializeValues(),
        lang: primaryLang,
      };
      const res = await fetch(schema.endpoint.url, {
        method: schema.endpoint.method,
        headers: {
          'Content-Type': 'application/json',
          'X-Requested-With': 'XMLHttpRequest',
        },
        body: JSON.stringify(payload),
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || `Save failed (${res.status})`);
      }
      const data = await res.json();
      const issues = data.validation?.issues ?? [];
      if (issues.length > 0) {
        addToast('warning', 'Example saved with validation issues');
        currentExampleId = data.example?.id || currentExampleId;
        currentMode = 'edit';
        titleValues = data.example?.title ?? titleValues;
        descriptionValues = data.example?.description ?? descriptionValues;
        await loadAvailableExamples();
        await loadSchemaForEdit(currentExampleId);
        return;
      }
      addToast('success', translated(schema.ui.success_message, 'Example saved'));
      await loadAvailableExamples();
      onsuccess?.();
    } catch (e: any) {
      error = e?.message || 'Failed to save example';
    } finally {
      saving = false;
    }
  }

  const titleField: FieldDef = {
    name: 'title',
    widget: 'multilingual-text',
    required: false,
    label: { en: 'Title' },
    help: { en: 'Optional label for this example.' },
    value: {},
  };

  const descriptionField: FieldDef = {
    name: 'description',
    widget: 'multilingual-textarea',
    required: false,
    label: { en: 'Description' },
    help: { en: 'Optional note explaining what this example demonstrates.' },
    value: {},
  };
</script>

{#if loading}
  <div class="space-y-4 animate-pulse">
    <div class="h-10 w-1/3 rounded bg-gray-200"></div>
    <div class="h-28 rounded bg-gray-100"></div>
    <div class="h-40 rounded bg-gray-100"></div>
  </div>
{:else}
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h3 class="text-lg font-semibold text-gray-900">
          {currentMode === 'create' ? `New ${selectedModelLabel || ''} example`.replace('  ', ' ') : `Editing ${selectedModelLabel || 'example'}`}
          {#if currentMode !== 'create'}
            <span class="ml-2 align-middle font-mono text-xs font-normal text-gray-400">{currentExampleId}</span>
          {/if}
        </h3>
        <p class="mt-1 text-sm text-gray-500">
          Fill in a worked example to verify that the model can actually be used as intended.
        </p>
      </div>
      <button
        type="button"
        class="inline-flex items-center rounded-md border border-gray-300 px-3 py-2 text-sm text-gray-700 hover:bg-gray-50"
        onclick={() => oncancel?.()}
      >
        Back to list
      </button>
    </div>

    {#if error}
      <div class="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
    {/if}

    {#if schema?.issues?.length}
      <div class="rounded-md border border-amber-200 bg-amber-50 px-4 py-3">
        <p class="text-sm font-medium text-amber-900">Validation issues</p>
        <ul class="mt-2 list-disc space-y-1 pl-5 text-sm text-amber-800">
          {#each schema.issues as issue, idx (`${issue.code}-${idx}`)}
            <li>{translated(issue.message, issue.code)}</li>
          {/each}
        </ul>
      </div>
    {/if}

    {#if currentMode === 'create' && !selectedModelId}
      <section class="rounded-xl border border-gray-200 bg-white p-5 shadow-sm">
        <div class="space-y-3">
          <div>
            <h4 class="text-sm font-semibold uppercase tracking-wide text-gray-500">Target Model</h4>
            <p class="mt-1 text-sm text-gray-600">Choose the model this example should describe.</p>
          </div>
          <input
            type="text"
            bind:value={modelQuery}
            placeholder="Search models..."
            class="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-pletka-primary focus:outline-none focus:ring-pletka-primary"
            oninput={handleModelInput}
          />
          {#if loadingModels}
            <p class="text-sm text-gray-500">Searching…</p>
          {:else if modelResults.length > 0}
            <div class="divide-y divide-gray-100 overflow-hidden rounded-lg border border-gray-200">
              {#each modelResults as model (model.id)}
                <button
                  type="button"
                  class="block w-full px-4 py-3 text-left hover:bg-gray-50"
                  onclick={() => chooseModel(model)}
                >
                  <div class="flex items-center justify-between gap-3">
                    <span class="font-medium text-gray-900">{translated(model.ui_name, model.id)}</span>
                    <span class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-500">{model.id}</span>
                  </div>
                  {#if translated(model.description)}
                    <p class="mt-1 text-sm text-gray-500">{translated(model.description)}</p>
                  {/if}
                </button>
              {/each}
            </div>
          {:else if modelQuery.trim().length > 0}
            <p class="text-sm text-gray-500">No matching models.</p>
          {/if}
        </div>
      </section>
    {/if}

    {#if schema}
      {@const summary = overallSummary()}
      {@const status = currentStatus(summary)}
      {@const matchOrder = searchMatchOrder()}
      {@const activeMatch = activeMatchID()}

      <section class="sticky top-0 z-20 rounded-xl border border-gray-200 bg-white/95 p-4 shadow-sm backdrop-blur">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div class="space-y-3">
            <div class="flex flex-wrap items-center gap-2">
              <h4 class="text-base font-semibold text-gray-900">{translated(titleValues, selectedModelLabel || 'Example')}</h4>
              <span class={`inline-flex items-center rounded-full px-2.5 py-1 text-xs font-semibold ring-1 ring-inset ${statusClasses(status)}`}>
                {status === 'has_issues' ? 'Has issues' : status === 'valid' ? 'Valid' : 'Draft'}
              </span>
              {#if selectedModelId}
                <span class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-500">{selectedModelId}</span>
              {/if}
            </div>
            <div class="flex flex-wrap gap-2 text-sm text-gray-600">
              <span class="rounded-full bg-gray-100 px-3 py-1">{summary.required_remaining} required remaining</span>
              <span class="rounded-full bg-emerald-50 px-3 py-1 text-emerald-700">{completedSectionsCount()} sections complete</span>
              <span class="rounded-full bg-rose-50 px-3 py-1 text-rose-700">{summary.errors} errors</span>
              <span class="rounded-full bg-amber-50 px-3 py-1 text-amber-700">{summary.warnings} warnings</span>
            </div>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <div class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-1">
              <button type="button" class={`rounded-md px-3 py-1.5 text-sm ${workspaceMode === 'overview' ? 'bg-white shadow-sm text-gray-900' : 'text-gray-600 hover:text-gray-900'}`} onclick={() => setWorkspaceMode('overview')}>Overview</button>
              <button type="button" class={`rounded-md px-3 py-1.5 text-sm ${workspaceMode === 'edit' ? 'bg-white shadow-sm text-gray-900' : 'text-gray-600 hover:text-gray-900'}`} onclick={() => setWorkspaceMode('edit')}>Edit</button>
            </div>
            {#if workspaceMode === 'edit'}
              <div class="flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-2 py-1.5">
                <input
                  type="text"
                  value={searchQuery}
                  placeholder="Search fields..."
                  class="w-48 border-0 bg-transparent px-2 py-1 text-sm text-gray-700 placeholder:text-gray-400 focus:outline-none focus:ring-0"
                  oninput={handleSearchInput}
                  onkeydown={handleSearchKeydown}
                />
                {#if searchQuery.trim()}
                  <span class="text-xs text-gray-500">
                    {matchOrder.length === 0 ? '0 matches' : `${Math.min(searchIndex + 1, matchOrder.length)} of ${matchOrder.length}`}
                  </span>
                  <button type="button" class="rounded px-1.5 py-1 text-xs text-gray-500 hover:bg-gray-100 hover:text-gray-700" onclick={prevSearchMatch} disabled={matchOrder.length === 0}>Prev</button>
                  <button type="button" class="rounded px-1.5 py-1 text-xs text-gray-500 hover:bg-gray-100 hover:text-gray-700" onclick={nextSearchMatch} disabled={matchOrder.length === 0}>Next</button>
                  <button type="button" class="rounded px-1.5 py-1 text-xs text-gray-500 hover:bg-gray-100 hover:text-gray-700" onclick={clearSearch}>Clear</button>
                {/if}
              </div>
              <div class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-1">
                <button type="button" class={`rounded-md px-3 py-1.5 text-sm ${activeFilter === 'all' ? 'bg-white shadow-sm text-gray-900' : 'text-gray-600 hover:text-gray-900'}`} onclick={() => setFilter('all')}>All</button>
                <button type="button" class={`rounded-md px-3 py-1.5 text-sm ${activeFilter === 'required' ? 'bg-white shadow-sm text-gray-900' : 'text-gray-600 hover:text-gray-900'}`} onclick={() => setFilter('required')}>Required Only</button>
                <button type="button" class={`rounded-md px-3 py-1.5 text-sm ${activeFilter === 'issues' ? 'bg-white shadow-sm text-gray-900' : 'text-gray-600 hover:text-gray-900'}`} onclick={() => setFilter('issues')}>Issues Only</button>
              </div>
            {/if}
            {#if workspaceMode === 'edit'}
              <button type="button" class="rounded-md border border-gray-300 px-3 py-2 text-sm text-gray-700 hover:bg-gray-50" onclick={expandAllStructure}>Expand all</button>
              <button type="button" class="rounded-md border border-gray-300 px-3 py-2 text-sm text-gray-700 hover:bg-gray-50" onclick={collapseOptionalStructure}>Collapse optional</button>
            {/if}
            <button
              type="button"
              class="rounded-md bg-pletka-primary px-4 py-2 text-sm font-medium text-white hover:bg-pletka-secondary disabled:opacity-60"
              disabled={saving}
              onclick={submit}
            >
              {saving ? 'Saving…' : translated(schema.ui.submit_label, 'Save Example')}
            </button>
          </div>
        </div>
      </section>

      {#if workspaceMode === 'edit'}
        <section class="rounded-xl border border-gray-200 bg-white p-5 shadow-sm">
          <div class="grid gap-4 md:grid-cols-2">
            <WidgetDispatcher field={titleField} bind:value={titleValues} formValues={{}} {lang} languages={schema.ui.languages} errors={[]} />
            <WidgetDispatcher field={descriptionField} bind:value={descriptionValues} formValues={{}} {lang} languages={schema.ui.languages} errors={[]} />
          </div>
        </section>
      {:else if translated(descriptionValues)}
        <section class="rounded-xl border border-gray-200 bg-white p-5 shadow-sm">
          <h4 class="text-sm font-semibold uppercase tracking-wide text-gray-500">Description</h4>
          <p class="mt-2 whitespace-pre-line text-sm leading-6 text-gray-700">{translated(descriptionValues)}</p>
        </section>
      {/if}

      <div class="grid gap-6 xl:grid-cols-[250px_minmax(0,1fr)]">
        <aside class="hidden xl:block">
          <div class="sticky top-40 rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
            <h4 class="text-sm font-semibold uppercase tracking-wide text-gray-500">Structure</h4>
            <div class="mt-4 space-y-2">
              {#each schema.sections as section (section.id)}
                {#if workspaceMode === 'edit' ? sectionVisible(section) : sectionVisibleInOverview(section)}
                  {@const sectionCounts = sectionSummary(section)}
                  <button
                    type="button"
                    class="block w-full rounded-lg border border-gray-200 px-3 py-3 text-left hover:border-gray-300 hover:bg-gray-50"
                    onclick={() => jumpToSection(section.id)}
                  >
                    <div class="flex items-start justify-between gap-2">
                      <span class="text-sm font-medium text-gray-900">{translated(section.label, section.id)}</span>
                      {#if sectionCounts.errors > 0}
                        <span class="rounded-full bg-rose-50 px-2 py-0.5 text-xs font-medium text-rose-700">{sectionCounts.errors}</span>
                      {:else if sectionCounts.warnings > 0}
                        <span class="rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700">{sectionCounts.warnings}</span>
                      {/if}
                    </div>
                    <p class="mt-2 text-xs text-gray-500">
                      {sectionCounts.required_remaining} required remaining · {sectionCounts.filled_fields} fields filled
                    </p>
                  </button>
                {/if}
              {/each}
            </div>
          </div>
        </aside>

        <div class="space-y-5">
          {#each schema.sections as section (section.id)}
            {#if workspaceMode === 'edit' ? sectionVisible(section) : sectionVisibleInOverview(section)}
              {@const sectionCounts = sectionSummary(section)}
              <section id={`example-section-${section.id}`} class="rounded-xl border border-gray-200 bg-white shadow-sm">
                <button
                  type="button"
                  class="flex w-full items-start justify-between gap-4 rounded-xl px-5 py-4 text-left hover:bg-gray-50/80"
                  onclick={() => toggleSection(section.id)}
                >
                  <div>
                    <div class="flex flex-wrap items-center gap-2">
                      <h4 class="text-base font-semibold text-gray-900">{translated(section.label, section.id)}</h4>
                      {#if sectionCounts.required_total > 0}
                        <span class="rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-700">
                          {sectionCounts.required_filled}/{sectionCounts.required_total} required
                        </span>
                      {/if}
                      {#if sectionCounts.errors > 0}
                        <span class="rounded-full bg-rose-50 px-2 py-0.5 text-xs font-medium text-rose-700">{sectionCounts.errors} errors</span>
                      {/if}
                      {#if sectionCounts.warnings > 0}
                        <span class="rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700">{sectionCounts.warnings} warnings</span>
                      {/if}
                    </div>
                    <p class="mt-2 text-sm text-gray-500">
                      {sectionCounts.filled_fields} fields filled
                      {#if sectionCounts.required_remaining > 0}
                        · {sectionCounts.required_remaining} required still missing
                      {/if}
                    </p>
                  </div>
                  <span class="mt-1 text-sm text-gray-500">{expandedSections[section.id] ? 'Hide' : 'Show'}</span>
                </button>

                {#if expandedSections[section.id]}
                  <div class="space-y-5 border-t border-gray-100 px-5 py-5">
                    {#if workspaceMode === 'edit' && (section.groups ?? []).some((group) => groupVisible(group))}
                      <div class="space-y-5">
                        {#each section.groups ?? [] as group (group.id)}
                          {#if groupVisible(group)}
                            {@const groupCounts = groupSummary(group)}
                            <div class="rounded-xl border border-gray-200 bg-gray-50/40">
                              <button
                                type="button"
                                class="flex w-full items-start justify-between gap-4 rounded-xl px-4 py-4 text-left hover:bg-gray-100/60"
                                onclick={() => toggleGroup(group.id)}
                              >
                                <div>
                                  <div class="flex flex-wrap items-center gap-2">
                                    <h5 class="text-sm font-semibold uppercase tracking-wide text-gray-600">{translated(group.label, group.id)}</h5>
                                    {#if groupCounts.required_total > 0}
                                      <span class="rounded-full bg-white px-2 py-0.5 text-xs font-medium text-slate-700">
                                        {groupCounts.required_filled}/{groupCounts.required_total} required
                                      </span>
                                    {/if}
                                    {#if groupCounts.errors > 0}
                                      <span class="rounded-full bg-rose-50 px-2 py-0.5 text-xs font-medium text-rose-700">{groupCounts.errors} errors</span>
                                    {/if}
                                    {#if groupCounts.warnings > 0}
                                      <span class="rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700">{groupCounts.warnings} warnings</span>
                                    {/if}
                                  </div>
                                  <p class="mt-2 text-sm text-gray-500">
                                    {groupCounts.filled_fields} fields filled
                                    {#if groupCounts.required_remaining > 0}
                                      · {groupCounts.required_remaining} required still missing
                                    {/if}
                                  </p>
                                </div>
                                <span class="mt-1 text-sm text-gray-500">{expandedGroups[group.id] ? 'Hide' : 'Show'}</span>
                              </button>

                              {#if expandedGroups[group.id]}
                                <div class="space-y-4 border-t border-gray-200 px-4 py-4">
                                  {#each group.fields as field (field.override_id)}
                                    {#if fieldVisibleInEdit(field)}
                                      {@const fieldCounts = fieldIssueCounts(field)}
                                      <div id={`example-field-${field.override_id}`} class={`rounded-lg border bg-white p-4 shadow-sm ${activeMatch === field.override_id ? 'ring-2 ring-pletka-primary ring-offset-2' : ''} ${fieldCounts.errors > 0 ? 'border-rose-200 bg-rose-50/30' : fieldCounts.warnings > 0 ? 'border-amber-200 bg-amber-50/30' : 'border-white'}`}>
                                        <div class="mb-3 flex items-start justify-between gap-3">
                                          <div>
                                            <div class="flex flex-wrap items-center gap-2">
                                              <h6 class="text-sm font-semibold text-gray-900">{translated(field.label, field.field_id)}</h6>
                                              <span class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-500">{field.field_semantic_id}</span>
                                              {#if requiredMinimum(field) > 0}
                                                <span class="rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-700">Required</span>
                                              {/if}
                                              {#if field.repeatable}
                                                <span class="rounded-full bg-sky-50 px-2 py-0.5 text-xs font-medium text-sky-700">Repeatable</span>
                                              {/if}
                                            </div>
                                            {#if translated(field.help)}
                                              <p class="mt-1 text-sm text-gray-500">{translated(field.help)}</p>
                                            {/if}
                                            {#if fieldContextNotes(field).length}
                                              <div class="mt-2 flex flex-wrap gap-2">
                                                {#each fieldContextNotes(field) as note}
                                                  <span class="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-600">{note}</span>
                                                {/each}
                                              </div>
                                            {/if}
                                          </div>
                                          {#if field.repeatable}
                                            <button type="button" class="text-sm font-medium text-pletka-primary hover:text-pletka-secondary" onclick={() => addOccurrence(field)}>+ Add value</button>
                                          {/if}
                                        </div>
                                        {#if errorMessages(field.issues).length}
                                          <ul class="mb-3 list-disc space-y-1 pl-5 text-sm text-rose-700">
                                            {#each errorMessages(field.issues) as message, idx (`${field.override_id}-error-${idx}`)}
                                              <li>{message}</li>
                                            {/each}
                                          </ul>
                                        {/if}
                                        {#if warningMessages(field.issues).length}
                                          <ul class="mb-3 list-disc space-y-1 pl-5 text-sm text-amber-700">
                                            {#each warningMessages(field.issues) as message, idx (`${field.override_id}-warning-${idx}`)}
                                              <li>{message}</li>
                                            {/each}
                                          </ul>
                                        {/if}
                                        <div class="space-y-3">
                                          {#each fieldOccurrences(field) as occurrence, occurrenceIdx (occurrence.occurrence_index)}
                                            {@const fieldDef = occurrenceFieldDef(field, occurrence.occurrence_index)}
                                            {@const key = fieldKey(field)}
                                            {@const occurrenceErrorMessages = occurrenceIssues(field, occurrence.occurrence_index)}
                                            {@const occurrenceWarningMessages = occurrenceWarnings(field, occurrence.occurrence_index)}
                                            {@const linkedSelection = field.value_kind === 'example_ref' ? linkedExampleByID(valuesByOverride[key][occurrenceIdx].value) : null}
                                            {@const eligibleExamples = field.value_kind === 'example_ref' ? eligibleLinkedExamples(field) : []}
                                            <div class={`rounded-md border p-3 ${occurrenceErrorMessages.length ? 'border-rose-200 bg-rose-50/40' : occurrenceWarningMessages.length ? 'border-amber-200 bg-amber-50/40' : 'border-gray-100 bg-gray-50/60'}`}>
                                              <div class="mb-2 flex items-center justify-between">
                                                <div class="flex items-center gap-2">
                                                  <span class="text-xs font-medium uppercase tracking-wide text-gray-500">Occurrence {occurrence.occurrence_index + 1}</span>
                                                  {#if occurrenceErrorMessages.length}
                                                    <span class="rounded-full bg-rose-100 px-2 py-0.5 text-[11px] font-medium text-rose-700">{occurrenceErrorMessages.length} error{occurrenceErrorMessages.length === 1 ? '' : 's'}</span>
                                                  {:else if occurrenceWarningMessages.length}
                                                    <span class="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-medium text-amber-700">{occurrenceWarningMessages.length} warning{occurrenceWarningMessages.length === 1 ? '' : 's'}</span>
                                                  {/if}
                                                </div>
                                                {#if field.repeatable && fieldOccurrences(field).length > 1}
                                                  <button type="button" class="text-xs font-medium text-red-600 hover:text-red-700" onclick={() => removeOccurrence(field, occurrence.occurrence_index)}>Remove</button>
                                                {/if}
                                              </div>
                                              {#if field.value_kind === 'concept' && field.concept_sources?.length && !field.set_value}
                                                <ConceptPicker
                                                  {field}
                                                  bind:value={valuesByOverride[key][occurrenceIdx].value}
                                                  {lang}
                                                  errors={occurrenceErrorMessages}
                                                  onchoose={rememberConceptLabel}
                                                />
                                              {:else}
                                                <WidgetDispatcher
                                                  field={fieldDef}
                                                  bind:value={valuesByOverride[key][occurrenceIdx].value}
                                                  formValues={{}}
                                                  {lang}
                                                  languages={schema.ui.languages}
                                                  errors={occurrenceErrorMessages}
                                                />
                                              {/if}
                                              {#if field.value_kind === 'example_ref'}
                                                <div class="mt-3 space-y-2">
                                                  <div class="flex flex-wrap items-center gap-2 text-xs text-gray-500">
                                                    <span>{eligibleExamples.length} eligible example{eligibleExamples.length === 1 ? '' : 's'}</span>
                                                    {#if field.resource_models?.length}
                                                      <span>· scoped to {field.resource_models.length} model{field.resource_models.length === 1 ? '' : 's'}</span>
                                                    {/if}
                                                  </div>
                                                  {#if linkedSelection}
                                                    <div class="rounded-md border border-blue-200 bg-blue-50/50 px-3 py-2">
                                                      <div class="flex flex-wrap items-center gap-2">
                                                        {#if examplePageURL(linkedSelection.id)}
                                                          <a href={examplePageURL(linkedSelection.id)} target="_blank" rel="noopener" class="text-sm font-medium text-blue-900 underline decoration-blue-300 hover:decoration-blue-700">{translated(linkedSelection.title, linkedSelection.id)}</a>
                                                        {:else}
                                                          <span class="text-sm font-medium text-blue-900">{translated(linkedSelection.title, linkedSelection.id)}</span>
                                                        {/if}
                                                        <span class="rounded bg-white px-2 py-0.5 font-mono text-xs text-blue-700">{linkedSelection.entity_id}</span>
                                                        <span class={`rounded-full px-2 py-0.5 text-[11px] font-medium ${linkedStatusClasses(linkedSelection.status)}`}>
                                                          {linkedSelection.status ? linkedSelection.status.replace('_', ' ') : 'draft'}
                                                        </span>
                                                      </div>
                                                    </div>
                                                  {:else if eligibleExamples.length === 0}
                                                    <div class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
                                                      No eligible examples exist yet for this field.
                                                    </div>
                                                  {/if}
                                                  {#if !linkedSelection && (field.expected_value_type || '').trim() === 'Model' && (field.resource_models ?? []).length > 0}
                                                    {@const models = field.resource_models ?? []}
                                                    <div class="rounded-md border border-dashed border-gray-300 bg-white px-3 py-2">
                                                      <label class="block text-xs font-medium text-gray-600" for={`stub-${field.override_id}-${occurrence.occurrence_index}`}>
                                                        Or create a new draft {models.length === 1 ? translated(models[0].name, models[0].semantic_id || models[0].id) : 'example'} named
                                                      </label>
                                                      <div class="mt-1 flex flex-wrap items-center gap-2">
                                                        <input
                                                          id={`stub-${field.override_id}-${occurrence.occurrence_index}`}
                                                          type="text"
                                                          class="min-w-[12rem] flex-1 rounded-md border border-gray-300 px-2 py-1 text-sm"
                                                          placeholder="e.g. Van Gogh"
                                                          bind:value={valuesByOverride[key][occurrenceIdx].stub_label}
                                                        />
                                                        {#if models.length > 1}
                                                          <select class="rounded-md border border-gray-300 px-2 py-1 text-sm" bind:value={valuesByOverride[key][occurrenceIdx].stub_model}>
                                                            <option value="">Choose model…</option>
                                                            {#each models as model (model.id)}
                                                              <option value={model.id}>{translated(model.name, model.semantic_id || model.id)}</option>
                                                            {/each}
                                                          </select>
                                                        {/if}
                                                      </div>
                                                      {#if occurrenceIsStub(field, occurrence)}
                                                        <p class="mt-1 text-xs text-gray-500">Saving creates a <span class="font-medium">draft</span> example with this title and links it here. Type the same name in another field to link the same draft. Finish it from the Examples tab.</p>
                                                      {/if}
                                                    </div>
                                                  {/if}
                                                </div>
                                              {/if}
                                              {#if occurrenceWarningMessages.length}
                                                <ul class="mt-2 list-disc space-y-1 pl-5 text-sm text-amber-700">
                                                  {#each occurrenceWarningMessages as message, idx (`${field.override_id}-${occurrence.occurrence_index}-warning-${idx}`)}
                                                    <li>{message}</li>
                                                  {/each}
                                                </ul>
                                              {/if}
                                            </div>
                                          {/each}
                                        </div>
                                      </div>
                                    {/if}
                                  {/each}
                                </div>
                              {/if}
                            </div>
                          {/if}
                        {/each}
                      </div>
                    {/if}

                    {#if workspaceMode === 'overview'}
                      {#if (section.groups ?? []).some((group) => groupVisibleInOverview(group))}
                        <div class="space-y-5">
                          {#each section.groups ?? [] as group (group.id)}
                            {#if groupVisibleInOverview(group)}
                              {@const groupCounts = groupSummary(group)}
                              <div class="rounded-xl border border-gray-200 bg-gray-50/40">
                                <div class="border-b border-gray-200 px-4 py-4">
                                  <div class="flex flex-wrap items-center gap-2">
                                    <h5 class="text-sm font-semibold uppercase tracking-wide text-gray-600">{translated(group.label, group.id)}</h5>
                                    {#if groupCounts.required_total > 0}
                                      <span class="rounded-full bg-white px-2 py-0.5 text-xs font-medium text-slate-700">
                                        {groupCounts.required_filled}/{groupCounts.required_total} required
                                      </span>
                                    {/if}
                                  </div>
                                </div>
                                <div class="space-y-4 px-4 py-4">
                                  {#each group.fields as field (field.override_id)}
                                    {#if fieldHasOverviewContent(field)}
                                      <div class="rounded-lg border border-white bg-white p-4 shadow-sm">
                                        <div class="flex flex-wrap items-center gap-2">
                                          <h6 class="text-sm font-semibold text-gray-900">{translated(field.label, field.field_id)}</h6>
                                          <span class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-500">{field.field_semantic_id}</span>
                                          {#if field.value_kind === 'example_ref'}
                                            <span class="rounded-full bg-blue-50 px-2 py-0.5 text-xs font-medium text-blue-700">Linked example</span>
                                          {/if}
                                        </div>
                                        <div class="mt-3 space-y-2">
                                          {#each fieldOccurrences(field) as occurrence (occurrence.occurrence_index)}
                                            {#if !occurrenceIsBlank(field, occurrence)}
                                              <div class="rounded-md border border-gray-100 bg-gray-50/60 px-3 py-2">
                                                {#if field.repeatable}
                                                  <div class="mb-1 text-xs font-medium uppercase tracking-wide text-gray-500">Occurrence {occurrence.occurrence_index + 1}</div>
                                                {/if}
                                                {#if occurrenceLinkURL(field, occurrence)}
                                          <a href={occurrenceLinkURL(field, occurrence)} target="_blank" rel="noopener" class="text-sm text-blue-900 underline decoration-blue-300 hover:decoration-blue-700">{occurrenceDisplayValue(field, occurrence)}</a>
                                        {:else}
                                          <div class="text-sm text-gray-800">{occurrenceDisplayValue(field, occurrence)}</div>
                                        {/if}
                                                {#if occurrenceTechnicalValue(field, occurrence)}
                                                  <div class="mt-1 break-all font-mono text-xs text-gray-500">{occurrenceTechnicalValue(field, occurrence)}</div>
                                                {/if}
                                              </div>
                                            {/if}
                                          {/each}
                                        </div>
                                      </div>
                                    {/if}
                                  {/each}
                                </div>
                              </div>
                            {/if}
                          {/each}
                        </div>
                      {/if}
                    {/if}
                  </div>
                {/if}
              </section>
            {/if}
          {/each}
        </div>
      </div>

      {#if workspaceMode === 'edit'}
        <div class="flex items-center justify-end gap-3">
          <button
            type="button"
            class="rounded-md border border-gray-300 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50"
            onclick={() => oncancel?.()}
          >
            Cancel
          </button>
          <button
            type="button"
            class="rounded-md bg-pletka-primary px-4 py-2 text-sm font-medium text-white hover:bg-pletka-secondary disabled:opacity-60"
            disabled={saving}
            onclick={submit}
          >
            {saving ? 'Saving…' : translated(schema.ui.submit_label, 'Save Example')}
          </button>
        </div>
      {/if}
    {/if}
  </div>
{/if}
