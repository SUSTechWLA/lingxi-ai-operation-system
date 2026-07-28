package main

import (
	"reflect"
	"testing"

	"github.com/tangying-ai/aios-core/internal/core/config"
)

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

func TestCloudObservabilitySourceIdentityIsIndependentFromRuntimeValidationMode(t *testing.T) {
	if got := runtimeMode("release", "production"); got != "production" {
		t.Fatalf("runtime validation mode = %q", got)
	}
	source, sealing, err := cloudObservabilitySourceIdentity(config.ObservabilityConfig{
		SealingKey:                 "key-not-inspected-here",
		SealingDomain:              "domain-v1",
		SourceEnvironment:          "development",
		PreviousSourceEnvironments: "legacy-beta",
	})
	if err != nil {
		t.Fatal(err)
	}
	if source.Environment != "development" || source.Service != "cloud-backend" || source.Component != "http-server" {
		t.Fatalf("signed source = %#v", source)
	}
	if sealing.Domain != "domain-v1" || sealing.Key != "key-not-inspected-here" ||
		!reflect.DeepEqual(sealing.PreviousSourceEnvironments, []string{"legacy-beta"}) {
		t.Fatalf("sealing config = %#v", sealing)
	}
}
