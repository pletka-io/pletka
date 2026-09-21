// frontend/src/lib/stores/project-detail.svelte.ts

import type { ProjectPageSchema, ProjectPageTab } from '$lib/types/project-page';

/**
 * ProjectDetailState manages the project detail page:
 * - Fetches the page schema (tabs + children, header)
 * - Tracks the active tab (always a LEAF tab id when there are children)
 * - Lazily fetches and caches content per leaf tab for schema-driven tabs
 *   (e.g. overview). Entity-list tabs are rendered by EntityListView,
 *   which fetches its own schema from tab.content_url — for those tabs
 *   we do not cache content here.
 * - Syncs active tab with URL hash (#tab=<leafId>) for deep links
 * - Persists last-visited child per parent in localStorage so clicking
 *   a parent header returns to the most recently used sub-tab
 */
export class ProjectDetailState {
  projectId: string;
  schemaUrl: string;
  entityLabel: string;

  pageSchema: ProjectPageSchema | null = $state(null);
  activeTabId: string = $state('');
  /** Bumped when the active tab is re-clicked so the page remounts its list. */
  listReset = $state(0);
  tabContent: Map<string, unknown> = $state(new Map());

  loading: boolean = $state(false);
  error: string | null = $state(null);

  tabLoading: Map<string, boolean> = $state(new Map());
  tabErrors: Map<string, string> = $state(new Map());

  constructor(projectId: string, schemaUrl?: string, entityLabel = 'project') {
    this.projectId = projectId;
    const defaultSchemaURL = `/projects/${this.projectId}/page-schema`;
    this.schemaUrl = schemaUrl ?? this.withCurrentVersion(defaultSchemaURL);
    this.entityLabel = entityLabel;
  }

