import type { OntologyPageCard, OntologyPageSection } from '$lib/types/ontology-page';

export function statTone(tone = '') {
  switch (tone) {
    case 'green': return 'bg-emerald-50 text-emerald-800 ring-emerald-100';
    case 'purple': return 'bg-violet-50 text-violet-800 ring-violet-100';
    case 'amber': return 'bg-amber-50 text-amber-800 ring-amber-100';
    default: return 'bg-sky-50 text-sky-800 ring-sky-100';
  }
}

export function badgeTone(tone = '') {
  switch (tone) {
    case 'green': return 'bg-emerald-50 text-emerald-700';
    case 'amber': return 'bg-amber-50 text-amber-700';
    case 'purple': return 'bg-violet-50 text-violet-700';
    default: return 'bg-slate-100 text-slate-700';
  }
}

export function actionClass(style = '') {
  if (style === 'primary') return 'bg-sky-700 text-white hover:bg-sky-800 ring-sky-700';
  if (style === 'danger') return 'bg-red-700 text-white hover:bg-red-800 ring-red-700';
  return 'bg-white text-gray-700 hover:bg-gray-50 ring-gray-300';
}

export function visibleMetadata(card: OntologyPageCard) {
  return (card.metadata ?? []).filter((m) => m.value || m.href);
}

export function visibleLinks(section: OntologyPageSection) {
  return (section.links ?? []).filter((l) => l.href);
}
