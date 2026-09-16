<!-- frontend/src/lib/components/project/ProjectHeader.svelte -->
<script lang="ts">
  import type { ProjectPageEntity, ProjectPageNavLink, ProjectReleaseView } from '$lib/types/project-page';
  import { tr } from '$lib/types/weave-types';
  import { getUILang } from '$lib/utils/locale';
  import Badge from '$lib/components/shared/Badge.svelte';

  let {
    entity,
    release,
    navLinks = [],
  }: {
    entity: ProjectPageEntity;
    release?: ProjectReleaseView;
    navLinks?: ProjectPageNavLink[];
  } = $props();

  const lang = getUILang();
  const name = $derived(tr(entity.name, lang, entity.id));
  const releaseLabel = $derived(release ? tr(release.label, lang, release.version) : '');

  function statusVariant(s: string): 'gray' | 'green' | 'yellow' | 'amber' | 'blue' {
    switch (s) {
      case 'published': return 'green';
      case 'modified': return 'amber';
      case 'new': return 'blue';
      case 'draft': return 'gray';
      case 'deprecated': return 'yellow';
      default: return 'gray';
    }
  }
  const unreleased = $derived(entity.unreleased_changes ?? 0);
</script>

<div class="mb-6">
  <div class="flex items-start justify-between gap-4 mb-2">
    <div class="flex items-center gap-3 min-w-0">
      <h1 class="text-3xl font-bold text-gray-900 truncate">{name}</h1>
      <span class="text-sm text-gray-500 font-mono">{entity.id}</span>
      {#if entity.status}
        <Badge variant={statusVariant(entity.status)} size="sm">{entity.status}</Badge>
      {/if}
      {#if unreleased > 0}
        <Badge variant="amber" size="sm">{unreleased} unreleased change{unreleased === 1 ? '' : 's'}</Badge>
      {/if}
      {#if release}
        <Badge variant="blue" size="sm">{release.version}</Badge>
      {/if}
    </div>
    {#if navLinks.length > 0}
      <nav class="flex items-center gap-2 shrink-0">
        {#each navLinks as link (link.href)}
          <a
            href={link.href}
            class="inline-flex items-center gap-1.5 rounded-md border border-gray-200 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 hover:text-gray-900 focus:ring-2 focus:ring-pletka-primary focus:ring-offset-2"
          >
            {#if link.icon === 'cog'}
              <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/>
                <path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>
              </svg>
            {/if}
            {tr(link.label, lang, '')}
          </a>
        {/each}
      </nav>
    {/if}
  </div>
</div>

{#if release}
  <div class="mb-6 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-blue-200 bg-blue-50 px-4 py-3">
    <div class="flex items-center gap-3">
      <Badge variant="blue" size="sm">release</Badge>
      <div>
        <div class="text-sm font-medium text-blue-900">{releaseLabel}</div>
        <div class="text-xs text-blue-700">This page is showing a read-only snapshot.</div>
      </div>
    </div>
    {#if release.draft_url}
      <a
        href={release.draft_url}
        class="inline-flex items-center rounded-md border border-blue-300 bg-white px-3 py-1.5 text-sm font-medium text-blue-800 hover:bg-blue-100"
      >
        Return to draft
      </a>
    {/if}
  </div>
{/if}
