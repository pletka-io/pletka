import { mountIsland } from '../mount';
import ProjectDetail from '$lib/components/project/ProjectDetail.svelte';
// The project page renders the Examples tab. Register its entity-list editor
// here; without it getEntityListEditor('example-workspace') returns null and
// EntityListView falls back to FormRenderer with an empty form_schema_url,
// which resolves to the current page URL and returns HTML.
import '$lib/components/example/register-entity-list-editors';

mountIsland('project-detail', ProjectDetail);
