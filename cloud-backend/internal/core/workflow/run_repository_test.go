package workflow

import (
	"reflect"
	"testing"
)

func TestStageStatusJSONBPathUsesSingleJSONKey(t *testing.T) {
	got := stageStatusJSONBPath("recording_script")
	want := []string{"recording_script"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected jsonb path: got %#v want %#v", got, want)
	}
}
