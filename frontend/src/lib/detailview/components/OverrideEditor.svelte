<script lang="ts">
  import type {
    AdoptCollectionResponse,
    OverrideCategoryOption,
    OverrideEditorItem,
    OverrideEditorPresence,
    OverridePathSuggestion,
    OverridePathSuggestionsResponse,
    OverrideSearchResponse,
    OverrideSearchResult,
  } from '$lib/detailview/override-editor-types';
  import { OverrideEditorState, UNCATEGORIZED_ID } from '$lib/detailview/override-editor-state.svelte';
  import type { ViewField, ViewItem, ViewSection } from '$lib/detailview/types';
  import { tr, type OriginInfo } from '$lib/types/weave-types';
  import { getUILang } from '$lib/utils/locale';
  import { pathElementsToDisplay } from '$lib/utils/ontology-path';
  import { onDestroy, onMount } from 'svelte';
  import { confirmAction } from '$lib/stores/confirm';
  import { addToast } from '$lib/stores/toast';
  import {
    collectionIDForGroup,
    groupWidgetForCollectionState,
    isCollectionGroup,
  } from '$lib/detailview/group-kind';
  import LoadingState from '$lib/components/shared/LoadingState.svelte';
  import Badge from '$lib/components/shared/Badge.svelte';
  import PathPreview from './PathPreview.svelte';
  import EditableFieldRow from './EditableFieldRow.svelte';
  import EditableGroup from './EditableGroup.svelte';
  import OverrideSidebar from './OverrideSidebar.svelte';
  import CollectionSidebar from './CollectionSidebar.svelte';

  let {
    saveUrl,
    viewState,
    onback,
    onsuccess,
  }: {
    saveUrl: string;
    /** Parent EntityViewState. The composition editor borrows the
     *  sticky-header search query + match cursor by pointing
     *  viewState's category source at its own draft tree on mount. */
    viewState: import('$lib/detailview/state.svelte').EntityViewState;
    onback?: () => void;
    onsuccess?: () => void | Promise<void>;
  } = $props();

  // svelte-ignore state_referenced_locally
  const editorState = new OverrideEditorState(saveUrl);
  const lang = getUILang();
  let addTab = $state<'field' | 'collection' | 'category'>('field');
  let addModalOpen = $state(false);
  let activeCategoryId = $state<string | null>(null);
  let searchQuery = $state('');
  let searchLoading = $state(false);
  let searchError = $state<string | null>(null);
  let fieldResults = $state<OverrideSearchResult[]>([]);
  let collectionResults = $state<OverrideSearchResult[]>([]);
  let categoryResults = $state<OverrideCategoryOption[]>([]);
  let pathSuggestions = $state<OverridePathSuggestion[]>([]);
  let currentPath = $state('');
  let pathQuery = $state('');
  let usePathSearch = $state(false);
  let selectedFields = $state<OverrideSearchResult[]>([]);
  let selectedCollections = $state<OverrideSearchResult[]>([]);
  let selectedCategories = $state<OverrideCategoryOption[]>([]);
  let searchRequestId = 0;
  let suggestionRequestId = 0;
  let saving = $state(false);
  // discarding tracks a "discard my draft and reload" round trip the
  // same way `saving` tracks a save — Save stays clickable while dirty
  // is still true during that refetch, and the two can otherwise cross:
  // a save can land, adopt its new fingerprint, and then an
  // already-in-flight discard's earlier GET can overwrite the tree with
  // the pre-save payload while the state still holds the post-save
  // fingerprint — silently losing what the save just wrote the next time
  // the curator saves again (fix round 2, item 5).
  let discarding = $state(false);
  let saveError = $state<string | null>(null);
  // saveConflictKind mirrors the two distinct 409s the save endpoint can
  // return (see override_write.go): a real conflict (someone else's save
  // already landed) vs. a transient lock contention (someone else is
  // mid-save right now). Different copy, different actions — branch on
  // this, never on HTTP status alone.
  let saveConflictKind = $state<'pattern_changed' | 'save_in_progress' | null>(null);
  // dismissedStaleDraft hides the stale-draft banner once the curator
  // has explicitly chosen "Keep mine" for it. Never implies the draft
  // stopped being stale — editorState.staleDraft is untouched.
  let dismissedStaleDraft = $state(false);
  let othersEditing = $state<OverrideEditorPresence[]>([]);
  let viewMode = $state<'compact' | 'detailed'>('compact');
  let expandedCategories = $state<Set<string>>(new Set());
  let collapsedItems = $state<Set<string>>(new Set());
  let expandedFields = $state<Set<string>>(new Set());
  // Active row for the deep-edit sidebar. {category, item, override_id}.
  let activeRowKey = $state<{ categoryId: string; itemId: string; overrideID: number } | null>(null);
  // When set, sidebar shows the collection-level editor for this group
  // (multilingual collection-name override). Mutually exclusive with
  // activeRowKey — opening one closes the other.
  let activeCollectionKey = $state<{ categoryId: string; itemId: string } | null>(null);

  $effect(() => {
    editorState.init();
  });

  $effect(() => {
    if (!editorState.response) return;
    if (editorState.response.categories.length === 0) return;
    if (expandedCategories.size > 0) return;
    expandedCategories = new Set(editorState.response.categories.map((category) => category.category_id));
  });

  // Lend the sticky-header search to the editor while it's mounted —
  // viewState's matchSet/matchOrder/activeOverrideID will run against
  // the draft tree, the EditableFieldRow components already read those
  // off viewState. Restore the default source on unmount.
  $effect(() => {
    viewState.setCategoriesSource(() => editorState.response?.categories ?? []);
    return () => viewState.clearCategoriesSource();
  });

  // Auto-expand all groups when search is active so matches are
  // visible. Imperative, NOT inside a $effect that reads its own
  // writes (that's the loop trap).
  let lastSearchActive = false;
  $effect(() => {
    const active = !!viewState.searchQuery.trim();
    if (active && !lastSearchActive && editorState.response) {
      expandedCategories = new Set(editorState.response.categories.map((c) => c.category_id));
      collapsedItems = new Set();
    }
    lastSearchActive = active;
  });


  $effect(() => {
    if (!editorState.response || !addModalOpen || addTab === 'category' || usePathSearch) return;
    const query = searchQuery.trim();
    const timeout = window.setTimeout(() => {
      void runTextSearch(query);
    }, 250);
    return () => window.clearTimeout(timeout);
  });

  $effect(() => {
    if (!editorState.response || !addModalOpen || addTab === 'category' || !usePathSearch) return;
    const query = pathQuery.trim();
    if (!query) {
      pathSuggestions = [];
      return;
    }
    const timeout = window.setTimeout(() => {
      void loadPathSuggestions(query);
    }, 250);
    return () => window.clearTimeout(timeout);
  });

  $effect(() => {
    if (!editorState.response || !addModalOpen || addTab === 'category' || !usePathSearch) return;
    const path = currentPath.trim();
    if (!path) {
      fieldResults = [];
      collectionResults = [];
      return;
    }
    const timeout = window.setTimeout(() => {
      void runPathSearch(path);
    }, 250);
    return () => window.clearTimeout(timeout);
  });

  // --- Presence heartbeat -------------------------------------------------
  // A notice only — never a lock. Beats every 20s while the editor is in
  // edit mode, plus once as soon as the payload loads, and tells the
  // server it left on unmount/pagehide. presenceSessionId is per-tab so a
  // curator with two tabs open counts as one editor with two sessions,
  // never two editors (see override.Presence).
  let presenceSessionId = '';
  let presenceTimer: ReturnType<typeof setInterval> | null = null;
  // presenceArmed is a plain (non-reactive) flag, not $state: it only
  // guards the one-time "fire the first beat" effect below against
  // re-running (the effect reads editorState.response, which is
  // reassigned on every edit) and against ever sending a "left" beacon
  // for a viewer who never actually beat "editing" in the first place.
  let presenceArmed = false;

  function newPresenceSessionId(): string {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      return crypto.randomUUID();
    }
    return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  }

  function presenceUrl(): string | null {
    return editorState.response?.available.presence_url ?? null;
  }

  async function beatPresence() {
    const url = presenceUrl();
    if (!url || !canEditOverrides()) return;
    try {
      const res = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ state: 'editing', session_id: presenceSessionId }),
      });
      if (!res.ok) {
        // Age the claim out rather than leave it on screen — the server's
        // own presence entries expire after 60s of missed heartbeats, and
        // a failed beat (network down, lapsed session) means we no longer
        // know who is really still editing.
        othersEditing = [];
        return;
      }
      const data = (await res.json()) as { editors?: OverrideEditorPresence[] };
      othersEditing = data.editors ?? [];
    } catch {
      othersEditing = [];
    }
  }

  function sendLeaveBeacon() {
    const url = presenceUrl();
    if (!url) return;
    const body = JSON.stringify({ state: 'left', session_id: presenceSessionId });
    if (typeof navigator !== 'undefined' && typeof navigator.sendBeacon === 'function') {
      navigator.sendBeacon(url, new Blob([body], { type: 'application/json' }));
      return;
    }
    void fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body,
      keepalive: true,
    }).catch(() => {});
  }

  function handlePageHide() {
    if (presenceArmed) sendLeaveBeacon();
  }

  onMount(() => {
    presenceSessionId = newPresenceSessionId();
    presenceTimer = setInterval(() => { void beatPresence(); }, 20000);
    window.addEventListener('pagehide', handlePageHide);
  });

  onDestroy(() => {
    if (presenceTimer !== null) clearInterval(presenceTimer);
    window.removeEventListener('pagehide', handlePageHide);
    if (presenceArmed) sendLeaveBeacon();
    clearSaveInProgressTimer();
  });

  // Fire the first heartbeat the moment the payload (and its
  // presence_url) is ready, rather than waiting up to 20s for the first
  // interval tick. Never rearms once presenceArmed flips true, and never
  // returns a cleanup, so the later reruns this effect gets from every
  // edit (it reads editorState.response) are harmless no-ops.
  $effect(() => {
    if (presenceArmed) return;
    if (!editorState.response || !canEditOverrides()) return;
    presenceArmed = true;
    void beatPresence();
  });

  function presenceTimeLabel(since: string): string {
    return new Date(since).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
  }

  // joinNames renders a natural-language list: "A", "A and B", or
  // "A, B and C" — never a bare comma-joined string, and never "and"
  // before a single trailing name is missing its Oxford comma partner.
  function joinNames(names: string[]): string {
    if (names.length <= 1) return names[0] ?? '';
    if (names.length === 2) return `${names[0]} and ${names[1]}`;
    return `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`;
  }

  // earliestSince picks the oldest `since` across every listed editor, so
  // the plural line can carry the same "(since HH:MM)" the singular line
  // does instead of silently dropping it.
  function earliestSince(editors: OverrideEditorPresence[]): string {
    return editors.reduce(
      (earliest, editor) => (new Date(editor.since) < new Date(earliest) ? editor.since : earliest),
      editors[0].since,
    );
  }

  function presenceLineText(): string {
    if (othersEditing.length === 0) return '';
    const names = joinNames(othersEditing.map((editor) => editor.name));
    const verb = othersEditing.length === 1 ? 'is' : 'are';
    const since = presenceTimeLabel(earliestSince(othersEditing));
    return `${names} ${verb} also editing this pattern (since ${since})`;
  }

  // --- Conflict bar --------------------------------------------------------
  // Two independent triggers can show this bar: a 409 on save (pattern
  // changed, or someone else's save is in progress right now) and a
  // restored draft whose stored fingerprint no longer matches what was
  // live at load time. Never merges, never auto-overwrites — "Discard my
  // changes and reload" is the only action that throws the draft away,
  // and it always asks first (confirmAction) since it is a one-click,
  // irreversible loss of everything the curator has typed.
  //
  // save_in_progress renders as a separate, calmer (blue, not amber)
  // notice with no buttons: nothing has changed yet, the draft is
  // untouched, and retrying shortly will work — it is not the same kind
  // of decision as an actual conflict, so it must not look like one.
  const SAVE_IN_PROGRESS_NOTICE_MS = 15000;
  let saveInProgressTimer: ReturnType<typeof setTimeout> | null = null;

  function clearSaveInProgressTimer() {
    if (saveInProgressTimer !== null) {
      clearTimeout(saveInProgressTimer);
      saveInProgressTimer = null;
    }
  }

  function isTransientSaveNotice(): boolean {
    return saveConflictKind === 'save_in_progress';
  }

  function isActionableConflict(): boolean {
    return saveConflictKind === 'pattern_changed' || (saveConflictKind === null && editorState.staleDraft && !dismissedStaleDraft);
  }

  function conflictBannerText(): string {
    if (saveConflictKind === 'save_in_progress') {
      return 'Someone else is saving this pattern right now. Your changes are still here — try saving again in a moment.';
    }
    if (saveConflictKind === 'pattern_changed') {
      return 'Someone saved this pattern while you were editing. Your changes are still here, but saving is blocked until you reload or discard them.';
    }
    // Deliberately as certain as the pattern_changed copy above, not
    // softened to "may have changed": staleDraft is only ever set when
    // the draft's stored fingerprint is missing or differs from what was
    // live at load time, which means the next save is guaranteed to hit
    // the same 409 — not merely possible. Keeping the two messages
    // equally definite also means dismissing a live pattern_changed
    // conflict (keepEditingMine, below) and falling back to this text
    // never downgrades what the curator is told: both say saving stays
    // blocked until they reload or discard.
    return 'This pattern has changed since your unsaved changes were made. Your changes are still here, but saving is blocked until you reload or discard them.';
  }

  // discardDraftAndReload is the one path that throws a curator's local
  // draft away — shared by the conflict bar's "Discard my changes and
  // reload" and the unrelated unsaved-changes banner's "Discard Draft"
  // (same destruction, same confirmation; see fix round 2, item 2). Both
  // always ask first via confirmAction, since this is a one-click,
  // irreversible loss of everything typed.
  //
  // editorState.discardDraft() swallows its own fetch failure into
  // `error` rather than throwing (round 1 fix — so a failed refetch never
  // clears the draft), but `error` only ever renders in the "failed to
  // load" screen, which is unreachable once a response already exists —
  // exactly the state a discard is invoked from. Without surfacing it
  // here too, a curator on flaky wifi confirms the dialog and sees
  // nothing at all happen (fix round 2, item 1) — toast it instead, since
  // toasts outlive this component if the parent unmounts it.
  async function discardDraftAndReload() {
    const ok = await confirmAction({
      title: 'Discard my changes and reload',
      message: 'This throws away every change you have made since you started editing, and cannot be undone.',
      confirmLabel: 'Discard and reload',
      cancelLabel: 'Cancel',
      danger: true,
    });
    if (!ok) return;
    saveConflictKind = null;
    clearSaveInProgressTimer();
    dismissedStaleDraft = false;
    saveError = null;
    discarding = true;
    try {
      await editorState.discardDraft();
      if (editorState.error) {
        addToast('error', `Could not reload: ${editorState.error}. Your changes are still here.`);
      }
    } finally {
      discarding = false;
    }
  }

  function keepEditingMine() {
    if (saveConflictKind === 'pattern_changed') {
      saveConflictKind = null;
      clearSaveInProgressTimer();
      return;
    }
    dismissedStaleDraft = true;
  }

  function activeCategoryName(): string {
    if (!editorState.response) return '';
    const categoryId = activeCategoryId;
    if (!categoryId) return '';
    const category = editorState.response.categories.find((item) => item.category_id === categoryId);
    if (category) return tr(category.category_name, lang, category.category_id);
    const available = editorState.response.available_categories?.find((item) => item.id === categoryId);
    if (available) return tr(available.name, lang, categoryId);
    return '';
  }

  function scopeLabel(scope?: OverrideSearchResult['ontology_scope']): string {
    if (!scope) return '';
    return pathElementsToDisplay([scope])[0]?.label ?? '';
  }

  function sourceOriginLabel(origin?: OriginInfo): string {
    if (!origin || (origin.kind !== 'inherited' && origin.kind !== 'adopted')) return '';
    return origin.source_project_label || origin.source_project_id || 'source project';
  }

  function originBadgeText(origin?: OriginInfo): string {
    if (!origin) return '';
    if (origin.kind === 'inherited') return `Inherited · ${sourceOriginLabel(origin)}`;
    if (origin.kind === 'adopted') return `Adopted · ${sourceOriginLabel(origin)}`;
    return '';
  }

  function fieldActionLabel(result: OverrideSearchResult): string {
    if (result.is_linked) return 'Added';
    if (isFieldSelected(result.id)) return 'Remove';
    if (result.origin?.kind === 'inherited') return 'Adopt';
    return 'Add';
  }

  function collectionActionLabel(result: OverrideSearchResult): string {
    if (isCollectionSelected(result.id)) return 'Remove';
    if (result.origin?.kind === 'inherited') return 'Adopt';
    return 'Add';
  }

  function openAdd(categoryId: string) {
    if (!canOpenAdd() || !canEditOverrides()) return;
    addTab = 'field';
    addModalOpen = true;
    activeCategoryId = categoryId;
    searchError = null;
    fieldResults = [];
    collectionResults = [];
    categoryResults = [];
    pathSuggestions = [];
    searchQuery = '';
    currentPath = '';
    pathQuery = '';
    usePathSearch = false;
    selectedFields = [];
    selectedCollections = [];
    selectedCategories = [];
    loadCategories();
    void runTextSearch('');
  }

  function closeAdd() {
    addTab = 'field';
    addModalOpen = false;
    activeCategoryId = null;
    searchError = null;
    fieldResults = [];
    collectionResults = [];
    categoryResults = [];
    pathSuggestions = [];
    searchQuery = '';
    currentPath = '';
    pathQuery = '';
    usePathSearch = false;
    selectedFields = [];
    selectedCollections = [];
    selectedCategories = [];
  }

  function canEditOverrides(): boolean {
    return editorState.response?.capabilities.can_edit_overrides ?? false;
  }

  function canAddFields(): boolean {
    return editorState.response?.capabilities.can_add_field ?? false;
  }

  function canAddCollections(): boolean {
    return editorState.response?.capabilities.can_add_collection ?? false;
  }

  function canOpenAdd(): boolean {
    return canAddFields() || canAddCollections();
  }

  function supportsCollections(): boolean {
    return canAddCollections();
  }

  function openGlobalAdd() {
    if (!canOpenAdd()) return;
    addTab = 'field';
    addModalOpen = true;
    activeCategoryId = UNCATEGORIZED_ID;
    searchError = null;
    fieldResults = [];
    collectionResults = [];
    categoryResults = [];
    pathSuggestions = [];
    searchQuery = '';
    currentPath = '';
    pathQuery = '';
    usePathSearch = false;
    selectedFields = [];
    selectedCollections = [];
    selectedCategories = [];
    loadCategories();
    void runTextSearch('');
  }

  function chooseAddTab(kind: 'field' | 'collection' | 'category') {
    if (kind === 'collection' && !supportsCollections()) return;
    if (kind !== 'collection' && !canAddFields() && kind === 'field') return;
    if (!canEditOverrides()) return;
    addTab = kind;
    searchQuery = '';
    currentPath = '';
    pathQuery = '';
    fieldResults = [];
    collectionResults = [];
    pathSuggestions = [];
    searchError = null;
    usePathSearch = false;
    if (kind !== 'category') {
      void runTextSearch('');
    }
  }

  function hasPendingSelection(): boolean {
    return selectedFields.length > 0 || selectedCollections.length > 0 || selectedCategories.length > 0;
  }

  function isFieldSelected(id: string): boolean {
    return selectedFields.some((item) => item.id === id);
  }

  function isCollectionSelected(id: string): boolean {
    return selectedCollections.some((item) => item.id === id);
  }

  function isCategorySelected(id: string): boolean {
    return selectedCategories.some((item) => item.id === id);
  }

  function toggleFieldSelection(result: OverrideSearchResult) {
    selectedFields = isFieldSelected(result.id)
      ? selectedFields.filter((item) => item.id !== result.id)
      : [...selectedFields, result];
  }

  function toggleCollectionSelection(result: OverrideSearchResult) {
    selectedCollections = isCollectionSelected(result.id)
      ? selectedCollections.filter((item) => item.id !== result.id)
      : [...selectedCollections, result];
  }

  function toggleCategorySelection(category: OverrideCategoryOption) {
    selectedCategories = isCategorySelected(category.id)
      ? selectedCategories.filter((item) => item.id !== category.id)
      : [...selectedCategories, category];
  }

  function removeSelectedChip(kind: 'field' | 'collection' | 'category', id: string) {
    if (kind === 'field') {
      selectedFields = selectedFields.filter((item) => item.id !== id);
      return;
    }
    if (kind === 'collection') {
      selectedCollections = selectedCollections.filter((item) => item.id !== id);
      return;
    }
    selectedCategories = selectedCategories.filter((item) => item.id !== id);
  }

  function itemKey(categoryId: string, itemId: string): string {
    return `${categoryId}-${itemId}`;
  }

  function isExpanded(categoryId: string, itemId: string): boolean {
    return !collapsedItems.has(itemKey(categoryId, itemId));
  }

  function toggleItem(categoryId: string, itemId: string) {
    const key = itemKey(categoryId, itemId);
    const next = new Set(collapsedItems);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    collapsedItems = next;
  }

  function collapseAllItems() {
    if (!editorState.response) return;
    const next = new Set<string>();
    for (const category of editorState.response.categories) {
      for (const item of category.items) {
        next.add(itemKey(category.category_id, item.id));
      }
    }
    collapsedItems = next;
  }

  function expandAllItems() {
    collapsedItems = new Set();
  }

  function expandAllCategories() {
    if (!editorState.response) return;
    expandedCategories = new Set(editorState.response.categories.map((category) => category.category_id));
  }

  function collapseAllCategories() {
    expandedCategories = new Set();
  }

  function toggleCategory(categoryId: string) {
    const next = new Set(expandedCategories);
    if (next.has(categoryId)) next.delete(categoryId);
    else next.add(categoryId);
    expandedCategories = next;
  }

  function toggleField(fieldId: string) {
    const next = new Set(expandedFields);
    if (next.has(fieldId)) next.delete(fieldId);
    else next.add(fieldId);
    expandedFields = next;
  }

  function canHideFields(): boolean {
    return editorState.response?.capabilities.can_hide_fields ?? false;
  }

  function activeRow(): {
    field: OverrideEditorItem['fields'][number];
    categoryId: string;
    itemId: string;
    inCollectionGroup: boolean;
  } | null {
    if (!editorState.response || !activeRowKey) return null;
    const cat = editorState.response.categories.find((c) => c.category_id === activeRowKey!.categoryId);
    if (!cat) return null;
    const item = cat.items.find((i) => i.id === activeRowKey!.itemId);
    if (!item) return null;
    const f = item.fields.find((x) => x.override_id === activeRowKey!.overrideID);
    if (!f) return null;
    return {
      field: f,
      categoryId: cat.category_id,
      itemId: item.id,
      inCollectionGroup: isCollectionGroup(item),
    };
  }

  function openRowEditor(categoryId: string, itemId: string, overrideID: number) {
    activeCollectionKey = null;
    activeRowKey = { categoryId, itemId, overrideID };
  }
  function closeRowEditor() {
    activeRowKey = null;
  }
  function openCollectionEditor(categoryId: string, itemId: string) {
    activeRowKey = null;
    activeCollectionKey = { categoryId, itemId };
  }
  function closeCollectionEditor() {
    activeCollectionKey = null;
  }
  function activeCollection(): { item: OverrideEditorItem; categoryId: string } | null {
    if (!editorState.response || !activeCollectionKey) return null;
    const cat = editorState.response.categories.find((c) => c.category_id === activeCollectionKey!.categoryId);
    if (!cat) return null;
    const it = cat.items.find((i) => i.id === activeCollectionKey!.itemId);
    if (!it) return null;
    return { item: it, categoryId: cat.category_id };
  }

  function fieldSidebarSchemaUrl(inCollectionGroup: boolean, expectedValueType?: string): string {
    const base = editorState.response?.available.field_sidebar_schema_url;
    if (!base) return '';
    const params = new URLSearchParams({
      entity_type: editorState.response?.entity_type ?? 'model',
      group_widget: groupWidgetForCollectionState(inCollectionGroup),
    });
    if (expectedValueType) params.set('expected_value_type', expectedValueType);
    return `${base}?${params.toString()}`;
  }

  function collectionSidebarSchemaUrl(): string {
    const base = editorState.response?.available.collection_group_sidebar_schema_url;
    if (!base) return '';
    const params = new URLSearchParams({
      entity_type: editorState.response?.entity_type ?? 'model',
    });
    return `${base}?${params.toString()}`;
  }

  function canMoveGroupAt(categoryId: string, itemId: string, delta: -1 | 1): boolean {
    if (!editorState.response) return false;
    const category = editorState.response.categories.find((c) => c.category_id === categoryId);
    if (!category) return false;
    const index = category.items.findIndex((item) => item.id === itemId);
    const nextIndex = index + delta;
    return index >= 0 && nextIndex >= 0 && nextIndex < category.items.length;
  }

  function toViewSections(): ViewSection[] {
    if (!editorState.response) return [];
    return editorState.response.categories.map((category) => ({
      widget: 'category-group',
      id: category.category_id,
      name: category.category_name,
      canonical_order: category.position,
      items: category.items.map((item): ViewItem => ({
        widget: item.widget,
        id: item.id,
        name: item.name,
        field_count: item.field_count,
        shared_path_prefix: item.shared_path_prefix,
        fields: item.fields.map((field): ViewField => ({
          widget: 'field-override',
          override_id: field.override_id,
          field_id: field.field_id,
          field_semantic_id: field.field_id,
          field_system_name: field.field_id,
          origin: { kind: 'own' },
          position: field.position,
          display_name: field.display_name,
          description: field.description,
          expected_value_type: field.expected_value_type,
          set_value: field.set_value,
          is_required: field.is_required,
          is_hidden: field.is_hidden,
          ontology_path: field.ontology_path,
          path_elements: field.path_elements,
          resource_model_refs:
            field.expected_resource_model_refs?.length
              ? field.expected_resource_model_refs.map((ref) => ({ ...ref, semantic_id: ref.semantic_id || ref.id }))
              : (field.expected_resource_models || []).map((id) => ({ id, semantic_id: id, name: { en: id } })),
          collection_model_refs:
            field.expected_collection_model_refs?.length
              ? field.expected_collection_model_refs.map((ref) => ({ ...ref, semantic_id: ref.semantic_id || ref.id }))
              : (field.expected_collection_models || []).map((id) => ({ id, semantic_id: id, name: { en: id } })),
          category_id: field.category_id,
          collection_id: collectionIDForGroup(item),
        })),
      })),
    }));
  }

  const shellState = {
    get expandedCategories() { return expandedCategories; },
    get collapsedCollections() { return collapsedItems; },
    get expandedFields() { return expandedFields; },
    get viewMode() { return viewMode; },
    get projectId() { return editorState.response?.project_id ?? ''; },
    toggleCategory,
    toggleCollection: toggleItem,
    toggleField,
  };

  async function runTextSearch(query = searchQuery.trim()) {
    if (!editorState.response) return;
    const requestId = ++searchRequestId;
    searchLoading = true;
    searchError = null;
    try {
      if (addTab !== 'collection' && addTab !== 'category') {
        const fieldUrl = new URL(editorState.response.available.search_url, window.location.origin);
        fieldUrl.searchParams.set('type', 'field');
        if (query) fieldUrl.searchParams.set('q', query);
        fieldUrl.searchParams.set('target_id', editorState.response.entity_id);
        fieldUrl.searchParams.set('scope', 'inherited');
        const fieldRes = await fetch(fieldUrl);
        if (!fieldRes.ok) throw new Error(`Field search failed: ${fieldRes.status}`);
        const fieldData: OverrideSearchResponse = await fieldRes.json();
        if (requestId !== searchRequestId) return;
        fieldResults = fieldData.items;
      } else {
        fieldResults = [];
      }

      if (supportsCollections() && addTab === 'collection') {
        const collectionUrl = new URL(editorState.response.available.search_url, window.location.origin);
        collectionUrl.searchParams.set('type', 'collection');
        if (query) collectionUrl.searchParams.set('q', query);
        collectionUrl.searchParams.set('scope', 'inherited');
        const collectionRes = await fetch(collectionUrl);
        if (!collectionRes.ok) throw new Error(`Collection search failed: ${collectionRes.status}`);
        const collectionData: OverrideSearchResponse = await collectionRes.json();
        if (requestId !== searchRequestId) return;
        collectionResults = collectionData.items;
      } else {
        collectionResults = [];
      }
    } catch (err) {
      if (requestId !== searchRequestId) return;
      searchError = err instanceof Error ? err.message : String(err);
    } finally {
      if (requestId === searchRequestId) {
        searchLoading = false;
      }
    }
  }

  async function loadPathSuggestions(query = pathQuery.trim()) {
    if (!editorState.response) return;
    if (!query) {
      pathSuggestions = [];
      searchError = null;
      return;
    }
    const requestId = ++suggestionRequestId;
    searchLoading = true;
    searchError = null;
    try {
      const url = new URL(editorState.response.available.path_suggestions_url, window.location.origin);
      url.searchParams.set('current_path', currentPath);
      url.searchParams.set('query', query);
      // scope=inherited so the chain walks parent projects too — without
      // this, a child project (e.g. TRI inheriting from LA) only saw
      // its own fields, which on a fresh project means nothing matches
      // even though the parent uses the qname in 77 places. Matches
      // what the sibling field/collection search calls do.
      url.searchParams.set('scope', 'inherited');
      const res = await fetch(url);
      if (!res.ok) throw new Error(`Path suggestions failed: ${res.status}`);
      const data: OverridePathSuggestionsResponse = await res.json();
      if (requestId !== suggestionRequestId) return;
      pathSuggestions = data.suggestions;
    } catch (err) {
      if (requestId !== suggestionRequestId) return;
      searchError = err instanceof Error ? err.message : String(err);
    } finally {
      if (requestId === suggestionRequestId) {
        searchLoading = false;
      }
    }
  }

  function appendSuggestion(suggestion: OverridePathSuggestion) {
    const part = suggestion.prefix ? `${suggestion.prefix}:${suggestion.local_name}` : suggestion.local_name;
    currentPath = currentPath ? `${currentPath}->${part}` : part;
    pathSuggestions = [];
    pathQuery = '';
    void runPathSearch(currentPath);
  }

  async function runPathSearch(path = currentPath.trim()) {
    if (!editorState.response || !path) return;
    const requestId = ++searchRequestId;
    searchLoading = true;
    searchError = null;
    try {
      const fieldUrl = new URL(editorState.response.available.search_url, window.location.origin);
      fieldUrl.searchParams.set('type', 'field');
      fieldUrl.searchParams.set('path_starts_with', path);
      fieldUrl.searchParams.set('target_id', editorState.response.entity_id);
      fieldUrl.searchParams.set('scope', 'inherited');
      const fieldRes = await fetch(fieldUrl);
      if (!fieldRes.ok) throw new Error(`Field search failed: ${fieldRes.status}`);
      const fieldData: OverrideSearchResponse = await fieldRes.json();
      if (requestId !== searchRequestId) return;
      fieldResults = fieldData.items;

      if (supportsCollections() && addTab === 'collection') {
        const collectionUrl = new URL(editorState.response.available.search_url, window.location.origin);
        collectionUrl.searchParams.set('type', 'collection');
        collectionUrl.searchParams.set('path_starts_with', path);
        collectionUrl.searchParams.set('scope', 'inherited');
        const collectionRes = await fetch(collectionUrl);
        if (!collectionRes.ok) throw new Error(`Collection search failed: ${collectionRes.status}`);
        const collectionData: OverrideSearchResponse = await collectionRes.json();
        if (requestId !== searchRequestId) return;
        collectionResults = collectionData.items;
      } else {
        collectionResults = [];
      }
    } catch (err) {
      if (requestId !== searchRequestId) return;
      searchError = err instanceof Error ? err.message : String(err);
    } finally {
      if (requestId === searchRequestId) {
        searchLoading = false;
      }
    }
  }

  function loadCategories() {
    if (!editorState.response) return;
    const existing = new Set(editorState.response.categories.map((category) => category.category_id));
    categoryResults = [...(editorState.response.available_categories ?? [])]
      .filter((category) => !existing.has(category.id))
      .sort((a, b) => (a.canonical_order ?? 0) - (b.canonical_order ?? 0))
      .map((category) => ({
        id: category.id,
        semantic_id: category.semantic_id,
        ui_name: category.name ?? {},
        canonical_order: category.canonical_order ?? 0,
      }));
  }

  function selectCategory(category: OverrideCategoryOption) {
    editorState.addCategory(category);
    activeCategoryId = category.id;
  }

  async function applySelected() {
    if (!editorState.response || !hasPendingSelection() || !canEditOverrides()) return;
    searchLoading = true;
    searchError = null;
    try {
      for (const category of selectedCategories) {
        editorState.addCategory(category);
      }
      for (const field of selectedFields) {
        editorState.addFieldToCategory(activeCategoryId, field);
      }
      for (const collection of selectedCollections) {
        const adoptURL = editorState.response.available.adopt_collection_url;
        if (!adoptURL) throw new Error('Collection adoption is not available for this editor');
        const res = await fetch(adoptURL, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            collection_id: collection.id,
            category_id: activeCategoryId ?? UNCATEGORIZED_ID,
          }),
        });
        if (!res.ok) throw new Error(`Collection adoption failed: ${res.status}`);
        const adopted: AdoptCollectionResponse = await res.json();
        editorState.insertAdoptedCollection(adopted.category_id, adopted.item);
      }
      closeAdd();
    } catch (err) {
      searchError = err instanceof Error ? err.message : String(err);
    } finally {
      searchLoading = false;
    }
  }

  async function saveDraft() {
    if (!canEditOverrides()) return;
    const payload = editorState.serializePayload();
    if (!payload) return;
    // Captured before the request goes out: tells markSaved() below
    // whether any edit landed while this request was in flight, so a
    // drag/row-edit/sidebar change made during the round trip is never
    // silently erased along with the draft it was never part of sending.
    const sinceVersion = editorState.version;
    saving = true;
    saveError = null;
    saveConflictKind = null;
    clearSaveInProgressTimer();
    try {
      const res = await fetch(saveUrl, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          commit_message: 'Update overrides',
          // Never an empty string here — that is the ops-tooling/git-restore
          // opt-out sentinel the server treats as "skip the staleness
          // check", and the editor is never allowed to opt out (see
          // OverrideEditorState.fingerprintForSave / LEGACY_DRAFT_FINGERPRINT).
          fingerprint: editorState.fingerprintForSave,
          ...payload,
        }),
      });
      const body = await res.json().catch(() => null);
      if (!res.ok) {
        // The two 409s are different situations: branch on the `message`
        // field (apierror.Error.Details), never on the status code alone.
        if (res.status === 409 && body?.code === 'conflict' && body?.message === 'save_in_progress') {
          saveConflictKind = 'save_in_progress';
          saveInProgressTimer = setTimeout(() => {
            if (saveConflictKind === 'save_in_progress') saveConflictKind = null;
            saveInProgressTimer = null;
          }, SAVE_IN_PROGRESS_NOTICE_MS);
          return;
        }
        if (res.status === 409 && body?.code === 'conflict' && body?.message === 'pattern_changed') {
          saveConflictKind = 'pattern_changed';
          return;
        }
        throw new Error(body?.error || `Save failed: ${res.status}`);
      }
      const fingerprint = typeof body?.fingerprint === 'string' && body.fingerprint.length > 0 ? body.fingerprint : null;
      if (fingerprint) {
        // Only a 2xx response carries a real fingerprint — adopt it as
        // the new baseline before anything else can trigger another save.
        editorState.fingerprint = fingerprint;
        dismissedStaleDraft = false;
        const editedWhileSaving = sinceVersion !== editorState.version;
        editorState.markSaved(sinceVersion);
        if (editedWhileSaving) {
          // onsuccess() below leaves edit mode and unmounts this whole
          // component in the same tick, so a component-local flag or
          // banner saying "your in-flight edit is still unsaved" would
          // be torn down before it ever painted. A toast survives that
          // unmount (fix round 2, item 3) — the save went through, but
          // the draft markSaved just kept dirty above still needs to be
          // said out loud, or the curator has no reason to reopen the
          // editor and finish sending it.
          addToast('info', 'Saved. The changes you made while it was saving are still unsaved.');
        }
      } else {
        // The write went through (2xx) but the response carried no
        // usable fingerprint — this editor's copy of "what version this
        // now is" can no longer be trusted. Continuing with the OLD
        // fingerprint would make the curator's own next save look like a
        // conflict with itself, and the only recovery the conflict bar
        // offers is discarding everything typed since. Reload from the
        // server instead of guessing; markSaved(sinceVersion) first so a
        // draft with no edits since the snapshot doesn't get restored
        // over the reload (an in-flight edit, if any, is kept and will
        // correctly show as a stale draft once the reload lands).
        editorState.markSaved(sinceVersion);
        await editorState.init();
        // Same unmount-before-paint problem as above (fix round 2, item
        // 4) — a saveError here would flash at best. Toasting it costs
        // one line; this branch stays otherwise exactly as cheap as
        // before, since it is unreachable against today's server
        // (override_write.go always sets a fingerprint on a 2xx).
        addToast('warning', "Saved, but the server didn't confirm the new version — reloaded to stay in sync.");
      }
      await onsuccess?.();
    } catch (err) {
      saveError = err instanceof Error ? err.message : String(err);
    } finally {
      saving = false;
    }
  }
