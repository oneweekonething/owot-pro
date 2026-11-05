package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	StorageDB    = 1
	StorageLocal = 10
	StorageS3    = 20

	MaxFileSize = 20 << 20
)

type FileConfig struct {
	ID           int64
	Name, Remark string
	Storage      int
	Master       bool
	Settings     FileSettings
	CreateTime   time.Time
}

type FileSettings struct {
	BasePath     string `json:"basePath"`
	Endpoint     string `json:"endpoint,omitempty"`
	Bucket       string `json:"bucket,omitempty"`
	AccessKey    string `json:"accessKey,omitempty"`
	AccessSecret string `json:"accessSecret,omitempty"`
	Region       string `json:"region,omitempty"`
	Domain       string `json:"domain"`
	PathStyle    bool   `json:"enablePathStyleAccess"`
	PublicAccess bool   `json:"enablePublicAccess"`
}

func (c *FileConfig) Validate() error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || utf8.RuneCountInString(c.Name) > 100 || utf8.RuneCountInString(c.Remark) > 500 {
		return ErrInvalidArgument
	}
	s := &c.Settings
	var err error
	s.Domain, err = NormalizeFileDomain(s.Domain)
	if err != nil {
		return err
	}
	switch c.Storage {
	case StorageDB:
		*s = FileSettings{Domain: s.Domain}
	case StorageLocal:
		if len(s.BasePath) > 512 {
			return ErrInvalidArgument
		}
		*s = FileSettings{Domain: s.Domain, BasePath: s.BasePath}
	case StorageS3:
		s.BasePath = ""
		s.Endpoint = strings.TrimRight(strings.TrimSpace(s.Endpoint), "/")
		if !strings.Contains(s.Endpoint, "://") {
			s.Endpoint = "https://" + s.Endpoint
		}
		if !validHTTPURL(s.Endpoint, false) || s.Bucket == "" || s.AccessKey == "" || s.AccessSecret == "" ||
			len(s.Bucket) > 63 || len(s.AccessKey) > 256 || len(s.AccessSecret) > 512 || len(s.Region) > 64 {
			return ErrInvalidArgument
		}
	default:
		return ErrInvalidArgument
	}
	return nil
}

// NormalizeFileDomain validates the external base URL for file links.
func NormalizeFileDomain(raw string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base != "" && !validHTTPURL(base, true) {
		return "", ErrInvalidArgument
	}
	return base, nil
}

func validHTTPURL(raw string, allowPath bool) bool {
	u, err := url.Parse(raw)
	return err == nil && len(raw) <= 255 && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" &&
		u.User == nil && u.RawQuery == "" && !u.ForceQuery && !strings.Contains(raw, "#") &&
		!strings.ContainsAny(raw, "\\") && (allowPath || u.Path == "")
}

func (s FileSettings) SameLocation(other FileSettings) bool {
	return s.BasePath == other.BasePath && s.Endpoint == other.Endpoint &&
		s.Bucket == other.Bucket && s.PathStyle == other.PathStyle
}

type FileObject struct {
	ID, ConfigID, OwnerTenantID, OwnerUserID int64
	Name, Path, URL, Type                    string
	Size                                     int64
	CreateTime                               time.Time
	UploadExpiresAt                          *time.Time
	Avatar, CleanupPending                   bool
}

var (
	directoryPart = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)
	datePart      = regexp.MustCompile(`^[0-9]{8}$`)
	hashPart      = regexp.MustCompile(`^[0-9a-f]{32}_`)
)

func validFileName(name string) bool {
	return name != "" && len(name) <= 180 && utf8.ValidString(name) && name == strings.TrimSpace(name) &&
		name != "." && !strings.Contains(name, "..") && !strings.ContainsAny(name, "/\\:%") &&
		strings.IndexFunc(name, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) == -1
}

func NewFilePath(now time.Time, directory, name string) (string, error) {
	name = strings.TrimSpace(name)
	if !validFileName(name) {
		return "", ErrInvalidArgument
	}
	var nonce [32]byte
	_, _ = rand.Read(nonce[:])
	digest := sha256.Sum256(nonce[:])
	path := now.UTC().Format("20060102") + "/" + hex.EncodeToString(digest[:16]) + "_" + name
	if directory != "" {
		path = directory + "/" + path
	}
	return SanitizeFilePath(path)
}

// SanitizeFilePath accepts only paths produced by NewFilePath.
func SanitizeFilePath(path string) (string, error) {
	parts := strings.Split(path, "/")
	if len(parts) < 2 || len(parts) > 5 || !datePart.MatchString(parts[len(parts)-2]) {
		return "", ErrInvalidArgument
	}
	for _, directory := range parts[:len(parts)-2] {
		if !directoryPart.MatchString(directory) || directory == StagingDirectory {
			return "", ErrInvalidArgument
		}
	}
	file := parts[len(parts)-1]
	if !hashPart.MatchString(file) || !validFileName(file[33:]) {
		return "", ErrInvalidArgument
	}
	return path, nil
}

const StagingDirectory = "_upload"

// InlineFileType limits browser previews to media formats without scripting.
func InlineFileType(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp", "video/mp4", "video/webm", "audio/mpeg", "audio/wave", "audio/ogg":
		return true
	default:
		return false
	}
}
