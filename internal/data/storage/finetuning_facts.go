package storage

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	modelv1 "github.com/liangzai006/ani-model-service/api/model/v1"
	"io"
	"math"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/liangzai006/ani-model-service/internal/biz/model"
	"github.com/minio/minio-go/v7"
)

const maxFinetuningMetadataBytes int64 = 16 << 20
const maxFinetuningEntries = 4096
const maxFinetuningRelativePathBytes = 1024
const finetuningTarOverhead int64 = 512*maxFinetuningEntries + 511*modelv1.MaxFinetuningModelFiles + maxFinetuningMetadataBytes + 1024

// ConfigureFinetuningContentLimit is deployment-only. An omitted/invalid limit
// cannot silently enable the source reader with a second product default.
func (a *MinIOAdapter) ConfigureFinetuningContentLimit(limit int64) error {
	if a == nil || limit <= 0 || limit > math.MaxInt64-finetuningTarOverhead {
		return model.ErrFinetuningFactsUnavailable
	}
	a.finetuningContentLimit = limit
	return nil
}

var _ model.FinetuningArtifactReader = (*MinIOAdapter)(nil)

func (a *MinIOAdapter) InspectFinetuningArtifact(ctx context.Context, tenant, reference string, size int64, digest string) (model.ArtifactInventory, error) {
	if err := ctx.Err(); err != nil {
		return model.ArtifactInventory{}, err
	}
	if a == nil || a.client == nil || size <= 0 || a.finetuningContentLimit <= 0 || a.finetuningContentLimit > math.MaxInt64-finetuningTarOverhead || len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
		return model.ArtifactInventory{}, model.ErrFinetuningFactsUnavailable
	}
	if size > a.finetuningContentLimit+finetuningTarOverhead {
		return model.ArtifactInventory{}, archiveReadBoundExceeded(size, a.finetuningContentLimit)
	}
	key, err := tenantKey(tenant, reference)
	if err != nil {
		return model.ArtifactInventory{}, err
	}
	bucket, err := a.bucketForTenant(tenant)
	if err != nil {
		return model.ArtifactInventory{}, err
	}
	info, err := a.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return model.ArtifactInventory{}, finetuningProviderError(ctx, err)
	}
	if info.Size != size || info.VersionID == "" || strings.EqualFold(info.VersionID, "null") {
		return model.ArtifactInventory{}, model.ErrFinetuningFactsUnavailable
	}
	object, err := a.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{VersionID: info.VersionID})
	if err != nil {
		return model.ArtifactInventory{}, finetuningProviderError(ctx, err)
	}
	defer object.Close()
	// Start the GET before Stat: calling Stat on an unread MinIO Object issues
	// another HEAD and caches its metadata instead of the actual GET response.
	var first [1]byte
	if _, err := io.ReadFull(object, first[:]); err != nil {
		return model.ArtifactInventory{}, finetuningProviderError(ctx, err)
	}
	actual, err := object.Stat()
	if err != nil {
		return model.ArtifactInventory{}, finetuningProviderError(ctx, err)
	}
	if actual.VersionID != info.VersionID || actual.Size != size {
		return model.ArtifactInventory{}, model.ErrFinetuningFactsUnavailable
	}
	inventory, err := inspectFinetuningArchive(ctx, io.MultiReader(bytes.NewReader(first[:]), object), size, digest, a.finetuningContentLimit)
	if err != nil {
		return model.ArtifactInventory{}, err
	}
	inventory.Bucket, inventory.Key, inventory.VersionID = bucket, key, info.VersionID
	return inventory, nil
}

