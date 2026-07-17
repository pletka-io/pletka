<script lang="ts">
  import { tr } from '$lib/types/form-schema';
  import type { FilterWidgetProps } from '../filter-widget-registry';
  import { controlID } from '../control-id';

  let { filter, lang, controlPrefix, currentValue, options, onfilterchange }: FilterWidgetProps = $props();

  const filterID = $derived(controlID(controlPrefix, filter.param_name));
</script>

<select
  id={filterID}
  name={filter.param_name}
  aria-label={tr(filter.label, lang)}
  value={currentValue}
  onchange={(e) => onfilterchange(filter.param_name, (e.target as HTMLSelectElement).value)}
  class="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-pletka-primary focus:outline-none focus:ring-pletka-primary"
>
  {#each options as opt}
    <option value={opt.value}>{tr(opt.label, lang)}</option>
  {/each}
</select>
