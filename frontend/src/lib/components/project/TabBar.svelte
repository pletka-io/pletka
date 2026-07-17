<!-- frontend/src/lib/components/project/TabBar.svelte -->
<script lang="ts">
  import type { ProjectPageTab } from '$lib/types/project-page';
  import { tr } from '$lib/types/weave-types';
  import { getUILang } from '$lib/utils/locale';

  let {
    tabs,
    activeTabId,
    onTabClick,
    isActiveBranch,
  }: {
    tabs: ProjectPageTab[];
    activeTabId: string;
    onTabClick: (tabId: string) => void;
    isActiveBranch: (tab: ProjectPageTab) => boolean;
  } = $props();

  const lang = getUILang();

  // Icon paths — SVG path `d` values for each icon key.
  const iconPaths: Record<string, string> = {
    home: 'M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6',
    cube: 'M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4',
    collection: 'M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10',
    list: 'M4 6h16M4 12h16M4 18h7',
    bookmark: 'M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-4-7 4V5z',
    clock: 'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z',
    download: 'M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4',
    cog: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z M15 12a3 3 0 11-6 0 3 3 0 016 0z',
    share: 'M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.368 2.684 3 3 0 00-5.368-2.684z',
    'document-text': 'M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z',
  };

  function iconPath(key: string): string {
    return iconPaths[key] || iconPaths.list;
  }

  const leftTabs = $derived(tabs.filter((t) => t.align !== 'right'));
  const rightTabs = $derived(tabs.filter((t) => t.align === 'right'));

  /**
   * The active branch's children, rendered as a second row underneath
   * the primary tab bar. Null when the active tab has no parent
   * (Overview, or any tab the user clicked outright).
   */
  const activeBranch = $derived(leftTabs.find((t) => isActiveBranch(t)) ?? null);
  const subTabs = $derived(activeBranch?.children ?? []);

  function chipClasses(tab: ProjectPageTab, active: boolean, muted = false): string {
    if (tab.disabled) {
      return 'text-gray-300 border-transparent cursor-not-allowed';
    }
    if (muted) {
      return active
        ? 'text-gray-700 border-gray-400'
        : 'text-gray-400 border-transparent hover:text-gray-600';
    }
    return active
      ? 'text-blue-600 border-blue-600'
      : 'text-gray-500 border-transparent hover:text-gray-700';
  }

  function breakdownText(tab: ProjectPageTab): string {
    if (!tab.breakdown || tab.breakdown.length === 0) return '';
    return tab.breakdown.join('·');
  }

  function clickTab(tab: ProjectPageTab) {
    if (tab.disabled) return;
    onTabClick(tab.id);
  }
</script>

<div class="border-b border-gray-200 mb-6 bg-white">
  <nav class="flex items-center" aria-label="Tabs">
    <div class="flex space-x-4">
      {#each leftTabs as tab (tab.id)}
        <button
          type="button"
          class="px-4 py-3 font-medium text-sm border-b-2 transition-colors {chipClasses(tab, isActiveBranch(tab))}"
          disabled={tab.disabled}
          title={tab.tooltip || ''}
          onclick={() => clickTab(tab)}
        >
          <svg class="inline-block w-5 h-5 mr-2 -mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={iconPath(tab.icon)} />
          </svg>
          {tr(tab.label, lang, tab.id)}
          {#if tab.children && tab.children.length}
            {#if tab.breakdown && tab.breakdown.length}
              <span class="ml-2 px-2 py-0.5 rounded-full text-xs bg-gray-100 text-gray-700 font-mono">
                {breakdownText(tab)}
              </span>
            {/if}
            <svg class="inline-block w-3 h-3 ml-1 opacity-60" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
            </svg>
          {:else if tab.count !== undefined && tab.count > 0}
            <span class="ml-2 px-2 py-0.5 rounded-full text-xs bg-blue-100 text-blue-800">
              {tab.count}
            </span>
          {/if}
        </button>
      {/each}
    </div>

    {#if rightTabs.length > 0}
      <div class="ml-auto flex items-center gap-2 pr-2">
        <div class="h-6 w-px bg-gray-200 mx-1" aria-hidden="true"></div>
        {#each rightTabs as tab (tab.id)}
          {#if tab.href}
            <a
              href={tab.href}
              class="px-3 py-3 font-medium text-sm border-b-2 transition-colors {chipClasses(tab, false, true)}"
              title={tab.tooltip || ''}
            >
              <svg class="inline-block w-4 h-4 mr-1.5 -mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={iconPath(tab.icon)} />
              </svg>
              {tr(tab.label, lang, tab.id)}
            </a>
          {:else}
            <button
              type="button"
              class="px-3 py-3 font-medium text-sm border-b-2 transition-colors {chipClasses(tab, isActiveBranch(tab), true)}"
              disabled={tab.disabled}
              onclick={() => clickTab(tab)}
            >
              <svg class="inline-block w-4 h-4 mr-1.5 -mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={iconPath(tab.icon)} />
              </svg>
              {tr(tab.label, lang, tab.id)}
            </button>
          {/if}
        {/each}
      </div>
    {/if}
  </nav>

  {#if subTabs.length > 0}
    <nav class="flex flex-wrap items-center gap-2 pl-3 pb-3 pt-2" aria-label="Sub-tabs">
      {#each subTabs as sub (sub.id)}
        <button
          type="button"
          class="px-3 py-1.5 rounded-md text-xs font-medium transition-colors
            {sub.disabled
              ? 'text-gray-300 cursor-not-allowed'
              : activeTabId === sub.id
                ? 'bg-blue-50 text-blue-700'
                : 'text-gray-500 hover:bg-gray-50 hover:text-gray-700'}"
          disabled={sub.disabled}
          title={sub.tooltip || ''}
          onclick={() => clickTab(sub)}
        >
          {tr(sub.label, lang, sub.id)}
          {#if sub.count !== undefined && sub.count > 0}
            <span class="ml-1 text-xs opacity-70">{sub.count}</span>
          {/if}
        </button>
      {/each}
    </nav>
  {/if}
</div>
