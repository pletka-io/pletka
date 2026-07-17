import { mountIsland } from '../mount';
import DetailView from '$lib/detailview/components/DetailView.svelte';
// The model detail page renders the Examples tab. Register its entity-list
// editor here; without it getEntityListEditor('example-workspace') returns
// null and EntityListView falls back to FormRenderer with an empty
// form_schema_url, which resolves to the current page URL and returns HTML.
import '$lib/components/example/register-entity-list-editors';

// Legacy island name kept for compatibility. Conceptually this is the
// detailview platform surface.
mountIsland('entity-view', DetailView);
