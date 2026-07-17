<script lang="ts">
  import type { PathSuggestion } from '$lib/types/ontology-types';

  let { suggestion, selected = false }: {
    suggestion: PathSuggestion;
    selected: boolean;
  } = $props();

  const isClass = $derived(
    suggestion.type === 'class' || suggestion.type === 'property-class'
  );

  const typeLabel = $derived(
    isClass ? 'Class' : suggestion.type === 'object_property' ? 'Object Property' : 'Property'
  );

  function localName(uri: string): string {
    const hash = uri.lastIndexOf('#');
    if (hash >= 0) return uri.slice(hash + 1);
    const slash = uri.lastIndexOf('/');
    if (slash >= 0) return uri.slice(slash + 1);
    return uri;
  }

  function truncate(text: string, max: number): string {
    return text.length > max ? text.slice(0, max) + '...' : text;
  }
</script>

<div class="text-xs space-y-1.5">
  <!-- Header -->
  <div class="font-semibold">{suggestion.local_name}</div>

  {#if suggestion.label?.en && suggestion.label.en !== suggestion.local_name}
    <div class="text-gray-400">{suggestion.label.en}</div>
  {/if}

  <!-- Type + Ontology badges -->
  <div class="flex flex-wrap gap-1">
    <span class="px-1.5 py-0.5 bg-gray-700 rounded text-xs">{typeLabel}</span>
    {#if suggestion.ontology_info}
      <span class="px-1.5 py-0.5 bg-indigo-600 rounded text-xs">
        {suggestion.ontology_info.ontology_prefix || suggestion.prefix}
      </span>
    {:else if suggestion.prefix}
      <span class="px-1.5 py-0.5 bg-indigo-600 rounded text-xs">{suggestion.prefix}</span>
    {/if}
  </div>

  <!-- Comment/scope note -->
  {#if suggestion.comment?.en}
    <div class="text-gray-300 leading-relaxed">
      {truncate(suggestion.comment.en, 200)}
    </div>
  {/if}

  <!-- Domain -> Range (properties only) -->
  {#if !isClass && (suggestion.domain_classes?.length || suggestion.range_classes?.length)}
    <div class="border-t border-gray-700 pt-1.5 mt-1.5">
      <div class="text-gray-400 mb-0.5">Domain &rarr; Range</div>
      <div class="font-mono">
        {(suggestion.domain_classes ?? []).map(d => localName(d)).join(', ') || '\u2014'}
        &rarr;
        {(suggestion.range_classes ?? []).map(r => localName(r)).join(', ') || '\u2014'}
      </div>
    </div>
  {/if}

  <!-- Superclass hierarchy (classes only) -->
  {#if isClass && suggestion.super_classes?.length}
    <div class="border-t border-gray-700 pt-1.5 mt-1.5">
      <div class="text-amber-400 mb-0.5">Extends</div>
      <div>
        <span class="text-white">{suggestion.local_name}</span>
        {#each suggestion.super_classes.slice(0, 4) as sc}
          <span class="text-gray-400"> &rarr; {sc.local_name}</span>
        {/each}
        {#if suggestion.super_classes.length > 4}
          <span class="text-gray-500"> +{suggestion.super_classes.length - 4} more</span>
        {/if}
      </div>
    </div>
  {/if}

  <!-- Subclasses (classes only) -->
  {#if isClass && suggestion.sub_classes?.length}
    <div class="border-t border-gray-700 pt-1.5 mt-1.5">
      <div class="text-teal-400 mb-0.5">Inherited by ({suggestion.sub_classes.length})</div>
      <div class="text-gray-300">
        {#each suggestion.sub_classes.slice(0, 5) as sc}
          <span class="text-teal-300">{sc.local_name}</span>{' '}
        {/each}
        {#if suggestion.sub_classes.length > 5}
          <span class="text-gray-500">+{suggestion.sub_classes.length - 5} more</span>
        {/if}
      </div>
    </div>
  {/if}

  <!-- Class inheritance (for properties) -->
  {#if suggestion.inherited_from_class}
    <div class="border-t border-gray-700 pt-1.5 mt-1.5">
      <div class="text-amber-400 mb-0.5">Class Inheritance</div>
      <div>
        Inherited from <span class="font-semibold">{suggestion.inherited_from_class}</span>
        {#if suggestion.class_inheritance_depth}
          <span class="text-gray-400">
            ({suggestion.class_inheritance_depth === 1 ? 'parent'
              : suggestion.class_inheritance_depth === 2 ? 'grandparent'
              : `${suggestion.class_inheritance_depth} levels up`})
          </span>
        {/if}
      </div>
    </div>
  {/if}

  <!-- Project inheritance -->
  {#if suggestion.is_inherited && suggestion.source_project}
    <div class="border-t border-gray-700 pt-1.5 mt-1.5">
      <div class="text-blue-400 mb-0.5">Project Inheritance</div>
      <div>
        From <span class="font-semibold">{suggestion.source_project.project_name}</span>
        {#if suggestion.inheritance_level}
          <span class="text-gray-400">
            ({suggestion.inheritance_level === 1 ? 'parent project'
              : suggestion.inheritance_level === 2 ? 'grandparent project'
              : `${suggestion.inheritance_level} levels up`})
          </span>
        {/if}
      </div>
    </div>
  {/if}

  <!-- URI -->
  {#if suggestion.uri}
    <div class="border-t border-gray-700 pt-1.5 mt-1.5 text-gray-500 break-all">{suggestion.uri}</div>
  {/if}
</div>
