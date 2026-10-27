package modelv1

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestRecommendedImageWireCompatibility(t *testing.T) {
	for _, tc := range []struct {
		message   proto.Message
		id, scope protoreflect.FieldNumber
	}{
		{&ModelVersion{}, 17, 18},
		{&CreateModelVersionRequest{}, 16, 17},
		{&PrepareTrainedArtifactRequest{}, 18, 19},
		{&ImportModelBundleVersionData{}, 6, 7},
	} {
		fields := tc.message.ProtoReflect().Descriptor().Fields()
		if fields.ByName("recommended_image_id").Number() != tc.id || fields.ByName("recommended_image_scope").Number() != tc.scope {
			t.Fatalf("unexpected field numbers for %T", tc.message)
		}
		if err := protojson.Unmarshal([]byte(`{"engine_image":"registry/runtime:old"}`), tc.message); err != nil {
			t.Fatal(err)
		}
		if tc.message.ProtoReflect().Get(fields.ByName("recommended_image_id")).String() != "" || tc.message.ProtoReflect().Get(fields.ByName("recommended_image_scope")).String() != "" {
			t.Fatalf("legacy defaults changed for %T", tc.message)
		}
		if err := protojson.Unmarshal([]byte(`{"engine_image":"registry/runtime@sha256:fixed","recommended_image_id":"image-a","recommended_image_scope":"tenant"}`), tc.message); err != nil {
			t.Fatal(err)
		}
		encoded, err := proto.Marshal(tc.message)
		if err != nil {
			t.Fatal(err)
		}
		decoded := tc.message.ProtoReflect().New().Interface()
		if err := proto.Unmarshal(encoded, decoded); err != nil || !proto.Equal(tc.message, decoded) {
			t.Fatalf("round trip %T: %v", tc.message, err)
		}
	}
}
