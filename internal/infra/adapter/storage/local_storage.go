package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"oneweekonething.com/owot-pro/internal/infra/domain"
)

type localEngine struct{ root, directory string }

func (e *localEngine) Put(ctx context.Context, path string, data []byte, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := domain.SanitizeFilePath(path); err != nil {
		return err
	}
	if err := os.MkdirAll(e.root, 0750); err != nil {
		return err
	}
	root, err := os.OpenRoot(e.root)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	path = filepath.Join(e.directory, filepath.FromSlash(path))
	if err := makeLocalDirectory(root, filepath.Dir(path)); err != nil {
		return err
	}
	f, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0640)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	if err := errors.Join(writeErr, f.Close()); err != nil {
		_ = root.Remove(path)
		return err
	}
	return nil
}

// Go 1.25's Root.MkdirAll can fail with EEXIST when another writer
// creates an intermediate directory. Accept an existing directory at each step.
func makeLocalDirectory(root *os.Root, path string) error {
	if path == "." {
		return nil
	}
	err := root.Mkdir(path, 0750)
	if errors.Is(err, os.ErrNotExist) {
		if err := makeLocalDirectory(root, filepath.Dir(path)); err != nil {
			return err
		}
		err = root.Mkdir(path, 0750)
	}
	if errors.Is(err, os.ErrExist) {
		if info, statErr := root.Stat(path); statErr == nil && info.IsDir() {
			return nil
		}
	}
	return err
}

func (e *localEngine) Get(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := domain.SanitizeFilePath(path); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(e.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, domain.ErrFileNotFound
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(filepath.Join(e.directory, filepath.FromSlash(path)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, domain.ErrFileNotFound
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return readFile(f)
}

func (e *localEngine) Delete(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := domain.SanitizeFilePath(path); err != nil {
		return err
	}
	root, err := os.OpenRoot(e.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	err = root.Remove(filepath.Join(e.directory, filepath.FromSlash(path)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func readFile(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, domain.MaxFileSize+1))
	if len(data) > domain.MaxFileSize {
		return nil, domain.ErrFileTooLarge
	}
	return data, err
}
