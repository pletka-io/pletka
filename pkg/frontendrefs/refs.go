package frontendrefs

// The marker helpers return plain strings so schema structs and templates stay
// simple, while AST-based conformance checks can still find emitted frontend
// names reliably.

func GlobalEntry(name string) string {
	return name
}

func Island(name string) string {
	return name
}

func FormWidget(name string) string {
	return name
}

func EntityListRowWidget(name string) string {
	return name
}

func EntityListEditorWidget(name string) string {
	return name
}

func ContentWidget(name string) string {
	return name
}
