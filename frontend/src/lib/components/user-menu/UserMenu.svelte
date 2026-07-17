<!-- frontend/src/lib/components/user-menu/UserMenu.svelte -->
<script lang="ts">
  /**
   * UserMenu renders the role-aware dropdown for the logged-in user.
   * The schema (data-prop-menu) is built server-side by
   * pkg/weave/usermenu and carries the items already permission-gated.
   * This component is a generic renderer — no domain knowledge,
   * walks `items` and dispatches by `type`.
   *
   * Triggered by clicking the user-name button. Closes on outside
   * click and Escape. Arrow-key navigation when open.
   */
  import { tr } from '$lib/types/weave-types';
  import { getUILang } from '$lib/utils/locale';

  type Translations = Record<string, string>;

  interface MenuItem {
    type: 'link' | 'divider' | 'danger';
    label?: Translations;
    href?: string;
    icon?: string;
    active?: boolean;
    external?: boolean;
    badge?: number;
  }

  interface Menu {
    display_name: string;
    email?: string;
    avatar_url?: string;
    items: MenuItem[];
  }

  let { menu }: { menu: Menu } = $props();
  const lang = getUILang();

  let open = $state(false);
  let triggerEl: HTMLButtonElement | undefined = $state();
  let panelEl: HTMLDivElement | undefined = $state();

  function toggle() {
    open = !open;
  }

  function close() {
    open = false;
  }

  function onWindowClick(e: MouseEvent) {
    if (!open) return;
    const target = e.target as Node | null;
    if (!target) return;
    if (triggerEl?.contains(target)) return;
    if (panelEl?.contains(target)) return;
    close();
  }

  function onKey(e: KeyboardEvent) {
    if (!open) return;
    if (e.key === 'Escape') {
      e.preventDefault();
      close();
      triggerEl?.focus();
    }
  }
</script>

<svelte:window onclick={onWindowClick} onkeydown={onKey} />

<div class="relative inline-block text-left">
  <button
    bind:this={triggerEl}
    type="button"
    class="inline-flex items-center gap-1 text-gray-600 hover:text-pletka-primary focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-pletka-primary rounded-md px-2 py-1"
    aria-haspopup="menu"
    aria-expanded={open}
    onclick={toggle}
  >
    <span>{menu.display_name}</span>
    <svg class="h-4 w-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
    </svg>
  </button>

  {#if open}
    <div
      bind:this={panelEl}
      class="absolute right-0 mt-1 w-56 origin-top-right rounded-md bg-white shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none z-50 py-1"
      role="menu"
      tabindex="-1"
    >
      {#if menu.email}
        <div class="px-4 py-2 text-xs text-gray-400 truncate" title={menu.email}>{menu.email}</div>
      {/if}
      {#each menu.items as item, i (i)}
        {#if item.type === 'divider'}
          <hr class="my-1 border-gray-100" />
        {:else if item.type === 'link' || item.type === 'danger'}
          {@const label = tr(item.label || {}, lang, '')}
          <a
            href={item.href}
            target={item.external ? '_blank' : undefined}
            rel={item.external ? 'noopener noreferrer' : undefined}
            role="menuitem"
            class="flex items-center gap-2 px-4 py-2 text-sm hover:bg-gray-50 transition-colors
              {item.type === 'danger' ? 'text-red-600 hover:bg-red-50' : 'text-gray-700'}
              {item.active ? 'font-semibold' : ''}"
            onclick={close}
          >
            {#if item.icon}
              <span class="inline-block w-4 text-gray-400">
                <!-- Icon dispatch — server names the icon, frontend
                     renders a small inline SVG from a known set. New
                     icons land here as needed; unknown names fall back
                     to a small dot so layout doesn't shift. -->
                {#if item.icon === 'user'}
                  <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"/></svg>
                {:else if item.icon === 'cog'}
                  <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/></svg>
                {:else if item.icon === 'plus'}
                  <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/></svg>
                {:else if item.icon === 'shield'}
                  <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4M7.835 4.697a3.42 3.42 0 001.946-.806 3.42 3.42 0 014.438 0 3.42 3.42 0 001.946.806 3.42 3.42 0 013.138 3.138 3.42 3.42 0 00.806 1.946 3.42 3.42 0 010 4.438 3.42 3.42 0 00-.806 1.946 3.42 3.42 0 01-3.138 3.138 3.42 3.42 0 00-1.946.806 3.42 3.42 0 01-4.438 0 3.42 3.42 0 00-1.946-.806 3.42 3.42 0 01-3.138-3.138 3.42 3.42 0 00-.806-1.946 3.42 3.42 0 010-4.438 3.42 3.42 0 00.806-1.946 3.42 3.42 0 013.138-3.138z"/></svg>
                {:else if item.icon === 'lock'}
                  <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 11c0-1.657-1.343-3-3-3s-3 1.343-3 3v4h6v-4zm6 10H6a2 2 0 01-2-2v-6a2 2 0 012-2h12a2 2 0 012 2v6a2 2 0 01-2 2z"/></svg>
                {:else if item.icon === 'logout'}
                  <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"/></svg>
                {:else}
                  <span class="inline-block h-1.5 w-1.5 rounded-full bg-gray-400"></span>
                {/if}
              </span>
            {/if}
            <span class="flex-1">{label}</span>
            {#if item.badge != null && item.badge > 0}
              <span class="inline-flex items-center justify-center min-w-[1rem] h-4 px-1 text-[10px] rounded-full bg-pletka-primary text-white">{item.badge}</span>
            {/if}
          </a>
        {/if}
      {/each}
    </div>
  {/if}
</div>
