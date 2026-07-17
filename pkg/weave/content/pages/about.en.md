---
slug: about
template: article
nav:
  label: About
  order: 2
  visible: true
seo:
  title: About Pletka
  description: A tool for building, sharing and reusing semantic data patterns in support of LOUDER semantic data.
blocks:
  - type: hero
    title:    t:pages.about.title
    subtitle: t:pages.about.subtitle

  - type: prose
    heading: t:pages.about.service_title
    body: |
      t:pages.about.service_text

  - type: prose
    heading: t:pages.about.who_title
    body: |
      t:pages.about.who_text

  - type: prose
    heading: t:pages.about.method_title
    body: |
      t:pages.about.method_text

  - type: feature_grid
    heading: t:pages.about.tools_title
    columns: 3
    items:
      - title: t:pages.about.ontologies_title
        body:  t:pages.about.ontologies_text
      - title: t:pages.about.fields_title
        body:  t:pages.about.fields_text
      - title: t:pages.about.collections_title
        body:  t:pages.about.collections_text
      - title: t:pages.about.models_title
        body:  t:pages.about.models_text
      - title: t:pages.about.categories_title
        body:  t:pages.about.categories_text
      - title: t:pages.about.weave_title
        body:  t:pages.about.weave_text

  - type: prose
    heading: t:pages.about.overrides_title
    body: |
      t:pages.about.overrides_text

      t:pages.about.overrides_layers

  - type: prose
    heading: t:pages.about.reuse_title
    body: |
      t:pages.about.reuse_text

  - type: prose
    heading: t:pages.about.generators_title
    body: |
      t:pages.about.generators_text

  - type: actions
    items:
      - label: t:pages.about.explore_patterns
        href:  /projects
        style: secondary
      - label: t:pages.about.explore_ontologies
        href:  /ontologies
        style: primary
---