  /** Fetch page schema and activate the default/hash tab. */
  async init(): Promise<void> {
    this.loading = true;
    this.error = null;

    try {
      const res = await fetch(this.schemaUrl);
      if (!res.ok) {
        throw new Error(`Failed to load ${this.entityLabel}: ${res.status} ${res.statusText}`);
      }
      this.pageSchema = await res.json();

      const hashTab = this.readHashTab();
      const defaultTab = this.defaultLeafTabId();
      const initialTab = hashTab && this.findTab(hashTab) ? hashTab : defaultTab;

      if (initialTab) {
        await this.setActiveTab(initialTab);
      }
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err);
    } finally {
      this.loading = false;
    }
  }

  /** Refetch the page schema to refresh tab counts. */
  async refreshCounts(): Promise<void> {
    try {
      const res = await fetch(this.schemaUrl);
      if (!res.ok) return;
      const updated: ProjectPageSchema = await res.json();
      if (this.pageSchema) {
        this.pageSchema.tabs = updated.tabs;
        this.pageSchema.nav_links = updated.nav_links;
        this.pageSchema.warnings = updated.warnings;
        this.pageSchema.entity = updated.entity;
        this.pageSchema.release = updated.release;
      } else {
        this.pageSchema = updated;
      }
    } catch {
      // Best-effort refresh.
    }
  }

  /**
   * Activate a tab by id. If the id resolves to a parent with children,
   * resolves down to a leaf using the last-visited child for that parent
   * (or the first non-disabled child as fallback).
   *
   * The schema's default tab never carries a URL hash — fresh entry to
   * /projects/X and explicit clicks back to overview both strip any
   * stale tab hash.
   */
  async setActiveTab(tabId: string): Promise<void> {
    if (!this.pageSchema) return;

    const resolved = this.resolveToLeafId(tabId);
    if (!resolved) return;

    const tab = this.findTab(resolved);
    if (!tab) return;

    // Right-aligned tabs (Settings, Exports) navigate full-page — never
    // activate them in-place. Caller should treat the href as a link.
    if (tab.href && !tab.content_url) {
      window.location.href = tab.href;
      return;
    }

    this.activeTabId = resolved;
    if (resolved === this.defaultLeafTabId()) {
      // Default tab carries no hash — keeps fresh entry to /projects/X
      // clean and ensures clicking back to overview also strips any
      // stale tab hash.
      this.clearHashTab();
    } else {
      this.writeHashTab(resolved);
    }
    this.recordChildVisit(resolved);

    if (this.isEntityListTab(tab)) {
      return;
    }

    if (!this.tabContent.has(resolved) && !this.tabLoading.get(resolved)) {
      await this.fetchTabContent(tab);
    }
  }

  /** Entity-list tabs are routed to EntityListView with schemaUrl = content_url. */
  isEntityListTab(tab: ProjectPageTab): boolean {
    return !!tab.content_url && /\/entity-list-schema\//.test(tab.content_url);
  }

  /** Walk the tab tree to find a tab by id (parent or child). */
  findTab(tabId: string): ProjectPageTab | null {
    if (!this.pageSchema) return null;
    return findTabIn(this.pageSchema.tabs, tabId);
  }

  /** Return the parent of a child tab, if any. Null for top-level tabs. */
  findParent(tabId: string): ProjectPageTab | null {
    if (!this.pageSchema) return null;
    for (const top of this.pageSchema.tabs) {
      if (top.children?.some((c) => c.id === tabId)) return top;
    }
    return null;
  }

  /** Is the given top-level tab the ancestor of the currently active leaf? */
  isActiveBranch(top: ProjectPageTab): boolean {
    if (top.id === this.activeTabId) return true;
    return !!top.children?.some((c) => c.id === this.activeTabId);
  }

  private resolveToLeafId(tabId: string): string {
    const tab = this.findTab(tabId);
    if (!tab) return '';
    if (!tab.children || tab.children.length === 0) return tab.id;
    // Parent click: use last-visited or first non-disabled child.
    const remembered = this.readChildMemory(tab.id);
    if (remembered && tab.children.some((c) => c.id === remembered && !c.disabled)) {
      return remembered;
    }
    const firstEnabled = tab.children.find((c) => !c.disabled);
    return firstEnabled ? firstEnabled.id : tab.children[0].id;
  }

  private defaultLeafTabId(): string {
    if (!this.pageSchema) return '';
    const def = this.pageSchema.tabs.find((t) => t.default);
    if (def) return this.resolveToLeafId(def.id);
    const first = this.pageSchema.tabs.find((t) => t.align !== 'right' && !t.disabled);
    return first ? this.resolveToLeafId(first.id) : '';
  }

  private recordChildVisit(leafId: string): void {
    const parent = this.findParent(leafId);
    if (!parent) return;
    try {
      window.localStorage.setItem(this.childMemoryKey(parent.id), leafId);
    } catch {
      // Private mode / sandboxed — non-fatal.
    }
  }

  private readChildMemory(parentId: string): string | null {
    try {
      return window.localStorage.getItem(this.childMemoryKey(parentId));
    } catch {
      return null;
    }
  }

  private childMemoryKey(parentId: string): string {
    return `pletka.projectTab.${this.projectId}.${parentId}`;
  }

  private async fetchTabContent(tab: ProjectPageTab): Promise<void> {
    if (!tab.content_url) return;
    this.tabLoading.set(tab.id, true);
    this.tabLoading = new Map(this.tabLoading);
    this.tabErrors.delete(tab.id);

    try {
      const res = await fetch(tab.content_url);
      if (!res.ok) {
        throw new Error(`Failed to load ${tab.id}: ${res.status}`);
      }
      const data = await res.json();
      this.tabContent.set(tab.id, data);
      this.tabContent = new Map(this.tabContent);
    } catch (err) {
      this.tabErrors.set(tab.id, err instanceof Error ? err.message : String(err));
      this.tabErrors = new Map(this.tabErrors);
    } finally {
      this.tabLoading.set(tab.id, false);
      this.tabLoading = new Map(this.tabLoading);
    }
  }

  /** Read one `key=value` parameter from the URL hash (#tab=fields&item=01…). */
  private readHashParam(key: string): string | null {
    const hash = window.location.hash;
    if (!hash) return null;
    const match = hash.match(new RegExp(`(?:^#?|&)${key}=([^&]+)`));
    return match ? decodeURIComponent(match[1]) : null;
  }

  /** Read active tab from URL hash (#tab=fields). */
  private readHashTab(): string | null {
    return this.readHashParam('tab');
  }

  /** Read the item to open on the active tab (#tab=examples&item=01…). */
  readHashItem(): string | null {
    return this.readHashParam('item');
  }

  /** Write active tab to URL hash; drops any item (switching tab closes it).
   *  A no-op when the hash already names this tab, so an `item` parameter
   *  present on initial load survives until the list has read it. */
  private writeHashTab(tabId: string): void {
    if (this.readHashTab() === tabId) return;
    const newHash = `#tab=${encodeURIComponent(tabId)}`;
    if (window.location.hash !== newHash) {
      history.replaceState(null, '', newHash);
    }
  }

  /** Add or remove the open item on the current tab's hash. */
  writeHashItem(id: string | null): void {
    if (!this.activeTabId || this.activeTabId === this.defaultLeafTabId()) return;
    const base = `#tab=${encodeURIComponent(this.activeTabId)}`;
    const newHash = id ? `${base}&item=${encodeURIComponent(id)}` : base;
    if (window.location.hash !== newHash) {
      history.replaceState(null, '', newHash);
    }
  }

  /** Close whatever the active tab's list has open and remount it. */
  resetActiveList(): void {
    this.writeHashItem(null);
    this.listReset += 1;
  }

  /** Strip any tab hash from the URL — used on default-tab entry. */
  private clearHashTab(): void {
    if (!window.location.hash) return;
    const clean = window.location.pathname + window.location.search;
    history.replaceState(null, '', clean);
  }

  private withCurrentVersion(raw: string): string {
    const version = new URLSearchParams(window.location.search).get('version');
    if (!version) return raw;
    try {
      const url = new URL(raw, window.location.origin);
      url.searchParams.set('version', version);
      return `${url.pathname}${url.search}${url.hash}`;
    } catch {
      return raw;
    }
  }
}

function findTabIn(tabs: ProjectPageTab[], id: string): ProjectPageTab | null {
  for (const t of tabs) {
    if (t.id === id) return t;
    if (t.children && t.children.length) {
      const inner = findTabIn(t.children, id);
      if (inner) return inner;
    }
  }
  return null;
}
