package storage

import (
	"bytes"
	"context"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/s3utils"

	"oneweekonething.com/owot-pro/internal/infra/domain"
)

type s3Engine struct {
	client *minio.Client
	bucket string
}

func newS3(s domain.FileSettings) (*s3Engine, error) {
	endpoint, err := url.Parse(s.Endpoint)
	if err != nil || s3utils.CheckValidBucketNameStrict(s.Bucket) != nil {
		return nil, domain.ErrInvalidArgument
	}
	lookup := minio.BucketLookupDNS
	if s.PathStyle {
		lookup = minio.BucketLookupPath
	}
	client, err := minio.New(endpoint.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(s.AccessKey, s.AccessSecret, ""),
		Secure: endpoint.Scheme == "https", BucketLookup: lookup, Region: s.Region, MaxRetries: 2,
	})
	if err != nil {
		return nil, domain.ErrInvalidArgument
	}
	return &s3Engine{client: client, bucket: s.Bucket}, nil
}

func (e *s3Engine) Put(ctx context.Context, path string, data []byte, contentType string) error {
	if _, err := domain.SanitizeFilePath(path); err != nil {
		return err
	}
	// Use a fixed-length PUT with a signed Content-MD5 for S3-compatible
	// servers that do not decode AWS chunked payloads.
	options := minio.PutObjectOptions{
		ContentType: contentType, DisableMultipart: true, DisableContentSha256: true, SendContentMd5: true,
	}
	if !domain.InlineFileType(contentType) {
		options.ContentType = "application/octet-stream"
		options.ContentDisposition = "attachment"
	}
	_, err := e.client.PutObject(ctx, e.bucket, path, bytes.NewReader(data), int64(len(data)), options)
	return err
}

func (e *s3Engine) Get(ctx context.Context, path string) ([]byte, error) {
	if _, err := domain.SanitizeFilePath(path); err != nil {
		return nil, err
	}
	return e.read(ctx, path)
}

func (e *s3Engine) read(ctx context.Context, path string) ([]byte, error) {
	object, err := e.client.GetObject(ctx, e.bucket, path, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = object.Close() }()
	data, err := readFile(object)
	if response := minio.ToErrorResponse(err); response.Code == "NoSuchKey" {
		return nil, domain.ErrFileNotFound
	}
	return data, err
}

func (e *s3Engine) Delete(ctx context.Context, path string) error {
	if _, err := domain.SanitizeFilePath(path); err != nil {
		return err
	}
	return e.client.RemoveObject(ctx, e.bucket, path, minio.RemoveObjectOptions{})
}

func (e *s3Engine) PresignUpload(ctx context.Context, path string, ttl time.Duration) (string, error) {
	if _, err := domain.SanitizeFilePath(path); err != nil {
		return "", err
	}
	link, err := e.client.PresignedPutObject(ctx, e.bucket, domain.StagingDirectory+"/"+path, ttl)
	if err != nil {
		return "", err
	}
	return link.String(), nil
}

func (e *s3Engine) ReadUpload(ctx context.Context, path string) ([]byte, error) {
	if _, err := domain.SanitizeFilePath(path); err != nil {
		return nil, err
	}
	return e.read(ctx, domain.StagingDirectory+"/"+path)
}

func (e *s3Engine) DeleteUpload(ctx context.Context, path string) error {
	if _, err := domain.SanitizeFilePath(path); err != nil {
		return err
	}
	return e.client.RemoveObject(ctx, e.bucket, domain.StagingDirectory+"/"+path, minio.RemoveObjectOptions{})
}
