import type { Component } from 'svelte';
import type { ProjectOverviewSection } from '$lib/types/project-page';

import ReadmeSection from './overview-widgets/ReadmeSection.svelte';
import DescriptionSection from './overview-widgets/DescriptionSection.svelte';
import OntologiesSection from './overview-widgets/OntologiesSection.svelte';
import CreditsSection from './overview-widgets/CreditsSection.svelte';
import StatsCardsSection from './overview-widgets/StatsCardsSection.svelte';
import InfoSidebarSection from './overview-widgets/InfoSidebarSection.svelte';

export interface ProjectOverviewWidgetProps {
  section: ProjectOverviewSection;
  lang: string;
}

export type ProjectOverviewRegion = 'main' | 'sidebar';
export type ProjectOverviewWidgetComponent = Component<ProjectOverviewWidgetProps>;

export interface ProjectOverviewWidgetRegistration {
  component: ProjectOverviewWidgetComponent;
  region: ProjectOverviewRegion;
}

const registry = new Map<string, ProjectOverviewWidgetRegistration>();

export function registerProjectOverviewWidget(
  kind: string,
  component: ProjectOverviewWidgetComponent,
  region: ProjectOverviewRegion = 'main',
) {
  if (!kind) return;
  registry.set(kind, { component, region });
}

export function getProjectOverviewWidget(kind: string | undefined): ProjectOverviewWidgetRegistration | null {
  if (!kind) return null;
  return registry.get(kind) ?? null;
}

registerProjectOverviewWidget('readme', ReadmeSection);
registerProjectOverviewWidget('description', DescriptionSection);
registerProjectOverviewWidget('ontologies', OntologiesSection);
registerProjectOverviewWidget('stats-cards', StatsCardsSection);
registerProjectOverviewWidget('credits', CreditsSection);
registerProjectOverviewWidget('info-sidebar', InfoSidebarSection, 'sidebar');
