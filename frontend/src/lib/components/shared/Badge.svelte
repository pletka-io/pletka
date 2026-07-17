<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    variant = 'gray',
    size = 'sm',
    children,
  }: {
    variant?: 'gray' | 'green' | 'blue' | 'purple' | 'yellow' | 'red' | 'indigo' | 'amber';
    size?: 'xs' | 'sm';
    children: Snippet;
  } = $props();

  const colors: Record<string, string> = {
    gray: 'bg-gray-100 text-gray-800',
    green: 'bg-green-100 text-green-800',
    blue: 'bg-blue-100 text-blue-800',
    purple: 'bg-purple-100 text-purple-800',
    yellow: 'bg-yellow-100 text-yellow-800',
    red: 'bg-red-100 text-red-800',
    indigo: 'bg-indigo-100 text-indigo-800',
    amber: 'bg-amber-100 text-amber-800',
  };

  const sizes: Record<string, string> = {
    xs: 'px-1.5 py-0.5 text-xs',
    sm: 'px-2 py-0.5 text-xs',
  };
</script>

<span class="inline-flex items-center rounded-full font-medium {colors[variant]} {sizes[size]}">
  {@render children()}
</span>
