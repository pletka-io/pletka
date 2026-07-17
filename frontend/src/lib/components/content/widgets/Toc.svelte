<script lang="ts">
  /**
   * Sticky table of contents with scroll-spy.
   * Walks document headings (h2, h3) on mount, builds a TOC, observes
   * intersection to highlight the active section. Used by long-form
   * pages like /vision.
   */
  import { onMount } from 'svelte';

  let {
    block,
  }: {
    block: {
      heading?: string;
      // optional: limit which heading levels appear (default h2+h3)
      levels?: number[];
    };
  } = $props();

  interface Entry {
    id: string;
    text: string;
    level: number;
  }

  let entries = $state<Entry[]>([]);
  let activeId = $state<string>('');

  const levels = $derived(block.levels ?? [2, 3]);

  onMount(() => {
    const found: Entry[] = [];
    const selectors = levels.map((l) => 'h' + l).join(',');
    document.querySelectorAll<HTMLElement>(selectors).forEach((el) => {
      // Skip headings inside the TOC itself.
      if (el.closest('.content-toc')) return;
      if (!el.id) {
        el.id = el.textContent?.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') ?? '';
      }
      if (!el.id) return;
      found.push({
        id: el.id,
        text: el.textContent?.trim() ?? '',
        level: parseInt(el.tagName.slice(1), 10),
      });
    });
    entries = found;

    if (found.length === 0) return;

    const observer = new IntersectionObserver(
      (changes) => {
        for (const c of changes) {
          if (c.isIntersecting) {
            activeId = c.target.id;
            break;
          }
        }
      },
      { rootMargin: '-30% 0px -60% 0px', threshold: 0 },
    );
    found.forEach((e) => {
      const el = document.getElementById(e.id);
      if (el) observer.observe(el);
    });
    return () => observer.disconnect();
  });
</script>

{#if entries.length > 0}
  <aside class="content-toc hidden lg:block fixed top-24 right-6 w-64 max-h-[80vh] overflow-y-auto text-sm">
    {#if block.heading}
      <h2 class="font-semibold text-gray-700 mb-3 uppercase tracking-wide text-xs">{block.heading}</h2>
    {/if}
    <ul class="space-y-1.5 border-l border-gray-200">
      {#each entries as entry}
        <li>
          <a
            href="#{entry.id}"
            class="block pl-{entry.level === 2 ? 3 : 6} pr-2 py-1 border-l-2 -ml-px {activeId === entry.id
              ? 'border-pletka-primary text-pletka-primary font-medium'
              : 'border-transparent text-gray-500 hover:text-gray-900 hover:border-gray-400'}"
          >
            {entry.text}
          </a>
        </li>
      {/each}
    </ul>
  </aside>
{/if}

<style>
  .content-toc {
    z-index: 10;
  }
</style>
