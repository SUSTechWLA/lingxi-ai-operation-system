package main

import "testing"

func TestRuntimeModeCannotBypassProductionValidationWithMissingGinMode(t *testing.T) {
	for _, test := range []struct {
		ginMode string
		appEnv  string
		want    string
	}{
		{ginMode: "release", want: "production"},
		{appEnv: "production", want: "production"},
		{appEnv: "prod", want: "production"},
		{appEnv: "release", want: "production"},
		{want: "development"},
	} {
		if got := runtimeMode(test.ginMode, test.appEnv); got != test.want {
			t.Fatalf("runtimeMode(%q, %q) = %q, want %q", test.ginMode, test.appEnv, got, test.want)
		}
	}
}
