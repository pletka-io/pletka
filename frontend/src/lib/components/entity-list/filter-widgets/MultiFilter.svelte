<script lang="ts">
  import { tr } from '$lib/types/form-schema';
  import type { FilterWidgetProps } from '../filter-widget-registry';
  import { controlID } from '../control-id';

  let { filter, lang, controlPrefix, selectedValues, options, ontogglevalue }: FilterWidgetProps = $props();

  const filterName = $derived(controlID(controlPrefix, filter.param_name));
</script>

<ul class="max-h-60 space-y-1 overflow-y-auto rounded-md border border-gray-200 p-2">
  {#each options as opt}
    {#if opt.value !== ''}
      <li>
        <label class="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-sm hover:bg-gray-50">
          <input
            id={controlID(filterName, opt.value)}
            name={filterName}
            type="checkbox"
            class="rounded border-gray-300 text-pletka-primary focus:ring-pletka-primary"
            checked={selectedValues.includes(opt.value)}
            onchange={() => ontogglevalue(filter, opt.value)}
          />
          <span class="text-gray-800">{tr(opt.label, lang)}</span>
        </label>
      </li>
    {/if}
  {/each}
</ul>
