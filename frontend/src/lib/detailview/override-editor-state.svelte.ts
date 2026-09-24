import type {
  OverrideCategoryOption,
  OverrideEditorField,
  OverrideEditorItem,
  OverrideEditorResponse,
  OverrideSearchResult,
} from '$lib/detailview/override-editor-types';
import {
  DIRECT_FIELDS_ID,
  DIRECT_FIELDS_NAME,
  FIELD_GROUP_WIDGET,
  COLLECTION_GROUP_WIDGET,
  isDirectFieldGroup,
  isDirectFieldsBucket,
} from '$lib/detailview/group-kind';
import type { Translations } from '$lib/types/weave-types';

export type OverrideSearchMode = 'text' | 'path';
export const UNCATEGORIZED_ID = '__uncategorized__';
export { DIRECT_FIELDS_ID } from '$lib/detailview/group-kind';

// LEGACY_DRAFT_FINGERPRINT is sent on save instead of an empty string when
// a restored draft has no stored fingerprint (a draft written by the
// editor before this feature shipped). An empty fingerprint tells the
// server to skip its staleness check entirely — that opt-out exists for
// ops tooling and git restore, which never loaded an editor payload to
// carry one. The editor always loads a payload, so it must never opt
// out: a legacy draft with no fingerprint is exactly the case the
// conflict flow exists to catch (rollout day: a curator holds a
// pre-deploy draft, someone else saves first). This value is
// deliberately not 32 hex characters, so it can never collide with a
// real fingerprint and always falls into the normal "someone saved this
// while you were editing" conflict path instead of the skip path.
const LEGACY_DRAFT_FINGERPRINT = 'legacy-draft';

// filterRefsByIDs preserves the display refs (with name + semantic_id)
// matching the post-mutation ID list. New IDs that have no resolved ref
// yet keep a bare {id} entry so the chip still renders something — full
// resolution lands on the next /overrides reload. Order follows the
// supplied id list so the rendered pill order matches.
function filterRefsByIDs(
  refs: Array<{ id: string; semantic_id?: string; name?: import('$lib/types/weave-types').Translations; url?: string }>,
  ids: string[],
): Array<{ id: string; semantic_id?: string; name?: import('$lib/types/weave-types').Translations; url?: string }> {
  const byID = new Map(refs.map((r) => [r.id, r]));
  return ids.map((id) => byID.get(id) ?? { id });
}

export class OverrideEditorState {
  response: OverrideEditorResponse | null = $state(null);
  loading = $state(false);
  error: string | null = $state(null);
  searchMode: OverrideSearchMode = $state('text');
  dirty = $state(false);
  restoredDraft = $state(false);
  // fingerprint is the content hash this editor's in-memory state is
  // known to match — the baseline the next save round-trips back to the
  // server. Starts from the load payload; a restored draft overrides it
  // with the draft's OWN stored fingerprint (the version it actually
  // diverged from), never the freshly-loaded live one — substituting the
  // live fingerprint here would make a genuinely stale draft's save look
  // clean and silently overwrite newer work.
  fingerprint: string | null = $state(null);
  // staleDraft is true when a restored draft's stored fingerprint is
  // missing or no longer matches what was live at load time — someone
  // else saved between when this draft was captured and now. The draft
  // is still restored either way; this only flags it.
  staleDraft = $state(false);

  constructor(public readonly url: string) {}

  private tempId = 0;

  // draftVersion increments on every commit() — i.e. every user edit
  // that changes the in-memory tree. saveDraft() captures it (via
  // `version`) right before snapshotting the payload it sends; markSaved
  // compares it against the current value to tell "no edit landed while
  // this request was in flight" (safe to drop the local draft) from "an
  // edit landed mid-flight and was never sent" (keep it, stay dirty).
  private draftVersion = 0;

  get version(): number {
    return this.draftVersion;
  }

  // fingerprintForSave is what a save actually sends — never the bare,
  // possibly-null `fingerprint`. See LEGACY_DRAFT_FINGERPRINT: a real
  // fingerprint is never empty here (a legacy null is substituted with a
  // sentinel that always mismatches), so the server's empty-string
  // skip-the-check path is never reachable from the editor.
  get fingerprintForSave(): string {
    return this.fingerprint || LEGACY_DRAFT_FINGERPRINT;
  }

