package config

import "testing"

func TestBackendScopeValidation(t *testing.T) {
	for _, tc := range []struct {
		scope            string
		stateless, valid bool
	}{
		{"", false, true}, {"session", false, true}, {"shared", true, true},
		{"shared", false, false}, {"", true, false}, {"session", true, false}, {"typo", true, false},
	} {
		c := Config{Host: "test", Auth: Auth{TokenEnv: "TEST_TOKEN"}, Servers: map[string]Server{
			"test": {Command: "fixture", Tools: []string{"safe"}, BackendScope: tc.scope, Stateless: tc.stateless},
		}}
		if err := c.Validate(); (err == nil) != tc.valid {
			t.Errorf("scope=%q stateless=%v: %v", tc.scope, tc.stateless, err)
		}
	}
}
