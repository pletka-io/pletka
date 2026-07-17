<script lang="ts">
  let { message = 'Loading...' }: { message?: string } = $props();
</script>

<div class="animate-pulse space-y-4 p-6">
  {#each Array(3) as _}
    <div class="border border-gray-200 rounded-lg p-4">
      <div class="flex items-center space-x-3">
        <div class="w-4 h-4 bg-gray-200 rounded-full"></div>
        <div class="h-4 bg-gray-200 rounded w-1/3"></div>
        <div class="h-4 bg-gray-200 rounded w-16 ml-auto"></div>
      </div>
    </div>
  {/each}
  <p class="text-sm text-gray-400 text-center">{message}</p>
</div>
