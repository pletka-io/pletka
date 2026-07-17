import { registerEntityListEditor } from '$lib/components/entity-list/editor-widget-registry';
import ExampleEntityListEditor from '$lib/components/entity-list/editors/ExampleEntityListEditor.svelte';

registerEntityListEditor('example-workspace', ExampleEntityListEditor);
