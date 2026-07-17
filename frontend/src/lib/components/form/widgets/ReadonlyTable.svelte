<script lang="ts">
  import type { FieldDef } from '$lib/types/form-schema';
  import { tr } from '$lib/types/form-schema';

  let {
    field,
    value = $bindable([]),
    lang,
  }: {
    field: FieldDef;
    value: any[];
    lang: string;
  } = $props();

  // Columns are defined in field.options as {value: column_key, label: {en: "Header"}}
  let columns = $derived(field.options ?? []);
  let rows = $derived(Array.isArray(value) ? value : []);
</script>

<div>
  <div class="block text-sm font-medium text-gray-700 mb-2">
    {tr(field.label, lang)}
  </div>

  {#if rows.length === 0}
    <p class="text-sm text-gray-500 italic">No items</p>
  {:else}
    <div class="border border-gray-200 rounded-lg overflow-hidden">
      <table class="min-w-full divide-y divide-gray-200">
        <thead class="bg-gray-50">
          <tr>
            {#each columns as col}
              <th class="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                {tr(col.label, lang)}
              </th>
            {/each}
          </tr>
        </thead>
        <tbody class="bg-white divide-y divide-gray-200">
          {#each rows as row}
            <tr>
              {#each columns as col}
                <td class="px-4 py-2 text-sm text-gray-900 whitespace-nowrap">
                  {row[col.value] ?? ''}
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  {#if field.help}
    <p class="mt-1 text-sm text-gray-500">{tr(field.help, lang)}</p>
  {/if}
</div>
