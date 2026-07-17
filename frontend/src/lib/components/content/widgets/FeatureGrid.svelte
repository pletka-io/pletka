<script lang="ts">
  type Item = {
    icon?: string;
    title?: string;
    body?: string;
    href?: string;
    dot?: 'cyan' | 'orange' | 'gray';
  };

  let {
    block,
  }: {
    block: {
      heading?: string;
      columns?: number;
      items?: Item[];
      style?: 'card' | 'compact';
    };
  } = $props();

  const cols = $derived(block.columns ?? 3);
  const items = $derived(block.items ?? []);
  const style = $derived(block.style ?? 'card');

  function dotClass(dot?: string) {
    if (dot === 'orange') return 'bg-orange-500';
    if (dot === 'gray') return 'bg-gray-400';
    return 'bg-pletka-primary';
  }
</script>

<section class="py-12">
  {#if block.heading}
    <h2 class="text-2xl font-bold text-gray-900 mb-8 text-center">{block.heading}</h2>
  {/if}
  <div
    class="grid gap-3"
    style="grid-template-columns: repeat({cols}, minmax(0, 1fr));"
  >
    {#each items as item}
      {#if style === 'compact'}
        <div class="flex items-center bg-gray-50 rounded-lg px-4 py-3">
          <span class="w-2 h-2 rounded-full {dotClass(item.dot)} mr-3 flex-shrink-0"></span>
          <span class="text-gray-700">
            {#if item.title}<strong>{item.title}</strong>{/if}
            {#if item.body}{item.title ? ' — ' : ''}{item.body}{/if}
          </span>
        </div>
      {:else}
        <div class="bg-white border border-gray-200 rounded-lg p-6 shadow-sm hover:shadow-md transition-shadow">
          {#if item.icon}
            <div class="text-pletka-primary text-2xl mb-3" aria-hidden="true">{item.icon}</div>
          {/if}
          {#if item.title}
            <h3 class="font-semibold text-gray-900 mb-2">{item.title}</h3>
          {/if}
          {#if item.body}
            <p class="text-sm text-gray-600">{item.body}</p>
          {/if}
          {#if item.href}
            <a href={item.href} class="mt-3 inline-block text-sm text-pletka-primary hover:underline">Read more →</a>
          {/if}
        </div>
      {/if}
    {/each}
  </div>
</section>

<style>
  @media (max-width: 768px) {
    section :global(.grid) {
      grid-template-columns: 1fr !important;
    }
  }
</style>
