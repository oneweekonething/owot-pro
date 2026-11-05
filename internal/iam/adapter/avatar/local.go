// Package avatar supplies the profile upload port until Infra file storage is connected.
package avatar

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"

	"oneweekonething.com/owot-pro/internal/iam/domain"
)

type Local struct{ Directory string }

func (l *Local) Upload(ctx context.Context, _ string, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(data) == 0 || len(data) > 2<<20 {
		return "", domain.ErrInvalidArgument
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 10000 || config.Height > 10000 {
		return "", domain.ErrInvalidArgument
	}
	if format != "png" && format != "jpeg" && format != "gif" {
		return "", domain.ErrInvalidArgument
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	name := hex.EncodeToString(id[:]) + "." + format
	if err := os.MkdirAll(l.Directory, 0750); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(l.Directory, name), data, 0640); err != nil {
		return "", err
	}
	return "/admin-api/system/user/profile/avatar/" + name, nil
}
