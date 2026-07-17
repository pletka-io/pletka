// frontend/src/lib/detailview/state.svelte.ts

import type { EntityViewResponse, FieldReuseResponse, ModelViewStats } from '$lib/detailview/types';
import { tr } from '$lib/types/weave-types';
import { filterFields } from '$lib/detailview/field-filter';

export type ViewMode = 'compact' | 'detailed';
export type TabId = 'fields' | 'entries' | 'stats' | 'metadata' | 'examples' | 'derivatives' | 'reuse';

const STORAGE_PREFIX = 'entity-view:';

interface PersistedUIState {
  expandedCategories: string[];
  collapsedCollections: string[];
  viewMode: ViewMode;
  activeTab: TabId;
}

/** Lightweight state for read-only entity views. */
export class EntityViewState {
  // API data (stored as-is, no transformation)
  response: EntityViewResponse | null = $state(null);
  stats: ModelViewStats | null = $state(null);
  statsLoading = $state(false);
  reuse: FieldReuseResponse | null = $state(null);
  reuseLoading = $state(false);

  // Props
  projectId: string;
  entityType: string;
  entityId: string;
  routeBase: string;
  releaseVersion: string;

  // UI state
  expandedCategories: Set<string> = $state(new Set());
  collapsedCollections: Set<string> = $state(new Set());
  expandedFields: Set<string> = $state(new Set());
  viewMode: ViewMode = $state('compact');

  // Tab state
  activeTab: TabId = $state<TabId>('fields');

  // Field search state. matchSet + matchOrder are pure $derived from
  // searchQuery + the in-memory section tree — no DOM querying, no
  // setTimeout dance. searchIndex tracks the active match for next/prev
  // nav and the highlight class applied via a `class:` directive.
  searchQuery = $state('');
  searchIndex = $state(0);

  // External category source. When the user opens the composition
  // editor, OverrideEditor calls setCategoriesSource(() =>
  // editor.response.categories) — the sticky-header search now
  // filters the draft tree instead of the read-only sections. On
  // close, clearCategoriesSource() restores the default. The fn is
  // wrapped in $state so the $derived below tracks reassignment.
  private categoriesSource = $state<() => unknown[]>(() => this.sections);

  setCategoriesSource(fn: () => unknown[]): void {
    this.categoriesSource = fn;
  }

  clearCategoriesSource(): void {
    this.categoriesSource = () => this.sections;
  }

  // One filter call per (searchQuery, source) change; matchSet and
  // matchOrder pluck off the same result to avoid recomputing the walk
  // twice per keystroke.
  // deno-lint-ignore no-explicit-any
  private filterResult = $derived(
    filterFields(this.searchQuery, this.categoriesSource() as any),
  );
  matchSet = $derived(this.filterResult.set);
  matchOrder = $derived(this.filterResult.order);
  activeOverrideID = $derived(
    this.matchOrder.length > 0 && this.searchIndex < this.matchOrder.length
      ? this.matchOrder[this.searchIndex]
      : null,
  );

  // Scroll context
  headerCompacted = $state(false);
  scrollCategoryName = $state('');
  scrollCollectionName = $state('');

  // Loading
  loading: boolean = $state(false);
  error: string | null = $state(null);

  constructor(projectId: string, entityType: string, entityId: string, routeBase = '/projects') {
    this.projectId = projectId;
    this.entityType = entityType;
    this.entityId = entityId;
    this.routeBase = routeBase;
    this.activeTab = this.defaultTab();
    this.releaseVersion = typeof window === 'undefined'
      ? ''
      : new URLSearchParams(window.location.search).get('version') || '';
  }

  private get sections() {
    return this.response?.sections ?? [];
  }

