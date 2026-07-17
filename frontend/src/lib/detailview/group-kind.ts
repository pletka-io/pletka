import type { Translations } from '$lib/types/weave-types';

export const DIRECT_FIELDS_ID = '__direct__';
export const DIRECT_FIELDS_NAME: Translations = { en: 'Direct Fields' };
export const FIELD_GROUP_WIDGET = 'field-group';
export const COLLECTION_GROUP_WIDGET = 'collection-group';

export type DetailGroupWidget = typeof FIELD_GROUP_WIDGET | typeof COLLECTION_GROUP_WIDGET;

export interface DetailGroupLike {
  widget: string;
  id?: string;
}

export function isDirectFieldGroup(item: DetailGroupLike): boolean {
  return item.widget === FIELD_GROUP_WIDGET;
}

export function isCollectionGroup(item: DetailGroupLike): boolean {
  return item.widget === COLLECTION_GROUP_WIDGET;
}

export function isDirectFieldsBucket(item: DetailGroupLike): boolean {
  return isDirectFieldGroup(item) && item.id === DIRECT_FIELDS_ID;
}

export function groupWidgetForCollectionState(inCollectionGroup: boolean): DetailGroupWidget {
  return inCollectionGroup ? COLLECTION_GROUP_WIDGET : FIELD_GROUP_WIDGET;
}

export function collectionIDForGroup(item: DetailGroupLike): string | undefined {
  return isCollectionGroup(item) ? item.id : undefined;
}
