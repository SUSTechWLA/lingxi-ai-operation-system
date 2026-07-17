package model

import (
	"encoding/json"
	"testing"
)

func TestLegacyShotJSONRemainsReadableWithoutLayerPlan(t *testing.T) {
	var shot ShotUnit
	if err := json.Unmarshal([]byte(`{"id":"SHOT_01","durationSec":6,"startSec":0,"endSec":6,"reviewStatus":"pending"}`), &shot); err != nil {
		t.Fatal(err)
	}
	if shot.ID != "SHOT_01" || shot.DurationSec != 6 || shot.TalkingHeadLayers != nil {
		t.Fatalf("legacy shot compatibility = %+v", shot)
	}
}

func TestTalkingHeadLayerPlanRoundTripPreservesRevisionsAndDependencies(t *testing.T) {
	shot := ShotUnit{
		SchemaVersion: TalkingHeadSchemaVersion, ID: "SHOT_03", StartMs: 12000, EndMs: 18000, DurationMs: 6000, TimelineRevision: "audio-r2",
		TalkingHeadLayers: &TalkingHeadShotLayers{
			SchemaVersion: TalkingHeadSchemaVersion, TimelineRevision: "audio-r2", VisualMode: VisualModeIPWithBrollPIP,
			Audio: AudioLayerPlan{State: LayerArtifactState{SchemaVersion: TalkingHeadSchemaVersion, Layer: ShotLayerAudio, Status: LayerStatusCurrent, Revision: "audio-layer-r2", OutputFingerprint: "sha256:audio"}, AudioMasterRevision: "audio-r2"},
			Broll: BrollLayerPlan{State: LayerArtifactState{SchemaVersion: TalkingHeadSchemaVersion, Layer: ShotLayerBroll, Status: LayerStatusCurrent, Revision: "broll-r3", Dependencies: []ArtifactDependencyRef{{Layer: string(ShotLayerAudio), Revision: "audio-r2"}}}},
		},
	}
	data, err := json.Marshal(shot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ShotUnit
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.TalkingHeadLayers == nil || decoded.TalkingHeadLayers.Audio.AudioMasterRevision != "audio-r2" || decoded.TalkingHeadLayers.Broll.State.Dependencies[0].Revision != "audio-r2" {
		t.Fatalf("round-trip lost layer contract: %s", data)
	}
}

func TestIPAssetPackReferencePinsVersionAndContentHash(t *testing.T) {
	pack := IPAssetPack{SchemaVersion: TalkingHeadSchemaVersion, IPAssetPackRef: IPAssetPackRef{ID: "ip-tangying", Version: "2.1.0", ContentHash: "sha256:asset-pack"}, DisplayName: "Tangying"}
	data, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	var decoded IPAssetPack
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID != pack.ID || decoded.Version != pack.Version || decoded.ContentHash != pack.ContentHash {
		t.Fatalf("IP asset pack pin was not preserved: %s", data)
	}
}
