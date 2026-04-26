// Entry point for the Pletka renderer.
//
// Mount model B (single root per page):
//   1. Read the JSON page schema from the data attribute on #pletka-root.
//   2. Mount one PageRenderer component, fed that schema.
//   3. Navigation is full-page from the Go server; this script is loaded
//      fresh on every navigation. There is no client-side router.

import { mount } from "svelte";
import "./app.css";
import PageRenderer from "./PageRenderer.svelte";
import type { PageSchema } from "./types/schema";

const root = document.getElementById("pletka-root");
if (!root) {
  throw new Error("pletka renderer: #pletka-root not found in document");
}

const raw = root.getAttribute("data-page-schema");
if (!raw) {
  throw new Error("pletka renderer: data-page-schema attribute is missing");
}

let schema: PageSchema;
try {
  schema = JSON.parse(raw) as PageSchema;
} catch (err) {
  throw new Error(
    "pletka renderer: data-page-schema is not valid JSON: " + String(err),
  );
}

mount(PageRenderer, {
  target: root,
  props: { schema },
});
