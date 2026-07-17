<script lang="ts">
  type Item = string | { title?: string; body?: string };
  type Stanza = { color?: string; items?: Item[]; emphasis?: boolean };

  let {
    block,
  }: {
    block: {
      heading?: string;
      stanzas?: Stanza[];
      items?: Item[];
    };
  } = $props();

  // Color name → Tailwind border class. Falls back to pletka-primary.
  const colorMap: Record<string, string> = {
    primary: 'border-pletka-primary',
    orange: 'border-orange-400',
    blue: 'border-blue-400',
    yellow: 'border-yellow-400',
    green: 'border-green-400',
    red: 'border-red-400',
    purple: 'border-purple-400',
    indigo: 'border-indigo-400',
    pink: 'border-pink-400',
    teal: 'border-teal-400',
    gray: 'border-gray-400',
  };

  // Backward-compat: a flat `items` list becomes one stanza in primary.
  const stanzas = $derived(
    block.stanzas ?? (block.items ? [{ color: 'primary', items: block.items }] : []),
  );

  function itemText(it: Item): string {
    if (typeof it === 'string') return it;
    return it.body ?? it.title ?? '';
  }
</script>

<section class="max-w-2xl mx-auto py-8 text-center">
  {#if block.heading}
    <h2 class="text-3xl font-bold text-gray-900 mb-12">{block.heading}</h2>
  {/if}
  <div class="text-left space-y-8">
    {#each stanzas as stanza}
      {@const borderClass = colorMap[stanza.color ?? 'primary'] ?? colorMap.primary}
      <div class="border-l-4 {borderClass} pl-6 py-2">
        {#each stanza.items ?? [] as it}
          <p
            class="font-medium italic {stanza.emphasis
              ? 'text-2xl text-gray-900 font-bold'
              : 'text-xl text-gray-800'}"
          >
            {itemText(it)}
          </p>
        {/each}
      </div>
    {/each}
  </div>
</section>
