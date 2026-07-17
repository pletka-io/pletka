<script lang="ts">
  import { tr } from '$lib/types/form-schema';
  import type { FilterWidgetProps } from '../filter-widget-registry';
  import { controlID } from '../control-id';

  let {
    filter,
    lang,
    controlPrefix,
    selectedValues,
    filteredOptions,
    typeaheadQuery,
    onfilterchange,
    ontogglevalue,
    onquerychange,
  }: FilterWidgetProps = $props();

  const queryID = $derived(controlID(controlPrefix, filter.param_name, 'query'));
  const optionName = $derived(controlID(controlPrefix, filter.param_name));
</script>

<div class="space-y-2">
  <input
    id={queryID}
    name={queryID}
    type="text"
    aria-label="Search {tr(filter.label, lang).toLowerCase()}"
    placeholder="Search {tr(filter.label, lang).toLowerCase()}..."
    value={typeaheadQuery}
    oninput={(e) => onquerychange(filter.param_name, (e.target as HTMLInputElement).value)}
    class="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-pletka-primary focus:outline-none focus:ring-pletka-primary"
  />
  <ul class="max-h-60 space-y-1 overflow-y-auto rounded-md border border-gray-200 p-2">
    {#each filteredOptions as opt}
      {#if opt.value !== ''}
        <li>
          <label class="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-sm hover:bg-gray-50">
            <input
              id={controlID(optionName, opt.value)}
              type={filter.multi ? 'checkbox' : 'radio'}
              name={optionName}
              class="{filter.multi ? 'rounded' : ''} border-gray-300 text-pletka-primary focus:ring-pletka-primary"
              checked={selectedValues.includes(opt.value)}
              onchange={() => {
                if (filter.multi) {
                  ontogglevalue(filter, opt.value);
                } else {
                  onfilterchange(filter.param_name, selectedValues.includes(opt.value) ? '' : opt.value);
                }
              }}
            />
            <span class="text-gray-800">{tr(opt.label, lang)}</span>
          </label>
        </li>
      {/if}
    {/each}
    {#if filteredOptions.length === 0}
      <li class="px-2 py-1 text-sm text-gray-400">No matches</li>
    {/if}
  </ul>
</div>
