import type { Component } from 'svelte';
import type { OntologyPageAction, OntologyPageSection } from '$lib/types/ontology-page';

import HeroSection from './sections/HeroSection.svelte';
import StatsStripSection from './sections/StatsStripSection.svelte';
import TabsSection from './sections/TabsSection.svelte';
import MetadataListSection from './sections/MetadataListSection.svelte';
import CardListSection from './sections/CardListSection.svelte';
import EntityListLinkSection from './sections/EntityListLinkSection.svelte';
import EntityListSection from './sections/EntityListSection.svelte';

export interface OntologySectionWidgetProps {
  section: OntologyPageSection;
  lang: string;
  runAction: (action: OntologyPageAction) => void | Promise<void>;
}

export type OntologySectionWidgetComponent = Component<OntologySectionWidgetProps>;

const registry = new Map<string, OntologySectionWidgetComponent>();

export function registerOntologySectionWidget(kind: string, component: OntologySectionWidgetComponent) {
  if (!kind) return;
  registry.set(kind, component);
}

export function getOntologySectionWidget(kind: string | undefined): OntologySectionWidgetComponent | null {
  if (!kind) return null;
  return registry.get(kind) ?? null;
}

registerOntologySectionWidget('hero', HeroSection);
registerOntologySectionWidget('stats-strip', StatsStripSection);
registerOntologySectionWidget('tabs', TabsSection);
registerOntologySectionWidget('metadata-list', MetadataListSection);
registerOntologySectionWidget('card-list', CardListSection);
registerOntologySectionWidget('callout', CardListSection);
registerOntologySectionWidget('entity-list-link', EntityListLinkSection);
registerOntologySectionWidget('entity-list', EntityListSection);
