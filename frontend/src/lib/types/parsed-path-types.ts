// Hand-maintained mirror of cmd/airtable-import-v2/path_parser.go.
// The parser is command-local, so tygo no longer owns this file.
import type { PathElement } from './path-types';
export type { PathElement };

//////////
// source: path_parser.go
/**
 * ScopeInfo represents structured scope information
 */
export interface ScopeInfo {
  uri: string;
  prefix: string;
  class_code: string;
  local_name: string;
}
/**
 * ParsedPath represents a fully parsed ontology path
 */
export interface ParsedPath {
  elements: PathElement[];
  prefix_map?: { [key: string]: string}; // Known prefix to namespace mappings
  raw_path: string; // Original path string
  scope?: string; // Ontology scope (e.g., "E21_Person") — kept for backward compat
  scope_info?: ScopeInfo; // Structured scope information
  terminal_type?: string; // "literal", "class", "uri"
  short_path?: string; // Rebuilt from elements without prefixes
  long_path?: string; // Rebuilt from elements with prefixes
  version: number /* int */; // 2 for new format, 0 for legacy
}
/**
 * PathParser parses ontology paths into structured elements
 */
export interface PathParser {
}
