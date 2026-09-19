package client

import (
	"testing"

	modelv1 "github.com/liangzai006/ani-model-service/api/model/v1"
)

func TestRuntimeModelMappingLeavesEmptyCommandArgvEmpty(t *testing.T) {
	v, err := runtimeModelFromVersion(&modelv1.ModelVersion{Id: "v", ModelId: "m", Status: "ready", StoragePath: "tenant/m/v/model.gguf", ChecksumSha256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", EngineType: "vllm"})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.CommandArgv) != 0 {
		t.Fatalf("command argv = %#v", v.CommandArgv)
	}
}

func TestRuntimeModelMappingUsesCommandThenArgs(t *testing.T) {
	v, err := runtimeModelFromVersion(&modelv1.ModelVersion{Id: "v", ModelId: "m", Status: "ready", StoragePath: "object://model", ChecksumSha256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", StartupCommand: "python", StartupArgs: []string{"serve", "--port", "8080"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"python", "serve", "--port", "8080"}
	if len(v.CommandArgv) != len(want) {
		t.Fatalf("command argv = %#v", v.CommandArgv)
	}
	for i := range want {
		if v.CommandArgv[i] != want[i] {
			t.Fatalf("command argv = %#v", v.CommandArgv)
		}
	}
}
