<script lang="ts">
  import { tr } from '$lib/types/form-schema';
  import type { OntologySectionWidgetProps } from '../section-registry';
  import { actionClass, badgeTone, visibleMetadata } from './shared';

  let { section, lang, runAction }: OntologySectionWidgetProps = $props();
</script>

<section class="rounded-lg bg-white p-6 shadow-sm ring-1 ring-gray-200">
  {#if tr(section.title, lang)}
    <h2 class="text-base font-semibold text-gray-900">{tr(section.title, lang)}</h2>
  {/if}
  {#if tr(section.description, lang)}
    <p class="mt-1 text-sm text-gray-500">{tr(section.description, lang)}</p>
  {/if}
  {#if section.cards && section.cards.length}
    <div class="mt-5 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      {#each section.cards as card (card.id || tr(card.title, lang))}
        <article class="rounded-md bg-gray-50 p-4 ring-1 ring-gray-200">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              {#if card.href}
                <a href={card.href} class="text-base font-semibold text-gray-900 hover:text-sky-700">{tr(card.title, lang)}</a>
              {:else}
                <h3 class="text-base font-semibold text-gray-900">{tr(card.title, lang)}</h3>
              {/if}
              {#if tr(card.subtitle, lang)}
                <p class="mt-1 text-sm font-medium text-gray-700">{tr(card.subtitle, lang)}</p>
              {/if}
            </div>
          </div>
          {#if card.badges && card.badges.length}
            <div class="mt-3 flex flex-wrap gap-2">
              {#each card.badges as badge, i (`${card.id}-badge-${i}`)}
                <span class="rounded-full px-2.5 py-1 text-xs font-medium {badgeTone(badge.tone)}">{tr(badge.label, lang)}</span>
              {/each}
            </div>
          {/if}
          {#if tr(card.description, lang)}
            <p class="mt-3 text-sm leading-6 text-gray-600">{tr(card.description, lang)}</p>
          {/if}
          {#if visibleMetadata(card).length}
            <dl class="mt-4 space-y-2">
              {#each visibleMetadata(card) as item, i (`${card.id}-meta-${i}`)}
                <div>
                  <dt class="text-xs text-gray-500">{tr(item.label, lang)}</dt>
                  <dd class="text-xs text-gray-700 {item.code ? 'break-all font-mono' : ''}">{item.value}</dd>
                </div>
              {/each}
            </dl>
          {/if}
          {#if card.stats && card.stats.length}
            <dl class="mt-4 grid grid-cols-3 gap-3 text-sm">
              {#each card.stats as stat, i (`${card.id}-stat-${i}`)}
                <div>
                  <dt class="text-gray-500">{tr(stat.label, lang)}</dt>
                  <dd class="mt-1 font-semibold text-gray-900">{stat.value}</dd>
                </div>
              {/each}
            </dl>
          {/if}
          {#if card.actions && card.actions.length}
            <div class="mt-4 flex flex-wrap gap-2">
              {#each card.actions as action (action.id)}
                <button
                  type="button"
                  class="inline-flex items-center rounded-md px-3 py-1.5 text-sm font-medium ring-1 transition-colors {actionClass(action.style)}"
                  onclick={() => runAction(action)}
                >
                  {tr(action.label, lang)}
                </button>
              {/each}
            </div>
          {/if}
        </article>
      {/each}
    </div>
  {:else if tr(section.empty_text, lang)}
    <p class="mt-4 rounded-md border border-dashed border-gray-300 bg-gray-50 p-4 text-sm text-gray-500">{tr(section.empty_text, lang)}</p>
  {/if}
</section>