</script>

{#if editorState.loading && !editorState.response}
  <LoadingState message="Loading override editor..." />
{:else if editorState.error && !editorState.response}
  <div class="bg-red-50 border border-red-200 rounded-lg p-6 text-center">
    <h3 class="text-sm font-medium text-red-800">Failed to load override editor</h3>
    <p class="mt-1 text-sm text-red-600">{editorState.error}</p>
    <button
      type="button"
      class="mt-4 inline-flex items-center px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-red-600 hover:bg-red-700"
      onclick={() => { saveError = null; void editorState.init(); }}
    >
      Retry
    </button>
  </div>
{:else if editorState.response}
  <div class="space-y-6 rounded-xl border border-blue-200 bg-blue-50/30 p-4 md:p-6">
    {#if isTransientSaveNotice()}
      <div role="status" aria-live="polite" class="rounded-lg border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800">
        {conflictBannerText()}
      </div>
    {/if}

    {#if isActionableConflict()}
      <div role="status" aria-live="polite" class="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 flex items-center justify-between gap-4">
        <div class="text-sm text-amber-800">{conflictBannerText()}</div>
        <div class="flex items-center gap-2 shrink-0">
          <button
            type="button"
            class="inline-flex items-center px-3 py-1.5 text-sm font-medium rounded-md border border-amber-300 bg-white text-amber-800 hover:bg-amber-100"
            onclick={discardDraftAndReload}
          >
            Discard my changes and reload
          </button>
          <button
            type="button"
            class="inline-flex items-center px-3 py-1.5 text-sm font-medium rounded-md border border-blue-600 bg-blue-600 text-white hover:bg-blue-700"
            onclick={keepEditingMine}
          >
            Keep editing mine
          </button>
        </div>
      </div>
    {/if}

    {#if othersEditing.length > 0}
      <div role="status" aria-live="polite" class="rounded-lg border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800">
        {presenceLineText()}
      </div>
    {/if}

    <div class="flex items-center justify-between">
      <div>
        <div class="flex items-center gap-2">
          <h2 class="text-xl font-semibold text-gray-900">Edit Composition</h2>
          <Badge variant="purple" size="xs">{editorState.response.scope.display}</Badge>
        </div>
        <p class="mt-1 text-sm text-gray-600">
          Compose existing fields and collections into this {editorState.response.entity_type} context.
        </p>
      </div>
      <div class="flex items-center gap-2">
        <div class="flex items-center gap-1 bg-white rounded p-0.5 border border-blue-100">
          <button
            type="button"
            class="px-2.5 py-1 text-xs rounded transition-colors
              {viewMode === 'compact' ? 'bg-blue-600 text-white shadow-sm' : 'text-gray-600 hover:text-gray-800'}"
            onclick={() => { viewMode = 'compact'; }}
          >
            Compact
          </button>
          <button
            type="button"
            class="px-2.5 py-1 text-xs rounded transition-colors
              {viewMode === 'detailed' ? 'bg-blue-600 text-white shadow-sm' : 'text-gray-600 hover:text-gray-800'}"
            onclick={() => { viewMode = 'detailed'; }}
          >
            Detailed
          </button>
        </div>
        <button
          type="button"
          class="inline-flex items-center px-3 py-1.5 text-sm font-medium rounded-md border border-blue-100 bg-white hover:bg-blue-50"
          onclick={expandAllItems}
        >
          Expand All
        </button>
        <button
          type="button"
          class="inline-flex items-center px-3 py-1.5 text-sm font-medium rounded-md border border-blue-100 bg-white hover:bg-blue-50"
          onclick={collapseAllItems}
        >
          Collapse All
        </button>
        {#if onback}
          <button
            type="button"
            class="inline-flex items-center px-3 py-1.5 text-sm font-medium rounded-md border border-gray-300 bg-white hover:bg-gray-50"
            onclick={onback}
          >
            Back
          </button>
        {/if}
        {#if canOpenAdd()}
          <button
            type="button"
            class="inline-flex items-center px-3 py-1.5 text-sm font-medium rounded-md border border-sky-300 bg-sky-50 text-sky-800 hover:bg-sky-100"
            onclick={openGlobalAdd}
          >
            Add
          </button>
        {/if}
        {#if canEditOverrides()}
          <button
            type="button"
            class="inline-flex items-center px-3 py-1.5 text-sm font-medium rounded-md border border-blue-600 bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
            onclick={saveDraft}
            disabled={!editorState.dirty || saving || discarding}
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
        {/if}
      </div>
    </div>

    {#if saveError}
      <div role="status" aria-live="polite" class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
        {saveError}
      </div>
    {/if}

    {#if editorState.dirty}
      <div class="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 flex items-center justify-between gap-4">
        <div class="text-sm text-amber-800">
          {#if editorState.restoredDraft}
            Restored an unsaved local draft for this composition editor.
          {:else}
            You have unsaved composition changes in local draft state.
          {/if}
        </div>
        {#if canEditOverrides()}
          <button
            type="button"
            class="inline-flex items-center px-3 py-1.5 text-sm font-medium rounded-md border border-amber-300 bg-white text-amber-800 hover:bg-amber-100"
            onclick={discardDraftAndReload}
          >
            Discard Draft
          </button>
        {/if}
      </div>
    {:else}
      <div class="rounded-lg border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800">
        {#if canEditOverrides()}
          Adopt and edit composition here. Changes are kept locally until you save.
        {:else}
          Composition is read-only in this context. You can inspect the structure, but editing actions are disabled.
        {/if}
      </div>
    {/if}

    <!-- Inline editable tree: categories → groups → editable rows.
         Each EditableGroup owns its dnd-action items state to avoid
         lifecycle bugs. Sidebar opens on row click. -->
    {#each editorState.response.categories as category (category.category_id)}
      {@const isEmptyCategory =
        editorState.response.entity_type === 'collection' &&
        category.category_id === UNCATEGORIZED_ID}
      <section class="space-y-3">
        {#if !isEmptyCategory}
          <header class="flex items-baseline gap-3 flex-wrap">
            <h2 class="text-base font-semibold text-gray-900">{tr(category.category_name, lang, category.semantic_id || 'Unnamed category')}</h2>
            {#if category.semantic_id}
              <span class="text-xs text-gray-400 font-mono" title={category.category_id}>{category.semantic_id}</span>
            {/if}
            <span class="flex-1"></span>
            {#if canOpenAdd()}
              <button
                type="button"
                class="text-xs text-pletka-primary hover:underline"
                onclick={() => openAdd(category.category_id)}
              >+ Add</button>
            {/if}
          </header>
        {/if}

        {#each category.items as item (item.id)}
          <EditableGroup
            categoryId={category.category_id}
            {item}
            editor={editorState}
            canEdit={canEditOverrides()}
            canHide={canHideFields()}
            canReorder={editorState.response.capabilities.can_reorder}
            activeOverrideID={activeRowKey?.overrideID ?? null}
            activeCollectionId={activeCollectionKey?.itemId ?? null}
            {lang}
            categories={editorState.response.categories}
            availableCategories={editorState.response.available_categories ?? []}
            entityType={editorState.response.entity_type}
            {viewState}
            oneditrow={(id) => openRowEditor(category.category_id, item.id, id)}
            oneditcollection={openCollectionEditor}
          />
        {/each}
      </section>
    {/each}
  </div>

  <!-- Deep-edit sidebar — field mode -->
  {#if activeRowKey}
    {@const r = activeRow()}
    {#if r}
      <OverrideSidebar
        categoryId={r.categoryId}
        itemId={r.itemId}
        field={r.field}
        editor={editorState}
        schemaUrl={fieldSidebarSchemaUrl(r.inCollectionGroup, r.field.expected_value_type)}
        inCollectionGroup={r.inCollectionGroup}
        entityType={editorState.response.entity_type}
        {lang}
        onclose={closeRowEditor}
      />
    {/if}
  {/if}

  <!-- Deep-edit sidebar — collection mode -->
  {#if activeCollectionKey}
    {@const c = activeCollection()}
    {#if c}
      <CollectionSidebar
        categoryId={c.categoryId}
        item={c.item}
        editor={editorState}
        schemaUrl={collectionSidebarSchemaUrl()}
        {lang}
        onclose={closeCollectionEditor}
      />
    {/if}
  {/if}
{/if}

  {#if editorState.response && addModalOpen && canEditOverrides()}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-gray-950/45 p-6">
    <div class="flex w-full max-w-5xl max-h-[85vh] flex-col overflow-hidden rounded-2xl border border-blue-100 bg-white shadow-2xl">
      <div class="flex items-start justify-between gap-4 border-b border-gray-100 bg-blue-50 px-6 py-4">
        <div>
          <div class="text-sm font-semibold text-blue-900">
            {#if activeCategoryId === UNCATEGORIZED_ID}
              Add to composition
            {:else if activeCategoryName()}
              Add to {activeCategoryName()}
            {:else}
              Add to composition
            {/if}
          </div>
          <div class="mt-1 text-sm text-blue-700">
            Browse existing fields, collections, or categories and add them into this composition.
          </div>
        </div>
        <button
          type="button"
          class="inline-flex items-center px-3 py-1.5 text-sm font-medium rounded-md border border-blue-200 bg-white text-blue-700 hover:bg-blue-100"
          onclick={closeAdd}
        >
          Close
        </button>
      </div>

      <div class="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden px-6 py-5">
        <div class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            class="inline-flex items-center px-3 py-2 text-sm font-medium rounded-md border transition-colors {addTab === 'field' ? 'border-blue-600 bg-blue-600 text-white' : 'border-blue-200 bg-white text-blue-700 hover:bg-blue-50'}"
            onclick={() => chooseAddTab('field')}
          >
            Fields
          </button>
          {#if supportsCollections()}
            <button
              type="button"
              class="inline-flex items-center px-3 py-2 text-sm font-medium rounded-md border transition-colors {addTab === 'collection' ? 'border-blue-600 bg-blue-600 text-white' : 'border-blue-200 bg-white text-blue-700 hover:bg-blue-50'}"
              onclick={() => chooseAddTab('collection')}
            >
              Collections
            </button>
          {/if}
          <button
            type="button"
            class="inline-flex items-center px-3 py-2 text-sm font-medium rounded-md border transition-colors {addTab === 'category' ? 'border-blue-600 bg-blue-600 text-white' : 'border-blue-200 bg-white text-blue-700 hover:bg-blue-50'}"
            onclick={() => chooseAddTab('category')}
          >
            Categories
          </button>
        </div>

        {#if addTab !== 'category'}
          <label class="flex items-center gap-2 text-sm text-gray-700">
            <input type="checkbox" bind:checked={usePathSearch} class="rounded border-blue-300 text-blue-600 focus:ring-blue-500" />
            Path search
          </label>
        {/if}

        <div class="space-y-2">
          <div class="flex items-center justify-between gap-4">
            <div class="text-xs font-semibold uppercase tracking-wide text-gray-500">
              Selected
              {#if hasPendingSelection()}
                <span class="ml-1 normal-case tracking-normal text-gray-400">
                  ({selectedCategories.length + selectedFields.length + selectedCollections.length})
                </span>
              {/if}
            </div>
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="inline-flex items-center px-3 py-1.5 text-sm font-medium rounded-md border border-gray-300 bg-white text-gray-700 hover:bg-gray-50"
                onclick={closeAdd}
              >
                Cancel
              </button>
              <button
                type="button"
                class="inline-flex items-center px-3 py-1.5 text-sm font-medium rounded-md border border-blue-600 bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
                onclick={applySelected}
                disabled={!hasPendingSelection() || searchLoading}
              >
                Apply Selected
              </button>
            </div>
          </div>
          <div class="rounded border border-blue-100 bg-blue-50/50 px-3 py-2 min-h-[3rem]">
            {#if !hasPendingSelection()}
              <div class="text-sm text-gray-500">No items selected yet.</div>
            {:else}
              <div class="flex flex-wrap gap-2">
                {#each selectedCategories as category (category.id)}
                  <button
                    type="button"
                    class="inline-flex items-center gap-2 rounded-full bg-amber-100 px-3 py-1 text-xs font-medium text-amber-900"
                    onclick={() => removeSelectedChip('category', category.id)}
                  >
                    <span class="font-mono">{category.semantic_id || category.id}</span>
                    <span>{tr(category.ui_name, lang, category.id)}</span>
                    <span class="text-amber-700">&times;</span>
                  </button>
                {/each}
                {#each selectedFields as result (result.id)}
                  <button
                    type="button"
                    class="inline-flex items-center gap-2 rounded-full bg-blue-100 px-3 py-1 text-xs font-medium text-blue-900"
                    onclick={() => removeSelectedChip('field', result.id)}
                  >
                    <span class="font-mono">{result.semantic_id || result.id}</span>
                    <span>{tr(result.ui_name, lang, result.id)}</span>
                    <span class="text-blue-700">&times;</span>
                  </button>
                {/each}
                {#each selectedCollections as result (result.id)}
                  <button
                    type="button"
                    class="inline-flex items-center gap-2 rounded-full bg-green-100 px-3 py-1 text-xs font-medium text-green-900"
                    onclick={() => removeSelectedChip('collection', result.id)}
                  >
                    <span class="font-mono">{result.semantic_id || result.id}</span>
                    <span>{tr(result.ui_name, lang, result.id)}</span>
                    <span class="text-green-700">&times;</span>
                  </button>
                {/each}
              </div>
            {/if}
          </div>
        </div>

        {#if addTab !== 'category' && !usePathSearch}
          <div class="flex items-center gap-2">
            <input
              type="text"
              bind:value={searchQuery}
              placeholder={addTab === 'field' ? 'Browse or search fields…' : 'Browse or search collections…'}
              class="flex-1 px-3 py-2 text-sm border border-blue-200 rounded-md bg-white"
              onkeydown={(e) => { if (e.key === 'Enter') void runTextSearch(); }}
            />
            <button
              type="button"
              class="px-3 py-2 text-sm font-medium rounded-md bg-blue-600 text-white hover:bg-blue-700"
              onclick={() => runTextSearch()}
            >
              Search
            </button>
          </div>
        {:else if addTab !== 'category'}
          <div class="space-y-2">
            <div class="flex items-center gap-2">
              <input
                type="text"
                bind:value={pathQuery}
                placeholder="Search next path element..."
                class="flex-1 px-3 py-2 text-sm border border-blue-200 rounded-md bg-white"
                onkeydown={(e) => { if (e.key === 'Enter') void loadPathSuggestions(); }}
              />
              <button
                type="button"
                class="px-3 py-2 text-sm font-medium rounded-md bg-blue-600 text-white hover:bg-blue-700"
                onclick={() => loadPathSuggestions()}
              >
                Suggest
              </button>
              <button
                type="button"
                class="px-3 py-2 text-sm font-medium rounded-md border border-blue-200 bg-white text-blue-700 hover:bg-blue-100"
                onclick={() => runPathSearch()}
                disabled={!currentPath}
              >
                Search
              </button>
            </div>
            {#if currentPath}
              <div class="text-xs text-blue-800">
                Current path: <code class="rounded bg-white px-1.5 py-0.5">{currentPath}</code>
              </div>
            {/if}
            {#if pathSuggestions.length}
              <div class="flex flex-wrap gap-2">
                {#each pathSuggestions as suggestion}
                  <button
                    type="button"
                    class="inline-flex items-center px-2 py-1 text-xs rounded border border-blue-200 bg-white text-blue-700 hover:bg-blue-100"
                    onclick={() => appendSuggestion(suggestion)}
                  >
                    {suggestion.display}
                    <span class="ml-1 text-blue-400">({suggestion.field_count})</span>
                  </button>
                {/each}
              </div>
            {/if}
          </div>
        {/if}

        {#if searchError}
          <div class="text-sm text-red-700">{searchError}</div>
        {/if}

        {#if searchLoading}
          <div class="text-sm text-blue-700">Loading…</div>
        {/if}

        {#if addTab === 'field'}
          <div class="flex min-h-0 flex-1 flex-col space-y-2">
            <div class="text-xs font-semibold uppercase tracking-wide text-gray-500">Fields</div>
            <div class="flex-1 min-h-0 rounded border border-blue-100 bg-white divide-y divide-gray-100 overflow-y-auto">
              {#if fieldResults.length === 0}
                <div class="px-4 py-4 text-sm text-gray-500">No field results</div>
              {:else}
                {#each fieldResults as result (result.id)}
                  <div class="px-4 py-3 flex items-start justify-between gap-4">
                    <div class="min-w-0 space-y-1">
                      <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
                        <div class="font-medium text-gray-900">{tr(result.ui_name, lang, result.id)}</div>
                        <div class="text-xs font-mono text-gray-500">{result.semantic_id || result.id}</div>
                        {#if result.origin?.kind === 'inherited' || result.origin?.kind === 'adopted'}
                          <Badge variant={result.origin?.kind === 'adopted' ? 'blue' : 'amber'} size="xs">{originBadgeText(result.origin)}</Badge>
                        {/if}
                      </div>
                      <div class="text-xs text-gray-600 flex flex-wrap items-center gap-x-3 gap-y-1">
                        {#if result.ontology_scope}<span>Scope: {scopeLabel(result.ontology_scope)}</span>{/if}
                        {#if result.expected_value_type}<span>Type: {result.expected_value_type}</span>{/if}
                      </div>
                      {#if result.path_elements?.length}
                        <div class="text-xs text-gray-500 break-words">
                          <PathPreview path={pathElementsToDisplay(result.path_elements)} />
                        </div>
                      {:else if result.ontology_path}
                        <div class="text-xs text-gray-500 break-words">{result.ontology_path}</div>
                      {/if}
                      {#if (result.expected_resource_models?.length || 0) > 0 || (result.expected_collection_models?.length || 0) > 0}
                        <div class="text-xs text-gray-500 break-words">
                          {#if (result.expected_resource_models?.length || 0) > 0}
                            <span>Models: {(result.expected_resource_models || []).join(', ')}</span>
                          {/if}
                          {#if (result.expected_resource_models?.length || 0) > 0 && (result.expected_collection_models?.length || 0) > 0}
                            <span class="mx-1">•</span>
                          {/if}
                          {#if (result.expected_collection_models?.length || 0) > 0}
                            <span>Collections: {(result.expected_collection_models || []).join(', ')}</span>
                          {/if}
                        </div>
                      {/if}
                    </div>
                    <button
                      type="button"
                      class="shrink-0 px-2.5 py-1 text-xs font-medium rounded border {result.is_linked ? 'border-gray-200 bg-gray-100 text-gray-400' : isFieldSelected(result.id) ? 'border-blue-600 bg-blue-600 text-white' : 'border-blue-200 bg-blue-50 text-blue-700 hover:bg-blue-100'}"
                      onclick={() => toggleFieldSelection(result)}
                      disabled={result.is_linked}
                    >
                      {fieldActionLabel(result)}
                    </button>
                  </div>
                {/each}
              {/if}
            </div>
          </div>
        {:else if addTab === 'collection'}
          <div class="flex min-h-0 flex-1 flex-col space-y-2">
            <div class="text-xs font-semibold uppercase tracking-wide text-gray-500">Collections</div>
            <div class="flex-1 min-h-0 rounded border border-blue-100 bg-white divide-y divide-gray-100 overflow-y-auto">
              {#if collectionResults.length === 0}
                <div class="px-4 py-4 text-sm text-gray-500">No collection results</div>
              {:else}
                {#each collectionResults as result (result.id)}
                  <div class="px-4 py-3 flex items-start justify-between gap-4">
                    <div class="min-w-0 space-y-1">
                      <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
                        <div class="font-medium text-gray-900">{tr(result.ui_name, lang, result.id)}</div>
                        <div class="text-xs font-mono text-gray-500">{result.semantic_id || result.id}</div>
                        {#if result.origin?.kind === 'inherited' || result.origin?.kind === 'adopted'}
                          <Badge variant={result.origin?.kind === 'adopted' ? 'blue' : 'amber'} size="xs">{originBadgeText(result.origin)}</Badge>
                        {/if}
                      </div>
                      <div class="text-xs text-gray-600 flex flex-wrap items-center gap-x-3 gap-y-1">
                        {#if result.ontology_scope}<span>Scope: {scopeLabel(result.ontology_scope)}</span>{/if}
                        <span>Used: {result.adoption_count}</span>
                      </div>
                    </div>
                    <button
                      type="button"
                      class="shrink-0 px-2.5 py-1 text-xs font-medium rounded border {isCollectionSelected(result.id) ? 'border-green-600 bg-green-600 text-white' : 'border-blue-200 bg-blue-50 text-blue-700 hover:bg-blue-100'}"
                      onclick={() => toggleCollectionSelection(result)}
                    >
                      {collectionActionLabel(result)}
                    </button>
                  </div>
                {/each}
              {/if}
            </div>
          </div>
        {:else if addTab === 'category'}
          <div class="flex min-h-0 flex-1 flex-col space-y-2">
            <div class="text-xs font-semibold uppercase tracking-wide text-gray-500">Categories</div>
            <div class="flex-1 min-h-0 rounded border border-blue-100 bg-white divide-y divide-gray-100 overflow-y-auto">
              {#if categoryResults.length === 0}
                <div class="px-4 py-4 text-sm text-gray-500">No categories available</div>
              {:else}
                {#each categoryResults as category (category.id)}
                  <div class="px-4 py-3 flex items-start justify-between gap-4">
                    <div class="min-w-0">
                      <div class="font-medium text-gray-900">{tr(category.ui_name, lang, category.id)}</div>
                      <div class="mt-0.5 text-xs font-mono text-gray-500">{category.semantic_id || category.id}</div>
                    </div>
                    <button
                      type="button"
                      class="shrink-0 px-2.5 py-1 text-xs font-medium rounded border {isCategorySelected(category.id) ? 'border-amber-600 bg-amber-600 text-white' : 'border-blue-200 bg-blue-50 text-blue-700 hover:bg-blue-100'}"
                      onclick={() => toggleCategorySelection(category)}
                    >
                      {isCategorySelected(category.id) ? 'Remove' : 'Add'}
                    </button>
                  </div>
                {/each}
              {/if}
            </div>
          </div>
        {/if}

        <div class="flex items-center justify-between gap-4 border-t border-gray-100 pt-4">
          <div class="text-sm text-gray-500">
            {#if !activeCategoryId || activeCategoryId === UNCATEGORIZED_ID}
              Selected fields will be added to <span class="font-medium text-gray-700">Uncategorized</span>; collections are placed in their default category.
            {:else if activeCategoryName()}
              Selected fields and collections will be added to <span class="font-medium text-gray-700">{activeCategoryName()}</span>.
            {:else}
              Selected items will be applied to the composition.
            {/if}
          </div>
          <div class="text-sm text-gray-400">Apply from the selected drawer above.</div>
        </div>
      </div>
    </div>
  </div>
{/if}