  /** Fetch from EntityView API and restore persisted UI state. */
  async init(): Promise<void> {
    this.loading = true;
    this.error = null;

    try {
      const url = this.withVersion(`${this.routeBase}/${this.projectId}/entity-view/${this.entityType}/${this.entityId}`);
      const res = await fetch(url);

      if (!res.ok) {
        throw new Error(`Failed to load entity view: ${res.status} ${res.statusText}`);
      }

      this.response = await res.json();
      this.restoreUIState();

      // If no persisted state, expand all categories by default.
      if (this.expandedCategories.size === 0 && this.response) {
        for (const section of this.sections) {
          this.expandedCategories.add(section.id);
        }
      }
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err);
    } finally {
      this.loading = false;
    }
  }

  toggleCategory(id: string): void {
    if (this.expandedCategories.has(id)) {
      this.expandedCategories.delete(id);
    } else {
      this.expandedCategories.add(id);
    }
    // Trigger reactivity by reassigning.
    this.expandedCategories = new Set(this.expandedCategories);
    this.persistUIState();
  }

  toggleCollection(key: string): void {
    if (this.collapsedCollections.has(key)) {
      this.collapsedCollections.delete(key);
    } else {
      this.collapsedCollections.add(key);
    }
    this.collapsedCollections = new Set(this.collapsedCollections);
    this.persistUIState();
  }

  toggleField(id: string): void {
    if (this.expandedFields.has(id)) {
      this.expandedFields.delete(id);
    } else {
      this.expandedFields.add(id);
    }
    this.expandedFields = new Set(this.expandedFields);
  }

  expandAllCategories(): void {
    if (!this.response) return;
    this.expandedCategories = new Set(this.sections.map((s) => s.id));
    this.persistUIState();
  }

  collapseAllCategories(): void {
    this.expandedCategories = new Set();
    this.persistUIState();
  }

  setViewMode(mode: ViewMode): void {
    this.viewMode = mode;
    this.persistUIState();
  }

  get totalFieldCount(): number {
    if (this.stats) return this.stats.total_fields;
    if (this.sections.length === 0) return 0;
    // Count from sections when stats haven't been loaded yet.
    let count = 0;
    for (const s of this.sections) {
      for (const item of s.items) {
        count += item.fields.length;
      }
    }
    return count;
  }

  setTab(tab: TabId): void {
    this.activeTab = tab;
    this.persistUIState();
  }

  private defaultTab(): TabId {
    return this.entityType === 'concept-list' ? 'entries' : 'fields';
  }

  /** Lazily fetch stats from the stats endpoint. Only fetches once. */
  async loadStats(): Promise<void> {
    if (this.stats || this.statsLoading || !this.response?.stats_url) return;
    this.statsLoading = true;
    try {
      const res = await fetch(this.withVersion(this.response.stats_url));
      if (!res.ok) throw new Error(`Stats fetch failed: ${res.status}`);
      this.stats = await res.json();
    } catch (err) {
      console.error('Failed to load stats:', err);
    } finally {
      this.statsLoading = false;
    }
  }

  /** Lazily fetch the reuse list (Reuse tab on field item view).
   *  Reads the URL from capabilities.reuse_url so the frontend
   *  never constructs the URL itself (api-patterns.md). One fetch per
   *  view lifetime. */
  async loadReuse(): Promise<void> {
    const url = this.response?.capabilities?.reuse_url;
    if (this.reuse || this.reuseLoading || !url) return;
    this.reuseLoading = true;
    try {
      const res = await fetch(this.withVersion(url));
      if (!res.ok) throw new Error(`Reuse fetch failed: ${res.status}`);
      this.reuse = await res.json();
    } catch (err) {
      console.error('Failed to load reuse:', err);
    } finally {
      this.reuseLoading = false;
    }
  }

  /** Reset the active match index. Bind the input directly to
   *  `searchQuery` — the derived match set/order recomputes from there
   *  without explicit imperative call. Kept as a method so the existing
   *  template `oninput` handler still works. Also expands every
   *  category + collection imperatively (not via $effect) when search
   *  is active so matches are visible without nesting tracker reads
   *  inside an effect that would create a feedback loop. */
  fieldSearchUpdate(): void {
    this.searchIndex = 0;
    if (this.searchQuery.trim() && this.response) {
      this.expandedCategories = new Set(this.sections.map((s) => s.id));
      this.collapsedCollections = new Set();
    }
  }

  fieldSearchNav(direction: number): void {
    const len = this.matchOrder.length;
    if (len === 0) return;
    this.searchIndex = (this.searchIndex + direction + len) % len;
    this.scrollToActive();
  }

  fieldSearchClear(): void {
    this.searchQuery = '';
    this.searchIndex = 0;
  }

  /** Smooth-scroll the active match into view by querying the row's
   *  data-override-id attribute. Single DOM read, no class mutation. */
  private scrollToActive(): void {
    const id = this.activeOverrideID;
    if (id == null) return;
    const el = document.querySelector<HTMLElement>(`[data-override-id="${id}"]`);
    el?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  }

  private withVersion(raw: string): string {
    if (!this.releaseVersion || !raw) return raw;
    try {
      const url = new URL(raw, window.location.origin);
      url.searchParams.set('version', this.releaseVersion);
      return url.toString();
    } catch {
      return raw;
    }
  }

  /** Resolve a display name from the refs maps. */
  resolveRefName(id: string, type: 'model' | 'collection' | 'category', lang: string): string {
    if (!this.response?.refs) return id;
    const refsMap =
      type === 'model'
        ? this.response.refs.models
        : type === 'collection'
          ? this.response.refs.collections
          : this.response.refs.categories;
    const entry = refsMap?.[id];
    if (!entry) return id;
    return tr(entry.name, lang, id);
  }

  // --- localStorage persistence ---

  private get storageKey(): string {
    return `${STORAGE_PREFIX}${this.entityId}`;
  }

  private persistUIState(): void {
    try {
      const state: PersistedUIState = {
        expandedCategories: [...this.expandedCategories],
        collapsedCollections: [...this.collapsedCollections],
        viewMode: this.viewMode,
        activeTab: this.activeTab,
      };
      localStorage.setItem(this.storageKey, JSON.stringify(state));
    } catch {
      // localStorage may be unavailable; silently ignore.
    }
  }

  private restoreUIState(): void {
    try {
      const raw = localStorage.getItem(this.storageKey);
      if (!raw) return;
      const state: PersistedUIState = JSON.parse(raw);
      this.expandedCategories = new Set(state.expandedCategories ?? []);
      this.collapsedCollections = new Set(state.collapsedCollections ?? []);
      this.viewMode = state.viewMode ?? 'compact';
      // activeTab is intentionally NOT restored:
      // every visit to an entity opens on the default tab so curators
      // get a predictable starting state. Other UI state (expanded
      // categories, collapsed collections, view mode) is still
      // remembered because that's about workspace layout, not "where
      // was I last."
      this.activeTab = this.defaultTab();
    } catch {
      // Corrupt data; start fresh.
    }
  }
}
