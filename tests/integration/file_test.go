package integration

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	infracache "oneweekonething.com/owot-pro/internal/infra/adapter/cache"
	"oneweekonething.com/owot-pro/internal/infra/adapter/persistence"
	"oneweekonething.com/owot-pro/internal/infra/adapter/storage"
	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/outbox"
)

func realFiles(t *testing.T) (*gorm.DB, *redis.Client, *app.FileService) {
	t.Helper()
	db, client := realSchema(t, "files", "001_system.sql", "004_infra_file.sql", "004_infra_file.sql")
	if err := persistence.CheckFileSchema(db); err != nil {
		t.Fatal(err)
	}
	store := outbox.NewStore(db, infracache.NewConfigBroadcaster(client, outbox.NewRedisBroadcaster(client)),
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	repo := persistence.NewFileRepository(db)
	repos := app.FileRepositories{Configs: repo, ConfigWrites: repo, Files: repo, FileWrites: repo}
	service := app.NewFileService(repos, dbx.NewTxManager(db), store, storage.NewFactory(db, client, t.TempDir()), "")
	return db, client, service
}

func TestInfraFileMasterMySQLAndRedis(t *testing.T) {
	db, client, service := realFiles(t)
	ctx, cancel := context.WithTimeout(iamRoot(1), 25*time.Second)
	defer cancel()
	create := func(name string, engine int) int64 {
		t.Helper()
		id, err := service.CreateConfig(ctx, domain.FileConfig{Name: name, Storage: engine})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	local := create("local", domain.StorageLocal)
	alternate := create("alternate database", domain.StorageDB)
	repo := persistence.NewFileRepository(db)
	versionKey := outbox.VersionKey(app.FileConfigRefreshTopic, 0)
	rejected := errors.New("test: file outbox append rejected")
	failed := app.NewFileService(app.FileRepositories{ConfigWrites: repo}, dbx.NewTxManager(db), failedConfigOutbox{err: rejected}, nil, "")
	if err := failed.UpdateMaster(ctx, local); !errors.Is(err, rejected) {
		t.Fatalf("got %v for rejected outbox append", err)
	}
	if c, err := repo.Master(ctx); err != nil || c.ID != 1 {
		t.Fatalf("outbox rollback changed master: %+v %v", c, err)
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range 12 {
		wg.Go(func() {
			<-start
			if i%3 == 0 {
				id := local
				if i%2 == 0 {
					id = alternate
				}
				if err := service.UpdateMaster(ctx, id); err != nil {
					t.Errorf("concurrent master switch: %v", err)
				}
				return
			}
			body := fmt.Sprintf("concurrent upload %d", i)
			link, err := service.Upload(ctx, "note.txt", "", []byte(body))
			if err != nil {
				t.Errorf("concurrent upload: %v", err)
				return
			}
			for _, id := range []int64{1, local, alternate} {
				if path, ok := strings.CutPrefix(link, fmt.Sprintf("/admin-api/infra/file/%d/get/", id)); ok {
					_, data, err := service.Read(ctx, id, path, "")
					if err != nil || string(data) != body {
						t.Errorf("concurrent upload read: %q %v", data, err)
					}
					return
				}
			}
			t.Errorf("unexpected file URL %q", link)
		})
	}
	close(start)
	wg.Wait()
	var masters int
	if err := db.Raw("SELECT COUNT(*) FROM infra_file_config WHERE master_slot = 1").Scan(&masters).Error; err != nil || masters != 1 {
		t.Fatalf("expected one master, got %d: %v", masters, err)
	}
	var switches int
	if err := db.Raw("SELECT COUNT(*) FROM sys_outbox_event WHERE topic = ?", app.FileConfigRefreshTopic).Scan(&switches).Error; err != nil || switches != 4 {
		t.Fatalf("expected four committed switches, got %d: %v", switches, err)
	}
	if version, err := client.Get(ctx, versionKey).Result(); err != nil || version == "" {
		t.Fatalf("master switch did not publish a cache version: %q %v", version, err)
	}
	if err := service.UpdateMaster(ctx, alternate); err != nil {
		t.Fatal(err)
	}
	tenant := iamRoot(2)
	link, err := service.Upload(tenant, "tenant2.txt", "", []byte("tenant 2"))
	if err != nil {
		t.Fatal(err)
	}
	path := strings.TrimPrefix(link, fmt.Sprintf("/admin-api/infra/file/%d/get/", alternate))
	file, _, err := service.Read(ctx, alternate, path, "")
	if err != nil {
		t.Fatal(err)
	}
	if page, err := service.Page(ctx, app.FileFilter{Name: "tenant2.txt"}); err != nil || page.Total != 0 {
		t.Fatalf("cross-tenant file listing: %+v %v", page, err)
	}
	if err := service.Delete(ctx, []int64{file.ID}); err != nil {
		t.Fatal(err)
	}
	if _, data, err := service.Read(ctx, alternate, path, ""); err != nil || string(data) != "tenant 2" {
		t.Fatalf("cross-tenant deletion removed file: %q %v", data, err)
	}
	if err := service.Delete(tenant, []int64{file.ID}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Read(ctx, alternate, path, ""); !errors.Is(err, domain.ErrFileNotFound) {
		t.Fatalf("owner deletion left file: %v", err)
	}
}

func TestInfraFileS3DirectUpload(t *testing.T) {
	endpoint := os.Getenv("OWOT_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("OWOT_TEST_S3_ENDPOINT enables the S3-compatible storage test")
	}
	access, secret := os.Getenv("OWOT_TEST_S3_ACCESS_KEY"), os.Getenv("OWOT_TEST_S3_SECRET_KEY")
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || access == "" || secret == "" {
		t.Fatal("set S3 endpoint URL, access key and secret key")
	}
	_, _, service := realFiles(t)
	ctx, cancel := context.WithTimeout(iamRoot(1), 30*time.Second)
	defer cancel()
	s3, err := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(access, secret, ""), Secure: u.Scheme == "https",
		BucketLookup: minio.BucketLookupPath, Region: "us-east-1", MaxRetries: 1})
	if err != nil {
		t.Fatal(err)
	}
	bucket := "owot-" + strings.ToLower(rand.Text())
	if err := s3.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for object := range s3.ListObjects(cleanup, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if object.Err == nil {
				_ = s3.RemoveObject(cleanup, bucket, object.Key, minio.RemoveObjectOptions{})
			}
		}
		if err := s3.RemoveBucket(cleanup, bucket); err != nil {
			t.Errorf("remove test bucket: %v", err)
		}
	})
	id, err := service.CreateConfig(ctx, domain.FileConfig{Name: "S3", Storage: domain.StorageS3,
		Settings: domain.FileSettings{Endpoint: endpoint, Bucket: bucket, AccessKey: access, AccessSecret: secret,
			Region: "us-east-1", PathStyle: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateMaster(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := service.TestConfig(ctx, id); err != nil {
		t.Fatal(err)
	}
	reservation, err := service.Presign(ctx, "直传 #文件.txt", "docs")
	if err != nil || reservation.ConfigID != id {
		t.Fatalf("presign config %d: %v", reservation.ConfigID, err)
	}
	put := func(body string) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, reservation.UploadURL, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "image/png")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal("presigned PUT failed")
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
			t.Fatalf("presigned PUT returned %d: %s", response.StatusCode, body)
		}
	}
	if _, err := service.CompleteUpload(ctx, id, reservation.Path); !errors.Is(err, domain.ErrFileNotFound) {
		t.Fatalf("register before PUT: %v", err)
	}
	const original = "verified upload content"
	put(original)
	download, err := url.Parse(reservation.URL)
	if err != nil {
		t.Fatal(err)
	}
	grant := download.Query().Get("download")
	if grant == "" {
		t.Fatal("private file URL is missing a download grant")
	}
	if _, _, err := service.Read(ctx, id, reservation.Path, ""); !errors.Is(err, domain.ErrFileNotFound) {
		t.Fatalf("pending file was readable without a grant: %v", err)
	}
	other := ctxmeta.WithLoginUser(ctx, ctxmeta.LoginUser{ID: 2, TenantID: 1, UserType: 1})
	if _, err := service.CompleteUpload(other, id, reservation.Path); !errors.Is(err, domain.ErrFileNotFound) {
		t.Fatalf("another user registered the upload: %v", err)
	}
	if err := service.UpdateMaster(ctx, 1); err != nil {
		t.Fatal(err)
	}
	// The stock client starts reading before its asynchronous registration finishes.
	type previewResult struct {
		body string
		err  error
	}
	preview := make(chan previewResult, 1)
	go func() {
		_, data, err := service.Read(ctx, id, reservation.Path, grant)
		preview <- previewResult{body: string(data), err: err}
	}()
	select {
	case early := <-preview:
		t.Fatalf("preview returned before registration: %+v", early)
	case <-time.After(100 * time.Millisecond):
	}
	fileID, err := service.CompleteUpload(ctx, id, reservation.Path)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-preview:
		if got.err != nil || got.body != original {
			t.Fatalf("preview did not wait for registration: %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("preview never returned")
	}
	for _, invalidGrant := range []string{"", grant + "x"} {
		if _, _, err := service.Read(ctx, id, reservation.Path, invalidGrant); !errors.Is(err, domain.ErrFileNotFound) {
			t.Fatalf("private file was readable with an invalid grant: %v", err)
		}
	}
	put("replacement with previously signed URL")
	again, err := service.CompleteUpload(ctx, id, reservation.Path)
	if err != nil || again != fileID {
		t.Fatalf("registration retry was not idempotent: %d %v", again, err)
	}
	file, data, err := service.Read(ctx, id, reservation.Path, grant)
	if err != nil || string(data) != original || file.Type != "text/plain; charset=utf-8" || file.Size != int64(len(original)) {
		t.Fatalf("registered file did not match verified content: %+v %q %v", file, data, err)
	}
	config, err := service.GetConfig(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	invalid := config
	invalid.Settings.AccessKey = "invalid-access-key"
	if err := service.UpdateConfig(ctx, invalid); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(ctx, []int64{fileID}); err == nil {
		t.Fatal("S3 deletion unexpectedly succeeded with invalid credentials")
	}
	if _, _, err := service.Read(ctx, id, reservation.Path, grant); !errors.Is(err, domain.ErrFileNotFound) {
		t.Fatalf("failed storage deletion exposed file metadata: %v", err)
	}
	filter := app.FileFilter{Name: "直传 #文件.txt"}
	if page, err := service.Page(ctx, filter); err != nil || page.Total != 1 || len(page.List) != 1 ||
		page.List[0].ID != fileID || !page.List[0].CleanupPending || page.List[0].URL != "" {
		t.Fatalf("failed deletion is not retryable from the file list: %+v %v", page, err)
	}
	if err := service.UpdateConfig(ctx, config); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(ctx, []int64{fileID}); err != nil {
		t.Fatal(err)
	}
	if page, err := service.Page(ctx, filter); err != nil || page.Total != 0 {
		t.Fatalf("successfully deleted file remains listed: %+v %v", page, err)
	}
	if _, err := s3.StatObject(ctx, bucket, reservation.Path, minio.StatObjectOptions{}); minio.ToErrorResponse(err).Code != "NoSuchKey" {
		t.Fatalf("deleted S3 object still exists: %v", err)
	}
}
