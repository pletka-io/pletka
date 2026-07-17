<script lang="ts">
  import type { BadgeConfig } from '$lib/types/entity-list-schema';
  import { tr } from '$lib/types/form-schema';

  let {
    config,
    item,
    projectId,
    lang,
  }: {
    config: BadgeConfig;
    item: Record<string, any>;
    projectId: string;
    lang: string;
  } = $props();

  const styleColors: Record<string, string> = {
    purple: 'bg-purple-50 text-purple-700',
    blue: 'bg-blue-50 text-blue-700',
    green: 'bg-green-50 text-green-700',
    gray: 'bg-gray-100 text-gray-600',
    amber: 'bg-amber-50 text-amber-700',
    red: 'bg-red-50 text-red-700',
    indigo: 'bg-indigo-50 text-indigo-700',
    slate: 'bg-slate-100 text-slate-700 border border-slate-200',
  };

  const statusColors: Record<string, string> = {
    draft: 'bg-gray-100 text-gray-600',
    published: 'bg-green-50 text-green-700',
    released: 'bg-green-50 text-green-700',
    modified: 'bg-amber-50 text-amber-700',
    new: 'bg-blue-50 text-blue-700',
    'in process': 'bg-blue-50 text-blue-700',
    active: 'bg-blue-50 text-blue-700',
    deprecated: 'bg-red-50 text-red-700',
    inactive: 'bg-red-50 text-red-700',
  };

  function getStatusIcon(status: string): string {
    switch (status) {
      case 'draft': return '✎';
      case 'published': case 'released': return '✓';
      case 'modified': return '●';
      case 'new': return '+';
      case 'in process': case 'active': return '◎';
      case 'deprecated': case 'inactive': return '⊘';
      default: return '';
    }
  }

  // Ownership badge reads two server-supplied fields:
  //   - origin_kind:  "own" | "forked" | "adopted" | "inherited" (discriminator only)
  //   - origin_label: Translations map ({en: "...", nl: "...", ...}) the
  //                   server resolved via i18n.LocalizedText.
  // No domain wording lives in this component — schema-driven UI rule.
  // origin_kind drives the color; origin_label provides the text.
  function originKind(): string {
    return String(item.origin_kind ?? 'own');
  }

  let visible = $derived.by(() => {
    if (config.type === 'ownership') {
      // Show "Own" pill for local rows too, since that's the default
      // styling. Hide only when origin_kind is missing entirely.
      return item.origin_kind !== undefined && item.origin_kind !== null;
    }
    if (config.type === 'status') return !!item[config.key];
    if (config.type === 'count') {
      const val = Number(item[config.key] || 0);
      return !(config.hide_empty && val === 0);
    }
    if (config.type === 'text') {
      return !!item[config.key];
    }
    return !!item[config.key];
  });

  let badgeClass = $derived.by(() => {
    const base = 'inline-flex items-center px-2 py-0.5 rounded text-xs font-medium';
    if (config.type === 'status') {
      const status = String(item[config.key] || '').toLowerCase();
      return `${base} ${statusColors[status] || styleColors.gray}`;
    }
    if (config.type === 'ownership') {
      // Color by origin kind only. Frontend never knows what the label
      // text says — that's the server's job via origin_label
      // translations. Each origin state gets its own color so the
      // legend reads at a glance — UX review item 2.
      switch (originKind()) {
        case 'forked':            return `${base} ${styleColors.amber}`;
        case 'adopted':           return `${base} ${styleColors.indigo}`;
        case 'adopted_reference': return `${base} ${styleColors.slate}`;
        case 'inherited':         return `${base} ${styleColors.blue}`;
        default:                  return `${base} ${styleColors.green}`;
      }
    }
    return `${base} ${styleColors[config.style || 'gray'] || styleColors.gray}`;
  });

  let badgeText = $derived.by(() => {
    if (config.type === 'text') {
      return String(item[config.key] || '');
    }
    // "flag" renders a fixed label (with a ⊘ marker) only when the keyed
    // boolean is truthy — used for the Deprecated pill, which is separate
    // from the status badge since the two are orthogonal.
    if (config.type === 'flag') {
      return `⊘ ${tr(config.label, lang)}`;
    }
    if (config.type === 'status') {
      const status = String(item[config.key] || '');
      const icon = getStatusIcon(status.toLowerCase());
      return icon ? `${icon} ${status}` : status;
    }
    if (config.type === 'ownership') {
      // origin_label is a Translations map after the server's
      // LocalizedText walker ran. Server emits a label for every kind
      // (own / forked / adopted / inherited) so the frontend has no
      // domain wording. tr() falls back to en if the user's lang
      // isn't in the map.
      const label = item.origin_label;
      if (label && typeof label === 'object') {
        return tr(label, lang);
      }
      return '';
    }
    if (config.type === 'count') {
      const val = Number(item[config.key] || 0);
      const label = tr(config.label, lang);
      return `${val} ${label}`;
    }
    return '';
  });
</script>

{#if visible}
  <span class={badgeClass}>{badgeText}</span>
{/if}
