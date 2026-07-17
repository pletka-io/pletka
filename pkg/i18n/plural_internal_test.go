package i18n

import "testing"

func TestPluralForms(t *testing.T) {
	tests := []struct {
		lang     string
		count    int
		expected PluralForm
	}{
		{"en", 0, PluralOther},
		{"en", 1, PluralOne},
		{"en", 2, PluralOther},
		{"en", 5, PluralOther},

		{"pl", 1, PluralOne},
		{"pl", 2, PluralFew},
		{"pl", 3, PluralFew},
		{"pl", 4, PluralFew},
		{"pl", 5, PluralMany},
		{"pl", 11, PluralMany},
		{"pl", 12, PluralMany},
		{"pl", 22, PluralFew},

		{"ar", 0, PluralZero},
		{"ar", 1, PluralOne},
		{"ar", 2, PluralTwo},
		{"ar", 3, PluralFew},
		{"ar", 10, PluralFew},
		{"ar", 11, PluralMany},
		{"ar", 99, PluralMany},
		{"ar", 100, PluralOther},
	}

	for _, test := range tests {
		result := getPluralForm(test.lang, test.count)
		if result != test.expected {
			t.Errorf("getPluralForm(%s, %d) = %v, expected %v",
				test.lang, test.count, result, test.expected)
		}
	}
}
