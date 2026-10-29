package service

import (
	"context"
	"errors"
	"strconv"

	kratoserrors "github.com/go-kratos/kratos/v3/errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	modelv1 "github.com/liangzai006/ani-model-service/api/model/v1"
	"github.com/liangzai006/ani-model-service/internal/biz/model"
	"github.com/liangzai006/ani-model-service/internal/identity"
)

func (s *ModelService) ResolveFinetuningFacts(ctx context.Context, in *modelv1.ResolveFinetuningFactsRequest) (*modelv1.ResolveFinetuningFactsResponse, error) {
	if in == nil {
		return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "finetuning facts request is required")
	}
	p, err := identity.RequireTenant(ctx, in.GetTenantId())
	if err != nil {
		return nil, mapIdentity(err)
	}
	if len(in.ProtoReflect().GetUnknown()) != 0 {
		return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "unknown source request fields")
	}
	version, err := uuid.Parse(in.GetBaseModelVersionId())
	if err != nil || version == uuid.Nil {
		return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "one fixed base version is required")
	}
	switch target := in.Target.(type) {
	case nil:
	case *modelv1.ResolveFinetuningFactsRequest_TargetModelId:
		if target.TargetModelId == "" {
			return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "target model must not be empty")
		}
	case *modelv1.ResolveFinetuningFactsRequest_NewModelName:
		if target.NewModelName == "" {
			return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "new model name must not be empty")
		}
	default:
		return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "unknown model target")
	}
	targetID := in.GetTargetModelId()
	if targetID != "" {
		id, e := uuid.Parse(targetID)
		if e != nil || id == uuid.Nil {
			return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "target must be an internal model UUID")
		}
		targetID = id.String()
	}
	contents, _ := s.storage.(model.FinetuningArtifactReader)
	source, _ := s.reader.(model.FinetuningSourceReader)
	facts, err := model.ResolveFinetuningFacts(ctx, p.TenantID, version.String(), targetID, in.GetNewModelName(), source, s.catalog, contents)
	if err != nil {
		var limit *model.FinetuningLimitViolation
		if errors.As(err, &limit) {
			return nil, kratoserrors.New(422, limit.Reason, "model content or structure exceeds explicit bound").WithMetadata(map[string]string{"kind": "base_model", "limit_name": limit.LimitName, "actual": strconv.FormatInt(limit.Actual, 10), "limit": strconv.FormatInt(limit.Limit, 10)})
		}
		switch {
		case errors.Is(err, context.Canceled):
			return nil, kratoserrors.New(499, "CANCELED", "model facts read was canceled")
		case errors.Is(err, context.DeadlineExceeded):
			return nil, kratoserrors.New(504, "DEADLINE_EXCEEDED", "model facts read deadline exceeded")
		case errors.Is(err, model.ErrFinetuningSourceForbidden):
			return nil, kratoserrors.Forbidden("MODEL_SOURCE_FORBIDDEN", "source access denied")
		case errors.Is(err, pgx.ErrNoRows):
			return nil, kratoserrors.NotFound("MODEL_SOURCE_NOT_FOUND", "source version is not visible in the tenant catalogue")
		case errors.Is(err, model.ErrFinetuningBaseInvalid):
			return nil, kratoserrors.New(422, "BASE_MODEL_INVALID", "model archive or weight layout is invalid")
		case errors.Is(err, model.ErrFinetuningTargetConflict):
			return nil, kratoserrors.Conflict("MODEL_NAME_CONFLICT", "target model name is already used")
		case errors.Is(err, model.ErrInvalidModel):
			return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "invalid model name")
		case errors.Is(err, model.ErrModelNotReady):
			return nil, kratoserrors.New(409, "MODEL_NOT_READY", "base version is not an eligible READY safetensors archive")
		default:
			return nil, kratoserrors.New(503, "MODEL_DEPENDENCY_UNAVAILABLE", "fixed source and target facts could not be verified")
		}
	}
	a := facts.Artifact
	out := &modelv1.ResolveFinetuningFactsResponse{TenantId: p.TenantID, ModelId: facts.Version.ModelID, ModelVersionId: facts.Version.ID, Format: facts.Version.Format, Bucket: a.Bucket, ObjectKey: a.Key, ObjectVersionId: a.VersionID, SizeBytes: a.SizeBytes, Sha256: a.SHA256, ConfigJson: a.ConfigJSON, TokenizerConfigJson: a.TokenizerConfigJSON, WeightIndexJson: a.WeightIndexJSON, VerificationSource: modelv1.FinetuningVerificationSource}
	if facts.TargetModelID != "" {
		out.Target = &modelv1.ResolveFinetuningFactsResponse_TargetModelId{TargetModelId: facts.TargetModelID}
	} else if facts.NewModelName != "" {
		out.Target = &modelv1.ResolveFinetuningFactsResponse_NewModelName{NewModelName: facts.NewModelName}
	}
	for _, file := range a.Files {
		out.Files = append(out.Files, &modelv1.FinetuningArtifactFile{Path: file.Path, SizeBytes: file.SizeBytes, Sha256: file.SHA256})
	}
	normalized := &modelv1.ResolveFinetuningFactsRequest{TenantId: p.TenantID, BaseModelVersionId: version.String()}
	if targetID != "" {
		normalized.Target = &modelv1.ResolveFinetuningFactsRequest_TargetModelId{TargetModelId: targetID}
	} else if in.GetNewModelName() != "" {
		normalized.Target = &modelv1.ResolveFinetuningFactsRequest_NewModelName{NewModelName: in.GetNewModelName()}
	}
	if out.ValidateFor(normalized) != nil {
		return nil, kratoserrors.New(422, "BASE_MODEL_INVALID", "source archive members failed integrity validation")
	}
	return out, nil
}
