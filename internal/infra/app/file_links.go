package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"oneweekonething.com/owot-pro/internal/infra/domain"
)

const fileDownloadTTL = 24 * time.Hour

func (s *FileService) fileURL(c domain.FileConfig, path string) string {
	base := s.publicBase
	if c.Storage != domain.StorageS3 && c.Settings.Domain != "" {
		base = c.Settings.Domain
	}
	escaped := (&url.URL{Path: path}).EscapedPath()
	return fmt.Sprintf("%s/admin-api/infra/file/%d/get/%s", base, c.ID, escaped)
}

func fileDownloadURL(c domain.FileConfig, f domain.FileObject, allowDirect bool) string {
	if f.CleanupPending {
		return ""
	}
	if c.Storage != domain.StorageS3 || f.Avatar {
		return f.URL
	}
	if !c.Settings.PublicAccess {
		expiration := strconv.FormatInt(time.Now().Add(fileDownloadTTL).Unix(), 10)
		signature := fileDownloadSignature(c, f.Path, expiration)
		return f.URL + "?download=" + expiration + "." + base64.RawURLEncoding.EncodeToString(signature)
	}
	if allowDirect && c.Settings.Domain != "" {
		return c.Settings.Domain + "/" + (&url.URL{Path: f.Path}).EscapedPath()
	}
	return f.URL
}

func canDownload(c domain.FileConfig, f domain.FileObject, grant string) bool {
	if c.Storage != domain.StorageS3 || c.Settings.PublicAccess || f.Avatar {
		return true
	}
	expiration, encoded, ok := strings.Cut(grant, ".")
	if !ok || len(encoded) != base64.RawURLEncoding.EncodedLen(sha256.Size) {
		return false
	}
	expiresAt, err := strconv.ParseInt(expiration, 10, 64)
	if err != nil || expiresAt <= time.Now().Unix() {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(encoded)
	return err == nil && hmac.Equal(signature, fileDownloadSignature(c, f.Path, expiration))
}

func fileDownloadSignature(c domain.FileConfig, path, expiration string) []byte {
	mac := hmac.New(sha256.New, []byte(c.Settings.AccessSecret))
	_, _ = fmt.Fprintf(mac, "owot-pro/file-download\n%d\n%s\n%s", c.ID, expiration, path)
	return mac.Sum(nil)
}