  // categoryNamesByID reads from the response's available_categories
  // roster — backend builds the full project category list once and
  // every dropdown / ensureCategory call consumes it. No separate fetch.
  private categoryNamesByID(): Map<string, Translations> {
    const out = new Map<string, Translations>();
    if (!this.response) return out;
    for (const ref of this.response.available_categories ?? []) {
      if (ref.id && ref.name) out.set(ref.id, ref.name);
    }
    return out;
  }

  // fetchLive fetches and parses the current server payload with no
  // draft involved — shared by init() (which then decides whether to
  // overlay a local draft) and discardDraft() (which never does).
  private async fetchLive(): Promise<OverrideEditorResponse> {
    const res = await fetch(this.url);
    if (!res.ok) {
      throw new Error(`Failed to load override editor: ${res.status} ${res.statusText}`);
    }
    return (await res.json()) as OverrideEditorResponse;
  }

  async init(): Promise<void> {
    this.loading = true;
    this.error = null;
    try {
      const live = await this.fetchLive();
      const draft = this.loadDraft(live);
      // Dedupe items per category by id and fields per item by override_id
      // before handing to render. Stale localStorage drafts could carry
      // duplicates from a pre-guard editor session; without this, the
      // page crashes with each_key_duplicate before the curator can hit
      // "Discard draft".
      this.response = this.dedupe(draft?.response ?? live);
      this.dirty = draft !== null;
      this.restoredDraft = draft !== null;
      if (draft) {
        this.fingerprint = draft.fingerprint;
        this.staleDraft = !draft.fingerprint || draft.fingerprint !== live.fingerprint;
      } else {
        this.fingerprint = live.fingerprint;
        this.staleDraft = false;
      }
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err);
    } finally {
      this.loading = false;
    }
  }

  setSearchMode(mode: OverrideSearchMode): void {
    this.searchMode = mode;
  }

  addFieldToCategory(categoryId: string | null, result: OverrideSearchResult): void {
    if (!this.response) return;
    const targetCategoryId = categoryId || UNCATEGORIZED_ID;

    // Guard against the same field landing twice in the same category's
    // direct-fields group. Backend (field_id, entity_type, entity_id)
    // uniqueness would 500 on save; nicer to no-op now.
    const dupInDirect = this.response.categories.some(
      (cat) =>
        cat.category_id === targetCategoryId &&
        cat.items.some(
          (it) =>
            it.id === DIRECT_FIELDS_ID &&
            it.fields.some((f) => f.field_id === result.id),
        ),
    );
    if (dupInDirect) return;

    const nextField: OverrideEditorField = {
      field_id: result.id,
      override_id: this.nextTempId(),
      position: 1,
      display_name: result.ui_name,
      description: result.description,
      ontology_path: result.ontology_path,
      path_elements: result.path_elements,
      category_id: targetCategoryId,
      expected_value_type: result.expected_value_type,
      expected_resource_models: result.expected_resource_models || [],
      expected_collection_models: result.expected_collection_models || [],
      expected_concept_lists: result.expected_concept_lists || [],
      expected_resource_model_refs: result.expected_resource_model_refs || [],
      expected_collection_model_refs: result.expected_collection_model_refs || [],
      expected_concept_list_refs: result.expected_concept_list_refs || [],
      set_value: '',
      is_required: false,
      min_occurs: 0,
      max_occurs: null,
      is_hidden: false,
      visibility: '',
    };

    const categories = this.ensureCategory(this.response.categories, targetCategoryId).map((category) => {
      if (category.category_id !== targetCategoryId) return category;

      const existingGroup = category.items.find((item) => item.id === DIRECT_FIELDS_ID);
      nextField.position = existingGroup ? existingGroup.fields.length + 1 : 1;

      let items: OverrideEditorItem[];
      if (existingGroup) {
        items = category.items.map((item) =>
          item.id === DIRECT_FIELDS_ID
            ? {
                ...item,
                field_count: item.fields.length + 1,
                fields: [...item.fields, nextField],
              }
            : item,
        );
      } else {
        const directGroup: OverrideEditorItem = {
          widget: FIELD_GROUP_WIDGET,
          id: DIRECT_FIELDS_ID,
          name: DIRECT_FIELDS_NAME,
          position: this.nextItemPosition(category.items),
          field_count: 1,
          fields: [nextField],
        };
        items = [...category.items, directGroup].sort((a, b) => a.position - b.position);
      }

      return { ...category, items };
    });

    this.commit({ ...this.response, categories });
  }

  insertAdoptedCollection(categoryId: string | null, item: OverrideEditorItem): void {
    if (!this.response) return;
    const targetCategoryId = categoryId || UNCATEGORIZED_ID;

    // Guard against duplicate collection-group items. Adding the same
    // collection twice would produce two items keyed by `item.id`,
    // breaking the `{#each ... (item.id)}` loop with each_key_duplicate.
    // Collections are unique-per-category by design — re-adding is a
    // no-op rather than a render crash.
    const alreadyPresent = this.response.categories.some((cat) =>
      cat.items.some((it) => it.id === item.id),
    );
    if (alreadyPresent) return;

    const fields = item.fields.map((field, index) => ({
      ...field,
      override_id: this.nextTempId(),
      category_id: targetCategoryId,
      part_of_collection_id: item.id,
      position: index + 1,
    }));

    const categories = this.ensureCategory(this.response.categories, targetCategoryId).map((category) => {
      if (category.category_id !== targetCategoryId) return category;
      const nextItem: OverrideEditorItem = {
        ...item,
        widget: COLLECTION_GROUP_WIDGET,
        position: this.nextItemPosition(category.items),
        field_count: fields.length,
        fields,
      };
      return {
        ...category,
        items: [...category.items, nextItem].sort((a, b) => a.position - b.position),
      };
    });

    this.commit({ ...this.response, categories });
  }

  addCategory(category: OverrideCategoryOption): void {
    if (!this.response) return;
    const existing = this.response.categories.find((item) => item.category_id === category.id);
    if (existing) return;
    const categories = [
      ...this.response.categories,
      {
        category_id: category.id,
        category_name: category.ui_name,
        position: category.canonical_order || this.response.categories.length + 1,
        items: [],
      },
    ].sort((a, b) => a.position - b.position);
    this.commit({ ...this.response, categories });
  }

  updateField(
    categoryId: string,
    itemId: string,
    overrideID: number,
    patch: Partial<
      Pick<
        OverrideEditorField,
        | 'is_required'
        | 'is_hidden'
        | 'set_value'
        | 'min_occurs'
        | 'max_occurs'
        | 'visibility'
        | 'expected_resource_models'
        | 'expected_collection_models'
        | 'expected_concept_lists'
        | 'expected_resource_model_refs'
        | 'expected_collection_model_refs'
        | 'expected_concept_list_refs'
        | 'category_id'
        | 'display_name'
        | 'description'
      >
    >,
  ): void {
    if (!this.response) return;
    const categories = this.response.categories.map((category) => {
      if (category.category_id !== categoryId) return category;
      return {
        ...category,
        items: category.items.map((item) => {
          if (item.id !== itemId) return item;
          return {
            ...item,
            fields: item.fields.map((field) => {
              if (field.override_id !== overrideID) return field;
              const next = { ...field, ...patch };
              // Keep the display refs ({id, semantic_id, name}) in sync
              // with the bare-id arrays. Caller may also pass an
              // explicit `*_refs` patch carrying labels for newly-added
              // refs — when present, that wins over the filter-from-old
              // fallback (which can only stub names it doesn't know).
              if ('expected_resource_models' in patch && !('expected_resource_model_refs' in patch)) {
                next.expected_resource_model_refs = filterRefsByIDs(
                  field.expected_resource_model_refs ?? [],
                  patch.expected_resource_models ?? [],
                );
              }
              if ('expected_collection_models' in patch && !('expected_collection_model_refs' in patch)) {
                next.expected_collection_model_refs = filterRefsByIDs(
                  field.expected_collection_model_refs ?? [],
                  patch.expected_collection_models ?? [],
                );
              }
              if ('expected_concept_lists' in patch && !('expected_concept_list_refs' in patch)) {
                next.expected_concept_list_refs = filterRefsByIDs(
                  field.expected_concept_list_refs ?? [],
                  patch.expected_concept_lists ?? [],
                );
              }
              return next;
            }),
          };
        }),
      };
    });
    this.commit({ ...this.response, categories });
  }

  moveField(categoryId: string, itemId: string, overrideID: number, delta: -1 | 1): void {
    if (!this.response) return;
    const categories = this.response.categories.map((category) => {
      if (category.category_id !== categoryId) return category;
      return {
        ...category,
        items: category.items.map((item) => {
          if (item.id !== itemId) return item;
          const index = item.fields.findIndex((field) => field.override_id === overrideID);
          if (index < 0) return item;
          const nextIndex = index + delta;
          if (nextIndex < 0 || nextIndex >= item.fields.length) return item;
          const fields = [...item.fields];
          const [moved] = fields.splice(index, 1);
          fields.splice(nextIndex, 0, moved);
          return {
            ...item,
            fields: this.normalizeFieldPositions(fields),
          };
        }),
      };
    });
    this.commit({ ...this.response, categories });
  }

  // Update mutable display fields on an item (collection-group name
  // override, etc.). Item position + widget are not patchable here —
  // moveItem owns position; widget is structural.
  updateItem(
    categoryId: string,
    itemId: string,
    patch: Partial<Pick<OverrideEditorItem, 'name' | 'placement'>>,
  ): void {
    if (!this.response) return;
    const categories = this.response.categories.map((category) => {
      if (category.category_id !== categoryId) return category;
      return {
        ...category,
        items: category.items.map((item) =>
          item.id === itemId ? { ...item, ...patch } : item,
        ),
      };
    });
    this.commit({ ...this.response, categories });
  }

  // Move an item (collection-group or direct-fields group) from one
  // category to another. Inserts at the end of the destination
  // category's items, normalizes positions in both source and dest.
  // Each field inside the item also gets its `category_id` rewritten so
  // the backend flatten step produces consistent rows.
  // Move one field (by override_id) out of its current direct-fields
  // group into another category's direct-fields group. Only valid for
  // fields in a `field-group` (direct-fields) item — the OverrideSidebar
  // hides the category select for collection-group fields, but we
  // defensively no-op here too if the source isn't a direct group.
  //
  // Side effects:
  //   - field.category_id rewritten to the destination
  //   - source item.fields shrinks; if it ends up empty, the item is
  //     dropped from the source category
  //   - destination's direct-fields item gains the field at the end;
  //     created from scratch if it didn't exist
  //   - positions normalised in source + dest groups, plus item
  //     positions in source category if an item was dropped
  moveFieldToCategory(
    fromCategoryID: string,
    fromItemID: string,
    overrideID: number,
    toCategoryID: string,
  ): void {
    if (!this.response) return;
    if (fromCategoryID === toCategoryID) return;

    let movingField: OverrideEditorField | null = null;

    // Strip the field from the source category. Drop empty direct-fields
    // groups while we're at it so the source doesn't render a phantom
    // empty bucket.
    const stripped = this.response.categories.map((category) => {
      if (category.category_id !== fromCategoryID) return category;
      const items = category.items
        .map((item) => {
          if (item.id !== fromItemID) return item;
          // Lock out: only direct-fields groups can have individual
          // fields moved. Collection-group fields move as a unit via
          // moveItemToCategory.
          if (!isDirectFieldGroup(item)) return item;
          const fields = item.fields.filter((f) => {
            if (f.override_id === overrideID) {
              movingField = f;
              return false;
            }
            return true;
          });
          if (fields.length === 0) return null;
          return {
            ...item,
            field_count: fields.length,
            fields: this.normalizeFieldPositions(fields),
          };
        })
        .filter((item): item is OverrideEditorItem => item !== null);
      return { ...category, items: this.normalizeItemPositions(items) };
    });

    if (!movingField) return;

    const movingFieldNarrowed = movingField as OverrideEditorField;
    const rewrittenField: OverrideEditorField = {
      ...movingFieldNarrowed,
      category_id: toCategoryID,
    };

    // Insert into destination's direct-fields group; create the group
    // if the destination has none yet.
    let withTarget = this.ensureCategory(stripped, toCategoryID);
    withTarget = withTarget.map((category) => {
      if (category.category_id !== toCategoryID) return category;
      const directIdx = category.items.findIndex(isDirectFieldsBucket);
      let items: OverrideEditorItem[];
      if (directIdx >= 0) {
        items = category.items.map((item, idx) => {
          if (idx !== directIdx) return item;
          const fields = this.normalizeFieldPositions([
            ...item.fields,
            { ...rewrittenField, position: item.fields.length + 1 },
          ]);
          return { ...item, field_count: fields.length, fields };
        });
      } else {
        const direct: OverrideEditorItem = {
          widget: FIELD_GROUP_WIDGET,
          id: DIRECT_FIELDS_ID,
          name: DIRECT_FIELDS_NAME,
          position: this.nextItemPosition(category.items),
          field_count: 1,
          fields: [{ ...rewrittenField, position: 1 }],
        };
        items = [...category.items, direct];
      }
      return { ...category, items: this.normalizeItemPositions(items) };
    });

    this.commit({ ...this.response, categories: withTarget });
  }

  moveItemToCategory(fromCategoryID: string, itemID: string, toCategoryID: string): void {
    if (!this.response) return;
    if (fromCategoryID === toCategoryID) return;

    let movingItem: OverrideEditorItem | null = null;

    // Pull the item out of the source category.
    const stripped = this.response.categories.map((category) => {
      if (category.category_id !== fromCategoryID) return category;
      const next = category.items.filter((it) => {
        if (it.id === itemID) {
          movingItem = it;
          return false;
        }
        return true;
      });
      return { ...category, items: this.normalizeItemPositions(next) };
    });

    if (!movingItem) return;

    // Rewrite each child field's category_id, leave names/positions alone.
    const rewritten: OverrideEditorItem = {
      ...(movingItem as OverrideEditorItem),
      fields: (movingItem as OverrideEditorItem).fields.map((f) => ({
        ...f,
        category_id: toCategoryID,
      })),
    };

    // Append to destination, ensure category exists.
    let withTarget = this.ensureCategory(stripped, toCategoryID);
    withTarget = withTarget.map((category) => {
      if (category.category_id !== toCategoryID) return category;
      const items = [...category.items, { ...rewritten, position: this.nextItemPosition(category.items) }];
      return { ...category, items: this.normalizeItemPositions(items) };
    });

    this.commit({ ...this.response, categories: withTarget });
  }

  // Reorder a group's fields to match the supplied override_id sequence.
  // Used by the inline-DnD surface — `svelte-dnd-action`'s onfinalize
  // hands us the new ordered items, we feed only the IDs back so the
  // state stays the source of truth and `normalizeFieldPositions`
  // reassigns sequential positions.
  reorderFields(categoryId: string, itemId: string, orderedOverrideIDs: number[]): void {
    if (!this.response) return;
    const categories = this.response.categories.map((category) => {
      if (category.category_id !== categoryId) return category;
      return {
        ...category,
        items: category.items.map((item) => {
          if (item.id !== itemId) return item;
          const byID = new Map(item.fields.map((f) => [f.override_id, f]));
          const reordered: typeof item.fields = [];
          for (const id of orderedOverrideIDs) {
            const f = byID.get(id);
            if (f) {
              reordered.push(f);
              byID.delete(id);
            }
          }
          // Append any unreferenced fields (defensive — DnD should
          // surface every row, but a stale id won't drop a field).
          for (const leftover of byID.values()) reordered.push(leftover);
          return {
            ...item,
            fields: this.normalizeFieldPositions(reordered),
          };
        }),
      };
    });
    this.commit({ ...this.response, categories });
  }

  removeField(categoryId: string, itemId: string, overrideID: number): void {
    if (!this.response) return;
    const categories = this.response.categories.map((category) => {
      if (category.category_id !== categoryId) return category;
      const items = category.items
        .map((item) => {
          if (item.id !== itemId) return item;
          const fields = item.fields.filter((field) => field.override_id !== overrideID);
          if (fields.length === 0) return null;
          return {
            ...item,
            field_count: fields.length,
            fields: this.normalizeFieldPositions(fields),
          };
        })
        .filter((item): item is OverrideEditorItem => item !== null);
      return {
        ...category,
        items: this.normalizeItemPositions(items),
      };
    });
    this.commit({ ...this.response, categories });
  }

  // Remove a collection-group item (and its member field rows) from a
  // category. Used by the collection sidebar's destructive "Remove"
  // action. Omitting the group's rows from the next
  // save is what actually deletes them: saveDraft's PUT replaces the
  // whole override set for this entity (ReplaceForEntity deletes and
  // re-creates), so any row missing from the payload is gone from
  // storage after save. The collection entity and its base fields are
  // untouched — only this model/collection's use of them disappears.
  removeCollectionGroup(categoryId: string, itemId: string): void {
    if (!this.response) return;
    const categories = this.response.categories.map((category) => {
      if (category.category_id !== categoryId) return category;
      const items = category.items.filter((item) => item.id !== itemId);
      return {
        ...category,
        items: this.normalizeItemPositions(items),
      };
    });
    this.commit({ ...this.response, categories });
  }

  moveItem(categoryId: string, itemId: string, delta: -1 | 1): void {
    if (!this.response) return;
    const categories = this.response.categories.map((category) => {
      if (category.category_id !== categoryId) return category;
      const index = category.items.findIndex((item) => item.id === itemId);
      if (index < 0) return category;
      const nextIndex = index + delta;
      if (nextIndex < 0 || nextIndex >= category.items.length) return category;
      const items = [...category.items];
      const [moved] = items.splice(index, 1);
      items.splice(nextIndex, 0, moved);
      return {
        ...category,
        items: this.normalizeItemPositions(items),
      };
    });
    this.commit({ ...this.response, categories });
  }

  // discardDraft refetches the server payload FIRST and only discards the
  // local draft once that refetch succeeds — never the other way round.
  // Clearing localStorage before confirming the refetch worked would
  // leave a curator whose network drops (or whose session lapsed) mid
  // reload staring at their own still-on-screen work with the Save
  // button disabled and no local backup left to recover it from.
  async discardDraft(): Promise<void> {
    this.loading = true;
    this.error = null;
    try {
      const live = await this.fetchLive();
      this.clearDraft();
      this.response = this.dedupe(live);
      this.dirty = false;
      this.restoredDraft = false;
      this.staleDraft = false;
      this.fingerprint = live.fingerprint;
    } catch (err) {
      // Refetch failed — keep the draft and the on-screen work exactly
      // as they were; only the error changes.
      this.error = err instanceof Error ? err.message : String(err);
    } finally {
      this.loading = false;
    }
  }

  // markSaved is called after a successful save. sinceVersion is the
  // `version` captured right before the request was sent: if it still
  // matches, nothing changed in memory while the request was in flight
  // and the local draft is now redundant — clear it. If it no longer
  // matches, an edit landed mid-flight and was never sent: keep the
  // draft (still dirty, so the Save button and unsaved-changes banner
  // keep reflecting that there is more to send) and re-persist it so its
  // stored fingerprint matches whatever the caller just set as the new
  // baseline (see `fingerprint`) rather than the one this save replaced.
  // Callers that never track a version (none today) get the old
  // unconditional-clear behaviour.
  markSaved(sinceVersion?: number): void {
    this.staleDraft = false;
    if (sinceVersion !== undefined && sinceVersion !== this.draftVersion) {
      if (this.response) this.persistDraft(this.response);
      return;
    }
    this.clearDraft();
    this.dirty = false;
    this.restoredDraft = false;
  }

  serializePayload(): { categories: OverrideEditorResponse['categories'] } | null {
    if (!this.response) return null;
    return {
      categories: this.response.categories.map((category) => ({
        ...category,
        category_id: category.category_id === UNCATEGORIZED_ID ? '' : category.category_id,
        items: category.items.map((item) => ({
          ...item,
          fields: item.fields.map((field) => ({
            ...field,
            category_id: field.category_id === UNCATEGORIZED_ID ? '' : field.category_id,
          })),
        })),
      })),
    };
  }

  private nextTempId(): number {
    this.tempId += 1;
    return -this.tempId;
  }

  private nextItemPosition(items: OverrideEditorItem[]): number {
    return items.reduce((max, item) => Math.max(max, item.position), 0) + 1;
  }

  private normalizeFieldPositions(fields: OverrideEditorField[]): OverrideEditorField[] {
    return fields.map((field, index) => ({
      ...field,
      position: index + 1,
    }));
  }

  private normalizeItemPositions(items: OverrideEditorItem[]): OverrideEditorItem[] {
    return items.map((item, index) => ({
      ...item,
      position: index + 1,
    }));
  }

  private ensureCategory(categories: OverrideEditorResponse['categories'], categoryId: string): OverrideEditorResponse['categories'] {
    const existing = categories.find((item) => item.category_id === categoryId);
    if (existing) return categories;
    const cachedName = this.categoryNamesByID().get(categoryId);
    const next = [
      ...categories,
      {
        category_id: categoryId,
        category_name: cachedName ?? { en: categoryId },
        position: this.nextCategoryPosition(categories),
        items: [],
      },
    ];
    return next.sort((a, b) => a.position - b.position);
  }

  private nextCategoryPosition(categories: OverrideEditorResponse['categories']): number {
    return categories.reduce((max, category) => Math.max(max, category.position), 0) + 1;
  }

  private commit(response: OverrideEditorResponse): void {
    this.response = response;
    this.dirty = true;
    this.restoredDraft = false;
    this.draftVersion += 1;
    this.persistDraft(response);
  }

  private draftKey(response: OverrideEditorResponse): string {
    return [
      'override-editor-draft',
      response.project_id,
      response.entity_type,
      response.entity_id,
      this.url,
    ].join(':');
  }

  // Strip duplicate items (by id) within a category and duplicate fields
  // (by override_id) within an item. Defensive — the addX guards already
  // prevent duplicates being created, but stale localStorage drafts from
  // pre-guard sessions could still carry them and crash the each-key.
  // First occurrence wins; later duplicates dropped silently.
  //
  // Also normalises any `category_id === ''` (the backend's empty bucket)
  // to UNCATEGORIZED_ID so all in-memory state uses a single sentinel.
  // Without this, post-save reload returns '' for uncategorized rows but
  // openGlobalAdd targets UNCATEGORIZED_ID, leading ensureCategory to
  // create a second bucket → duplicate sections in the editor.
  // serializePayload converts back to '' before save.
  private dedupe(response: OverrideEditorResponse): OverrideEditorResponse {
    const norm = (id: string) => (id === '' ? UNCATEGORIZED_ID : id);

    // Merge categories sharing the same post-normalisation id. Stale
    // drafts may carry both '' and UNCATEGORIZED_ID buckets which would
    // otherwise collide on the each-key. Items + fields from later
    // buckets append; per-item dedupe (below) folds duplicate __direct__
    // groups into one.
    const merged = new Map<string, OverrideEditorResponse['categories'][number]>();
    for (const category of response.categories) {
      const id = norm(category.category_id);
      const existing = merged.get(id);
      if (!existing) {
        merged.set(id, { ...category, category_id: id });
        continue;
      }
      merged.set(id, {
        ...existing,
        items: [...existing.items, ...category.items],
      });
    }

    const categories = Array.from(merged.values()).map((category) => {
      // Fold items by id — duplicate __direct__ groups across the merged
      // categories combine their fields into one group.
      const itemsByID = new Map<string, OverrideEditorResponse['categories'][number]['items'][number]>();
      for (const item of category.items) {
        const prev = itemsByID.get(item.id);
        if (!prev) {
          itemsByID.set(item.id, item);
          continue;
        }
        itemsByID.set(item.id, {
          ...prev,
          fields: [...prev.fields, ...item.fields],
        });
      }

      const items = Array.from(itemsByID.values()).map((item) => {
        const seenFieldOverrideIDs = new Set<number>();
        const fields = item.fields
          .filter((f) => {
            if (seenFieldOverrideIDs.has(f.override_id)) return false;
            seenFieldOverrideIDs.add(f.override_id);
            return true;
          })
          .map((f) => ({ ...f, category_id: norm(f.category_id ?? '') }));
        return { ...item, fields, field_count: fields.length };
      });
      return { ...category, items };
    });
    return { ...response, categories };
  }

  private loadDraft(
    response: OverrideEditorResponse,
  ): { response: OverrideEditorResponse; fingerprint: string | null } | null {
    if (typeof window === 'undefined') return null;
    try {
      const raw = window.localStorage.getItem(this.draftKey(response));
      if (!raw) return null;
      const parsed = JSON.parse(raw) as { response?: OverrideEditorResponse; fingerprint?: string };
      if (!parsed.response) return null;
      if (
        parsed.response.project_id !== response.project_id ||
        parsed.response.entity_type !== response.entity_type ||
        parsed.response.entity_id !== response.entity_id
      ) {
        return null;
      }
      return { response: parsed.response, fingerprint: parsed.fingerprint ?? null };
    } catch {
      return null;
    }
  }

  private persistDraft(response: OverrideEditorResponse): void {
    if (typeof window === 'undefined') return;
    try {
      window.localStorage.setItem(
        this.draftKey(response),
        JSON.stringify({
          saved_at: new Date().toISOString(),
          response,
          fingerprint: this.fingerprint,
        }),
      );
    } catch {
      // ignore local-storage failures; editor state still lives in memory
    }
  }

  private clearDraft(): void {
    if (typeof window === 'undefined' || !this.response) return;
    try {
      window.localStorage.removeItem(this.draftKey(this.response));
    } catch {
      // ignore local-storage failures
    }
  }
}
