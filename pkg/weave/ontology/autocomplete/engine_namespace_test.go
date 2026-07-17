package autocomplete

import (
	"context"
	"strings"
	"testing"
)

func TestQnameResolverLiteralSuggestionUsesNamespaceBindings(t *testing.T) {
	resolver := qnameResolver{
		namespaceBindings: func(context.Context) ([]NamespaceBinding, error) {
			return []NamespaceBinding{
				{Prefix: "xsd", Namespace: "http://www.w3.org/2001/XMLSchema#"},
			}, nil
		},
	}

	got, err := resolver.literalSuggestion(context.Background(), "http://www.w3.org/2001/XMLSchema#string")
	if err != nil {
		t.Fatalf("literalSuggestion: %v", err)
	}
	if got.Qname != "xsd:string" {
		t.Fatalf("Qname = %q; want xsd:string", got.Qname)
	}
	if got.Datatype != "xsd:string" {
		t.Fatalf("Datatype = %q; want xsd:string", got.Datatype)
	}
}

func TestQnameResolverLiteralSuggestionRequiresNamespaceBinding(t *testing.T) {
	resolver := qnameResolver{
		namespaceBindings: func(context.Context) ([]NamespaceBinding, error) {
			return []NamespaceBinding{
				{Prefix: "crm", Namespace: "http://www.cidoc-crm.org/cidoc-crm/"},
			}, nil
		},
	}

	_, err := resolver.literalSuggestion(context.Background(), "https://missing.example.org/types#Value")
	if err == nil {
		t.Fatal("expected missing namespace error")
	}
	if !strings.Contains(err.Error(), "missing namespace binding") {
		t.Fatalf("error = %q; want missing namespace binding", err)
	}
}
