/**
 * Block-type → widget-component map. Each widget receives one prop:
 * `block` (the block.data payload). Adding a new block type =
 * register one entry here + drop the .svelte file in widgets/.
 *
 * Mirrors the form-schema WidgetDispatcher pattern.
 */

import type { Component } from 'svelte';

import Hero from './widgets/Hero.svelte';
import Prose from './widgets/Prose.svelte';
import FeatureGrid from './widgets/FeatureGrid.svelte';
import Principles from './widgets/Principles.svelte';
import CTAStrip from './widgets/CTAStrip.svelte';
import Quote from './widgets/Quote.svelte';
import Toc from './widgets/Toc.svelte';
import Actions from './widgets/Actions.svelte';
import Steps from './widgets/Steps.svelte';

export const widgetRegistry: Record<string, Component<any>> = {};

export function registerContentWidget(kind: string, component: Component<any>) {
  if (!kind) return;
  widgetRegistry[kind] = component;
}

export function registerContentWidgets(entries: Record<string, Component<any>>) {
  for (const [kind, component] of Object.entries(entries)) {
    registerContentWidget(kind, component);
  }
}

registerContentWidgets({
  hero: Hero,
  prose: Prose,
  feature_grid: FeatureGrid,
  principles: Principles,
  cta_strip: CTAStrip,
  quote: Quote,
  toc: Toc,
  actions: Actions,
  steps: Steps,
});
