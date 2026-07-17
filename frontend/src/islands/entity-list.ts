import { mountIsland } from '../mount';
import '$lib/components/example/register-entity-list-editors';
import EntityListView from '$lib/components/entity-list/EntityListView.svelte';

mountIsland('entity-list', EntityListView);