func inspectFinetuningArchive(ctx context.Context, body io.Reader, size int64, digest string, limit int64) (model.ArtifactInventory, error) {
	if err := ctx.Err(); err != nil {
		return model.ArtifactInventory{}, err
	}
	if limit <= 0 || limit > math.MaxInt64-finetuningTarOverhead || size <= 0 {
		return model.ArtifactInventory{}, model.ErrFinetuningFactsUnavailable
	}
	if size > limit+finetuningTarOverhead {
		return model.ArtifactInventory{}, archiveReadBoundExceeded(size, limit)
	}
	hash := sha256.New()
	readBound := size
	if readBound < math.MaxInt64 {
		readBound++
	}
	measured := &inventoryCountingReader{ctx: ctx, reader: io.TeeReader(io.LimitReader(body, readBound), hash)}
	archive := tar.NewReader(measured)
	result := model.ArtifactInventory{SizeBytes: size, SHA256: digest}
	seen := map[string]bool{}
	expanded := int64(0)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return model.ArtifactInventory{}, measured.archiveReadError(err, size)
		}
		if measured.count-expanded > finetuningTarOverhead {
			return model.ArtifactInventory{}, archiveOverheadExceeded(measured.count - expanded)
		}
		name := header.Name
		if header.Typeflag == tar.TypeDir {
			name = strings.TrimSuffix(name, "/")
		}
		if len(seen) >= maxFinetuningEntries {
			return model.ArtifactInventory{}, &model.FinetuningLimitViolation{Reason: "STRUCTURE_LIMIT_EXCEEDED", LimitName: "max_archive_entries", Actual: int64(len(seen)) + 1, Limit: maxFinetuningEntries}
		}
		if len(name) > maxFinetuningRelativePathBytes {
			return model.ArtifactInventory{}, &model.FinetuningLimitViolation{Reason: "STRUCTURE_LIMIT_EXCEEDED", LimitName: "max_relative_path_bytes", Actual: int64(len(name)), Limit: maxFinetuningRelativePathBytes}
		}
		if !utf8.ValidString(name) || name == "" || path.Clean(name) != name || strings.HasPrefix(name, "/") || name == "." || name == ".." || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\\\x00\r\n") || strings.IndexFunc(name, func(r rune) bool { return r < 32 || r == 127 }) >= 0 || seen[name] || header.Linkname != "" {
			return model.ArtifactInventory{}, model.ErrFinetuningBaseInvalid
		}
		seen[name] = true
		if header.Typeflag == tar.TypeDir && header.Size == 0 {
			continue
		}
		for key := range header.PAXRecords {
			if strings.HasPrefix(key, "GNU.sparse") {
				return model.ArtifactInventory{}, model.ErrFinetuningBaseInvalid
			}
		}
		if header.Size > math.MaxInt64-expanded {
			return model.ArtifactInventory{}, model.ErrFinetuningBaseInvalid
		}
		if header.Size > limit-expanded {
			return model.ArtifactInventory{}, &model.FinetuningLimitViolation{Reason: "CONTENT_LIMIT_EXCEEDED", LimitName: "max_content_bytes", Actual: expanded + header.Size, Limit: limit}
		}
		if len(result.Files) >= modelv1.MaxFinetuningModelFiles {
			return model.ArtifactInventory{}, &model.FinetuningLimitViolation{Reason: "STRUCTURE_LIMIT_EXCEEDED", LimitName: "max_model_files", Actual: int64(len(result.Files)) + 1, Limit: modelv1.MaxFinetuningModelFiles}
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA || header.Size < 0 || header.Size > limit-expanded || len(result.Files) >= modelv1.MaxFinetuningModelFiles {
			return model.ArtifactInventory{}, model.ErrFinetuningBaseInvalid
		}
		expanded += header.Size
		memberHash := sha256.New()
		var content bytes.Buffer
		output := io.Writer(memberHash)
		capture := name == "config.json" || name == "tokenizer_config.json" || name == "model.safetensors.index.json"
		if capture {
			if header.Size > modelv1.MaxFinetuningConfigBytes {
				return model.ArtifactInventory{}, &model.FinetuningLimitViolation{Reason: "STRUCTURE_LIMIT_EXCEEDED", LimitName: "max_model_config_bytes", Actual: header.Size, Limit: modelv1.MaxFinetuningConfigBytes}
			}
			output = io.MultiWriter(memberHash, &content)
		}
		count, err := io.Copy(output, archive)
		if err != nil {
			return model.ArtifactInventory{}, measured.archiveReadError(err, size)
		}
		if count != header.Size {
			return model.ArtifactInventory{}, model.ErrFinetuningBaseInvalid
		}
		result.Files = append(result.Files, model.ArtifactFile{Path: name, SizeBytes: count, SHA256: hex.EncodeToString(memberHash.Sum(nil))})
		if capture {
			if !utf8.Valid(content.Bytes()) || !json.Valid(content.Bytes()) {
				return model.ArtifactInventory{}, model.ErrFinetuningBaseInvalid
			}
			if name == "config.json" {
				result.ConfigJSON = content.Bytes()
			} else if name == "tokenizer_config.json" {
				result.TokenizerConfigJSON = content.Bytes()
			} else {
				result.WeightIndexJSON = content.Bytes()
			}
		}
	}
	// tar EOF is not stream EOF. Require any remaining padding to be zero and
	// hash all original archive bytes, detecting concatenated or appended data.
	padding := make([]byte, 32*1024)
	for {
		n, err := measured.Read(padding)
		for _, b := range padding[:n] {
			if b != 0 {
				return model.ArtifactInventory{}, model.ErrFinetuningBaseInvalid
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return model.ArtifactInventory{}, measured.archiveReadError(err, size)
		}
	}
	if measured.count-expanded > finetuningTarOverhead {
		return model.ArtifactInventory{}, archiveOverheadExceeded(measured.count - expanded)
	}
	if measured.count != size {
		return model.ArtifactInventory{}, fmt.Errorf("%w: original object size was not observed", model.ErrFinetuningFactsUnavailable)
	}
	if hex.EncodeToString(hash.Sum(nil)) != digest || len(result.ConfigJSON) == 0 || len(result.TokenizerConfigJSON) == 0 {
		return model.ArtifactInventory{}, model.ErrFinetuningBaseInvalid
	}
	files := make([]*modelv1.FinetuningArtifactFile, 0, len(result.Files))
	for _, f := range result.Files {
		files = append(files, &modelv1.FinetuningArtifactFile{Path: f.Path, SizeBytes: f.SizeBytes, Sha256: f.SHA256})
	}
	if _, err := modelv1.InspectFinetuningWeightLayout(files, result.WeightIndexJSON); err != nil {
		return model.ArtifactInventory{}, model.ErrFinetuningBaseInvalid
	}
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	return result, nil
}

