// Package avatar validates images accepted by the IAM profile upload port.
package avatar

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"oneweekonething.com/owot-pro/internal/iam/domain"
)

func Validate(data []byte) (string, error) {
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
	return format, nil
}
