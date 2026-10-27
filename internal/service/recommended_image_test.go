package service

import (
	"context"
	"testing"

	kratoserrors "github.com/go-kratos/kratos/v3/errors"
	modelv1 "github.com/liangzai006/ani-model-service/api/model/v1"
	"github.com/liangzai006/ani-model-service/internal/biz/model"
	"github.com/liangzai006/ani-model-service/internal/identity"
	"google.golang.org/protobuf/proto"
)

func TestPrepareTrainedArtifactFreezesInheritedImageForReplay(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "catalog", true: "legacy"}[legacy], func(t *testing.T) {
			reads := 0
			reader := versionReaderFunc(func(context.Context, string, string) (model.Version, error) {
				reads++
				if reads > 1 {
					t.Fatal("replay re-read mutable source runtime")
				}
				v := model.Version{Status: model.VersionReady, EngineType: "vllm", EngineImage: "registry/vllm@sha256:frozen", StartupCommand: "serve /models", RecommendedImageID: "image-a", RecommendedImageScope: "platform"}
				if legacy {
					v.RecommendedImageID, v.RecommendedImageScope = "", ""
				}
				return v, nil
			})
			registrar := &trainedArtifactRegistrarFake{}
			s := NewModelServiceWithDependencies(reader, nil, nil, &trainedArtifactStorageFake{}, nil)
			s.SetTrainedArtifactRegistrar(registrar)
			in := trainedArtifactRequest()
			in.EngineType, in.EngineImage, in.StartupCommand = "", "", ""
			in.SourceModelVersionId = "source-version"
			first, err := s.PrepareTrainedArtifact(trainedArtifactContext(), in)
			if err != nil {
				t.Fatal(err)
			}
			if registrar.session.EngineImage != "registry/vllm@sha256:frozen" {
				t.Fatalf("runtime=%+v", registrar.session)
			}
			if !legacy && (registrar.session.RecommendedImageID != "image-a" || registrar.session.RecommendedImageScope != "platform") {
				t.Fatalf("selection=%+v", registrar.session)
			}
			if legacy && (registrar.session.RecommendedImageID != "" || registrar.session.RecommendedImageScope != "") {
				t.Fatal("legacy source acquired a catalog identity")
			}
			second, err := s.PrepareTrainedArtifact(trainedArtifactContext(), in)
			if err != nil || first.GetRegistrationId() != second.GetRegistrationId() || reads != 1 {
				t.Fatalf("replay=%+v err=%v reads=%d", second, err, reads)
			}
			changed := proto.Clone(in).(*modelv1.PrepareTrainedArtifactRequest)
			changed.SourceModelVersionId = "different-source"
			if _, err := s.PrepareTrainedArtifact(trainedArtifactContext(), changed); kratoserrors.Code(err) != 409 {
				t.Fatalf("source change=%v", err)
			}
		})
	}
}

func TestPrepareTrainedArtifactRecommendedImageSelectionConflictAndValidation(t *testing.T) {
	registrar := &trainedArtifactRegistrarFake{}
	s := NewModelServiceWithDependencies(nil, nil, nil, &trainedArtifactStorageFake{}, nil)
	s.SetTrainedArtifactRegistrar(registrar)
	in := trainedArtifactRequest()
	in.RecommendedImageId, in.RecommendedImageScope = "image-a", "tenant"
	if _, err := s.PrepareTrainedArtifact(trainedArtifactContext(), in); err != nil {
		t.Fatal(err)
	}
	in.RecommendedImageId = "image-b"
	if _, err := s.PrepareTrainedArtifact(trainedArtifactContext(), in); kratoserrors.Code(err) != 409 {
		t.Fatalf("changed image=%v", err)
	}
	in.RecommendedImageScope = ""
	if _, err := s.PrepareTrainedArtifact(trainedArtifactContext(), in); kratoserrors.Code(err) != 400 {
		t.Fatalf("incomplete pair=%v", err)
	}
}

func TestRecommendedImageReturnedInVersionProjection(t *testing.T) {
	v := toProtoVersion(model.Version{ID: "version", RecommendedImageID: "image-a", RecommendedImageScope: "tenant", EngineImage: "registry/vllm@sha256:frozen"})
	if v.GetRecommendedImageId() != "image-a" || v.GetRecommendedImageScope() != "tenant" || v.GetEngineImage() != "registry/vllm@sha256:frozen" {
		t.Fatalf("version=%+v", v)
	}
	// Existing consumers may continue to omit both new request fields.
	if (&modelv1.PrepareTrainedArtifactRequest{}).GetRecommendedImageId() != "" {
		t.Fatal("legacy request default changed")
	}
}

func TestCreateModelVersionSavesRecommendedImageAndRejectsPartialSelection(t *testing.T) {
	versions := &versionCatalogFake{}
	s := NewModelServiceWithDependencies(versions, versions, &referenceCatalog{}, nil, nil)
	ctx := identity.WithPrincipal(context.Background(), identity.Principal{TenantID: "tenant-a", Actor: "test", Workload: "test"})
	in := &modelv1.CreateModelVersionRequest{TenantId: "tenant-a", ModelId: "Qwen3-32B", Version: "v1", Format: "gguf", EngineType: "vllm", EngineImage: "registry/vllm@sha256:frozen", StartupCommand: "serve /models", RecommendedImageId: "image-a", RecommendedImageScope: "platform"}
	got, err := s.CreateModelVersion(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if versions.created.RecommendedImageID != "image-a" || versions.created.RecommendedImageScope != "platform" || got.GetRecommendedImageId() != "image-a" || got.GetRecommendedImageScope() != "platform" {
		t.Fatalf("created=%+v response=%+v", versions.created, got)
	}
	in.RecommendedImageScope = ""
	if _, err := s.CreateModelVersion(ctx, in); kratoserrors.Code(err) != 400 {
		t.Fatalf("partial selection=%v", err)
	}
}

func TestImportModelBundleSavesRecommendedImageAndRejectsPartialSelection(t *testing.T) {
	importer := &bundleImporterFake{}
	s := NewModelService(nil)
	s.SetBundleImporter(importer)
	ctx := identity.WithPrincipal(context.Background(), identity.Principal{TenantID: "tenant-a", Actor: "test", Workload: "test"})
	in := bundleImportRequest("tenant-a")
	in.Version.RecommendedImageId, in.Version.RecommendedImageScope = "image-a", "tenant"
	if _, err := s.ImportModelBundle(ctx, in); err != nil {
		t.Fatal(err)
	}
	if importer.input.RecommendedImageID != "image-a" || importer.input.RecommendedImageScope != "tenant" {
		t.Fatalf("bundle=%+v", importer.input)
	}
	in.Version.RecommendedImageId = ""
	if _, err := s.ImportModelBundle(ctx, in); kratoserrors.Code(err) != 400 {
		t.Fatalf("partial selection=%v", err)
	}
}
