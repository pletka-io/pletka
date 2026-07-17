<script lang="ts">
  import type { FieldDef } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';
  import type { SubfieldPath } from '$lib/types/weave-types';

  let {
    field,
    value = $bindable([]),
    lang,
  }: {
    field: FieldDef;
    value: SubfieldPath[];
    lang: string;
  } = $props();

  // Defensive: backend omits the field entirely when empty, but guard anyway.
  const paths = $derived(Array.isArray(value) ? value : []);
</script>

{#if paths.length > 0}
  <div class="p-4 bg-gray-50 rounded-lg border border-gray-200">
    <dt class="flex items-center gap-2 text-sm font-medium text-gray-500">
      {tr(field.label, lang)}
      <span class="inline-flex items-center rounded bg-amber-100 px-1.5 py-0.5 text-xs font-medium text-amber-800">
        read-only · legacy
      </span>
    </dt>
    {#if field.help}
      <p class="mt-1 text-xs text-gray-400">{tr(field.help, lang)}</p>
    {/if}
    <ol class="mt-3 space-y-2">
      {#each paths as sub, i (i)}
        <li class="flex flex-wrap items-center gap-1 text-sm">
          <span class="mr-1 text-xs font-mono text-gray-400">#{i + 1}</span>
          {#each sub.path_elements as el, j (j)}
            {#if j > 0}<span class="text-gray-300">→</span>{/if}
            <span
              class="inline-flex items-center rounded bg-white px-2 py-0.5 font-mono text-xs ring-1 ring-inset ring-gray-200"
              class:text-blue-700={el.type === 'property'}
              class:text-emerald-700={el.type === 'class'}
              class:text-purple-700={el.type === 'literal'}
            >{el.uri || `${el.prefix}:${el.local_name}`}</span>
          {/each}
          {#if sub.expected_value_type}
            <span class="ml-1 rounded bg-gray-200 px-1.5 py-0.5 text-xs text-gray-600"
              >{sub.expected_value_type}</span>
          {/if}
        </li>
      {/each}
    </ol>
  </div>
{/if}
