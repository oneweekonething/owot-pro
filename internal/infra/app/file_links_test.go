package app

import (
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"oneweekonething.com/owot-pro/internal/infra/domain"
)

func TestPrivateS3DownloadGrant(t *testing.T) {
	c := domain.FileConfig{ID: 1, Storage: domain.StorageS3, Settings: domain.FileSettings{AccessSecret: "file-link-test-key"}}
	s := FileService{publicBase: "https://api.example.com"}
	f := domain.FileObject{ConfigID: c.ID, Path: "20261105/abc_sample.txt"}
	f.URL = s.fileURL(c, f.Path)
	link, err := url.Parse(fileDownloadURL(c, f, false))
	if err != nil || link.Host != "api.example.com" || !strings.HasSuffix(link.Path, f.Path) {
		t.Fatalf("unexpected download URL %v, %v", link, err)
	}
	grant := link.Query().Get("download")
	if !canDownload(c, f, grant) {
		t.Fatal("fresh link was rejected")
	}
	expiration := strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)
	expired := expiration + "." + base64.RawURLEncoding.EncodeToString(fileDownloadSignature(c, f.Path, expiration))
	if canDownload(c, f, expired) {
		t.Fatal("expired link was accepted")
	}
	other := f
	other.Path += ".txt"
	if canDownload(c, other, grant) {
		t.Fatal("link granted access to another file")
	}
	c.Settings.AccessSecret = "rotated-test-key"
	if canDownload(c, f, grant) {
		t.Fatal("link remained valid after the credentials were rotated")
	}
	f.Avatar = true
	if !canDownload(c, f, "") {
		t.Fatal("avatar URL requires a download grant")
	}
}