type inventoryCountingReader struct {
	ctx       context.Context
	reader    io.Reader
	count     int64
	sourceErr error
}

func (r *inventoryCountingReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	r.count += int64(n)
	if err != nil && err != io.EOF {
		r.sourceErr = err
	}
	return n, err
}

func (r *inventoryCountingReader) archiveReadError(err error, size int64) error {
	if contextErr := r.ctx.Err(); contextErr != nil {
		return contextErr
	}
	// archive/tar can itself produce unexpected EOF for malformed complete
	// contents. A transport error or EOF before the frozen object size instead
	// means the original source has not been observed and can be retried.
	if r.sourceErr != nil {
		return fmt.Errorf("%w: source read: %w", model.ErrFinetuningFactsUnavailable, r.sourceErr)
	}
	if r.count < size && (errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)) {
		return fmt.Errorf("%w: source read: %w", model.ErrFinetuningFactsUnavailable, err)
	}
	return fmt.Errorf("%w: invalid archive: %w", model.ErrFinetuningBaseInvalid, err)
}

func archiveReadBoundExceeded(actual, limit int64) error {
	return &model.FinetuningLimitViolation{Reason: "STRUCTURE_LIMIT_EXCEEDED", LimitName: "tar_read_bound", Actual: actual, Limit: limit + finetuningTarOverhead}
}

// The archive envelope policy is raw bytes minus actual regular payload, not
// a second parser or a count of physical metadata records hidden by archive/tar.
func archiveOverheadExceeded(actual int64) error {
	return &model.FinetuningLimitViolation{Reason: "STRUCTURE_LIMIT_EXCEEDED", LimitName: "max_archive_overhead_bytes", Actual: actual, Limit: finetuningTarOverhead}
}

func finetuningProviderError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return providerError(err)
}
