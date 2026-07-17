package aat

import (
	"strconv"
	"strings"
)

func searchQuery(query, lang string, limit int, parentURI string) string {
	if lang == "" {
		lang = "en"
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := strings.ToLower(query)
	parentFilter := ""
	if parentURI = sparqlURI(parentURI); parentURI != "" {
		parentFilter = "\n  ?subject gvp:broaderPreferredExtended <" + escapeSPARQLIRI(parentURI) + "> ."
	}
	queryFilter := ""
	if q != "" {
		queryFilter = "\n  FILTER(CONTAINS(LCASE(STR(?label)), \"" + escapeSPARQLString(q) + `"))`
	}
	return `
PREFIX gvp: <http://vocab.getty.edu/ontology#>
PREFIX skos: <http://www.w3.org/2004/02/skos/core#>
SELECT ?subject ?label ?scopeNote ?broader ?broaderLabel ?parentString WHERE {
  ?subject skos:inScheme <http://vocab.getty.edu/aat/> ;
           skos:prefLabel ?label .` + parentFilter + `
  FILTER(LANGMATCHES(LANG(?label), "` + escapeSPARQLString(lang) + `"))` + queryFilter + `
  OPTIONAL { ?subject skos:scopeNote ?scopeNote . FILTER(LANGMATCHES(LANG(?scopeNote), "` + escapeSPARQLString(lang) + `")) }
  OPTIONAL { ?subject gvp:parentString ?parentString . FILTER(LANG(?parentString) = "" || LANGMATCHES(LANG(?parentString), "` + escapeSPARQLString(lang) + `")) }
  OPTIONAL {
    ?subject gvp:broaderPreferred ?broader .
    OPTIONAL {
      ?broader skos:prefLabel ?broaderLabel .
      FILTER(LANGMATCHES(LANG(?broaderLabel), "` + escapeSPARQLString(lang) + `"))
    }
  }
}
ORDER BY LCASE(STR(?label))
LIMIT ` + strconv.Itoa(limit)
}

func fetchQuery(uri, lang string) string {
	if lang == "" {
		lang = "en"
	}
	return `
PREFIX gvp: <http://vocab.getty.edu/ontology#>
PREFIX skos: <http://www.w3.org/2004/02/skos/core#>
SELECT ?subject ?label ?scopeNote ?broader ?broaderLabel ?parentString WHERE {
  BIND(<` + escapeSPARQLIRI(sparqlURI(uri)) + `> AS ?subject)
  ?subject skos:prefLabel ?label .
  FILTER(LANGMATCHES(LANG(?label), "` + escapeSPARQLString(lang) + `"))
  OPTIONAL { ?subject skos:scopeNote ?scopeNote . FILTER(LANGMATCHES(LANG(?scopeNote), "` + escapeSPARQLString(lang) + `")) }
  OPTIONAL { ?subject gvp:parentString ?parentString . FILTER(LANG(?parentString) = "" || LANGMATCHES(LANG(?parentString), "` + escapeSPARQLString(lang) + `")) }
  OPTIONAL {
    ?subject gvp:broaderPreferred ?broader .
    OPTIONAL {
      ?broader skos:prefLabel ?broaderLabel .
      FILTER(LANGMATCHES(LANG(?broaderLabel), "` + escapeSPARQLString(lang) + `"))
    }
  }
}
LIMIT 1`
}

func escapeSPARQLString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func escapeSPARQLIRI(value string) string {
	value = strings.ReplaceAll(value, `>`, `%3E`)
	value = strings.ReplaceAll(value, `<`, `%3C`)
	return value
}
