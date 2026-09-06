package config

import "testing"

func TestMetadataPolicyValidation(t *testing.T) {
	for _, tc := range []struct {
		limit int
		tool  string
		valid bool
	}{
		{0, "tool", true}, {64, "tool", true}, {4096, "tool", true},
		{63, "tool", false}, {-1, "tool", false}, {4097, "tool", false},
		{64, DescribeToolName, false}, {0, DescribeToolName, true},
	} {
		c := Config{Host: "test", Auth: Auth{TokenEnv: "unused"}, Servers: map[string]Server{
			"fixture": {Command: "unused", Tools: []string{tc.tool}, DescriptionLimit: tc.limit}}}
		if err := c.Validate(); (err == nil) != tc.valid {
			t.Errorf("%+v: %v", tc, err)
		}
	}
}
