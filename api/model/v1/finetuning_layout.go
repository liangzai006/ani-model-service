package modelv1

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Structure bounds are independent of the deployment's content-byte limit.
const (
	MaxFinetuningModelFiles      = 1024
	MaxFinetuningConfigBytes     = 8 << 20
	MaxFinetuningControlRPCBytes = 64 << 20
)

// FinetuningWeightLayout describes the standard safetensors entry point. Tensor
// names may legitimately share one shard; each inventory path remains unique.
type FinetuningWeightLayout struct {
	Kind, EntryPath string
	ShardPaths      []string
}

// InspectFinetuningWeightLayout validates actual inventory and index bytes. It
// has no authority to grant access, select a model or create missing files.
func InspectFinetuningWeightLayout(files []*FinetuningArtifactFile, indexJSON []byte) (FinetuningWeightLayout, error) {
	invalid := func() (FinetuningWeightLayout, error) {
		return FinetuningWeightLayout{}, fmt.Errorf("invalid safetensors weight layout")
	}
	if len(files) == 0 || len(files) > MaxFinetuningModelFiles {
		return invalid()
	}
	seen := map[string]*FinetuningArtifactFile{}
	for _, f := range files {
		if f == nil || !factsPath(f.Path) || seen[f.Path] != nil || f.SizeBytes < 0 || !factsDigest(f.Sha256) {
			return invalid()
		}
		seen[f.Path] = f
	}
	single, index := seen["model.safetensors"], seen["model.safetensors.index.json"]
	if (single == nil) == (index == nil) {
		return invalid()
	}
	if single != nil {
		if single.SizeBytes <= 0 || len(indexJSON) != 0 {
			return invalid()
		}
		for name := range seen {
			if strings.HasSuffix(name, ".safetensors") && name != "model.safetensors" {
				return invalid()
			}
		}
		return FinetuningWeightLayout{Kind: "SINGLE", EntryPath: "model.safetensors"}, nil
	}
	if len(indexJSON) == 0 || len(indexJSON) > MaxFinetuningConfigBytes || !factsJSONObject(indexJSON) {
		return invalid()
	}
	hash := sha256.Sum256(indexJSON)
	if index.SizeBytes != int64(len(indexJSON)) || index.Sha256 != hex.EncodeToString(hash[:]) {
		return invalid()
	}
	var document struct {
		WeightMap map[string]string `json:"weight_map"`
	}
	if json.Unmarshal(indexJSON, &document) != nil || len(document.WeightMap) == 0 {
		return invalid()
	}
	shards := map[string]bool{}
	for tensor, reference := range document.WeightMap {
		if tensor == "" || !factsPath(reference) || !strings.HasSuffix(reference, ".safetensors") {
			return invalid()
		}
		member := seen[reference]
		if member == nil || member.SizeBytes <= 0 {
			return invalid()
		}
		shards[reference] = true
	}
	for name := range seen {
		if strings.HasSuffix(name, ".safetensors") && !shards[name] {
			return invalid()
		}
	}
	result := FinetuningWeightLayout{Kind: "SHARDED", EntryPath: "model.safetensors.index.json"}
	for name := range shards {
		result.ShardPaths = append(result.ShardPaths, name)
	}
	sort.Strings(result.ShardPaths)
	return result, nil
}
