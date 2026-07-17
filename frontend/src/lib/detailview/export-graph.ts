export type PathElement = {
  type?: string;
  uri?: string;
  prefix?: string;
  local_name?: string;
  class_code?: string;
  instance_id?: string;
};

export type ResourceEdge = {
  predicate?: PathElement;
  target_key: string;
  field_ids?: string[];
};

export type LiteralBinding = {
  predicate?: PathElement;
  field: FieldBinding;
};

export type FieldBinding = {
  id: string;
  semantic_id?: string;
  system_name?: string;
  label?: Record<string, string>;
  relative_path?: string;
  node_key?: string;
  group_keys?: string[];
};

export type GroupBinding = {
  key: string;
  kind: string;
};

export type SourcePathBinding = {
  field_id?: string;
  path?: string;
};

export type TypeRef = {
  uri?: string;
  prefix?: string;
  local_name?: string;
};

export type ResourceNode = {
  key: string;
  class: PathElement;
  identity_source?: string;
  instance_ids?: string[];
  edges?: ResourceEdge[];
  literals?: LiteralBinding[];
  fields?: FieldBinding[];
  groups?: GroupBinding[];
  source_paths?: SourcePathBinding[];
  additional_types?: TypeRef[];
};

export type VisualGroup = {
  key: string;
  kind: string;
  id?: string;
  semantic_id?: string;
  system_name?: string;
  label?: Record<string, string>;
  order?: number;
  path?: string;
  parent_key?: string;
  branch_key?: string;
  path_prefix?: PathElement[];
  children?: string[];
  field_ids?: string[];
};

export type ExportGraph = {
  root_key: string;
  nodes: ResourceNode[];
  groups?: VisualGroup[];
  fields?: FieldBinding[];
};
