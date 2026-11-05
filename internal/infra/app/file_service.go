package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

const (
	fileOperationTimeout = 15 * time.Second
	uploadTTL            = 10 * time.Minute
)

type PresignedUpload struct {
	ConfigID  int64  `json:"configId"`
	Path      string `json:"path"`
	URL       string `json:"url"`
	UploadURL string `json:"uploadUrl"`
}

func (s *FileService) Upload(ctx context.Context, name, directory string, data []byte) (string, error) {
	return s.upload(ctx, 0, name, directory, data, false)
}

func (s *FileService) UploadAvatar(ctx context.Context, name string, data []byte) (string, error) {
	return s.upload(ctx, 0, name, "avatar", data, true)
}

func (s *FileService) TestConfig(ctx context.Context, id int64) (string, error) {
	if id <= 0 {
		return "", domain.ErrInvalidArgument
	}
	return s.upload(ctx, id, "storage-test.txt", "", []byte("owot-pro file storage test\n"), false)
}

func (s *FileService) upload(ctx context.Context, configID int64, name, directory string, data []byte, avatar bool) (string, error) {
	typ, err := detectFileType(data)
	if err != nil {
		return "", err
	}
	file, err := newFile(ctx, name, directory)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, fileOperationTimeout)
	defer cancel()
	var engine FileStorageEngine
	var result string
	var attempted bool
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var c domain.FileConfig
		var err error
		if configID == 0 {
			c, err = s.repo.Configs.Master(ctx)
		} else {
			c, err = s.repo.Configs.Config(ctx, configID)
		}
		if err != nil {
			return err
		}
		engine, err = s.storage.Engine(ctx, c)
		if err != nil {
			return err
		}
		file.ConfigID, file.URL, file.Type, file.Size = c.ID, s.fileURL(c, file.Path), typ, int64(len(data))
		file.Avatar = avatar
		result = fileDownloadURL(c, file, true)
		if err := s.repo.FileWrites.CreateFile(ctx, &file); err != nil {
			return err
		}
		attempted = c.Storage != domain.StorageDB
		return engine.Put(ctx, file.Path, data, typ)
	})
	if err != nil && attempted {
		err = errors.Join(err, s.discardUncommittedUpload(ctx, engine.Delete, file))
	}
	if err != nil {
		return "", fmt.Errorf("store uploaded file: %w", err)
	}
	return result, nil
}

func (s *FileService) Presign(ctx context.Context, name, directory string) (PresignedUpload, error) {
	file, err := newFile(ctx, name, directory)
	if err != nil {
		return PresignedUpload{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, fileOperationTimeout)
	defer cancel()
	var result PresignedUpload
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		c, err := s.repo.Configs.Master(ctx)
		if err != nil {
			return err
		}
		_, direct, err := s.directStorage(ctx, c)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		link, err := direct.PresignUpload(ctx, file.Path, uploadTTL)
		if err != nil {
			return err
		}
		expires := now.Add(uploadTTL)
		file.ConfigID, file.URL, file.UploadExpiresAt = c.ID, s.fileURL(c, file.Path), &expires
		if err := s.repo.FileWrites.CreateFile(ctx, &file); err != nil {
			return err
		}
		result = PresignedUpload{ConfigID: c.ID, Path: file.Path, URL: fileDownloadURL(c, file, false), UploadURL: link}
		return nil
	})
	if err != nil {
		return PresignedUpload{}, fmt.Errorf("reserve direct upload: %w", err)
	}
	// Prune outside the upload transaction so concurrent reservations do not
	// take gap locks on the same pending-upload index.
	_ = s.repo.FileWrites.PrunePending(ctx, result.ConfigID, time.Now().UTC())
	return result, nil
}

func (s *FileService) CompleteUpload(ctx context.Context, configID int64, path string) (int64, error) {
	path, err := domain.SanitizeFilePath(path)
	if err != nil || configID <= 0 {
		return 0, domain.ErrInvalidArgument
	}
	user, ok := ctxmeta.User(ctx)
	if !ok {
		return 0, domain.ErrInvalidArgument
	}
	ctx, cancel := context.WithTimeout(ctx, fileOperationTimeout)
	defer cancel()
	var file domain.FileObject
	var engine FileStorageEngine
	var direct DirectFileStorage
	var attempted bool
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		c, err := s.repo.Configs.Config(ctx, configID)
		if err != nil {
			return err
		}
		file, err = s.repo.Files.FindFile(ctx, configID, path)
		if err != nil {
			return err
		}
		if file.OwnerUserID != user.ID || file.OwnerTenantID != ctxmeta.TenantID(ctx) {
			return domain.ErrFileNotFound
		}
		if file.UploadExpiresAt == nil {
			return nil
		}
		if !time.Now().Before(*file.UploadExpiresAt) {
			return domain.ErrFileNotFound
		}
		engine, direct, err = s.directStorage(ctx, c)
		if err != nil {
			return err
		}
		data, err := direct.ReadUpload(ctx, path)
		if err != nil {
			return err
		}
		file.Type, err = detectFileType(data)
		if err != nil {
			return err
		}
		file.Size = int64(len(data))
		attempted = true
		if err := engine.Put(ctx, path, data, file.Type); err != nil {
			return err
		}
		return s.repo.FileWrites.CompleteFile(ctx, file)
	})
	if err != nil {
		if attempted {
			err = errors.Join(err, s.discardUncommittedUpload(ctx, engine.Delete, file))
		}
		return 0, fmt.Errorf("complete direct upload: %w", err)
	}
	if direct != nil {
		// Lifecycle expiration also covers abandoned uploads and replayed PUTs.
		cleanup, cancel := context.WithTimeout(ctxmeta.CloneContext(ctx), 2*time.Second)
		defer cancel()
		_ = direct.DeleteUpload(cleanup, path)
	}
	return file.ID, nil
}

func (s *FileService) Read(ctx context.Context, configID int64, path, grant string) (domain.FileObject, []byte, error) {
	path, err := domain.SanitizeFilePath(path)
	if err != nil || configID <= 0 {
		return domain.FileObject{}, nil, domain.ErrFileNotFound
	}
	c, err := s.repo.Configs.Config(ctx, configID)
	if err != nil {
		return domain.FileObject{}, nil, fmt.Errorf("read storage config: %w", err)
	}
	file, err := s.repo.Files.FindFile(ctx, configID, path)
	if err != nil {
		return file, nil, fmt.Errorf("find file: %w", err)
	}
	if !canDownload(c, file, grant) {
		return file, nil, domain.ErrFileNotFound
	}
	if file.UploadExpiresAt != nil {
		file, err = s.waitForRegistration(ctx, file)
		if err != nil {
			return file, nil, err
		}
		c, err = s.repo.Configs.Config(ctx, configID)
		if err != nil {
			return file, nil, fmt.Errorf("refresh storage config after registration: %w", err)
		}
		if !canDownload(c, file, grant) {
			return file, nil, domain.ErrFileNotFound
		}
	}
	ctx, cancel := context.WithTimeout(ctx, fileOperationTimeout)
	defer cancel()
	engine, err := s.storage.Engine(ctx, c)
	if err != nil {
		return file, nil, fmt.Errorf("resolve storage engine: %w", err)
	}
	data, err := engine.Get(ctx, path)
	if err != nil {
		return file, nil, fmt.Errorf("read file content: %w", err)
	}
	return file, data, nil
}

// The frontend starts registration asynchronously after PUT and may request
// the preview immediately. Keep the temporary object private while it waits.
func (s *FileService) waitForRegistration(ctx context.Context, file domain.FileObject) (domain.FileObject, error) {
	deadline := time.Now().Add(fileOperationTimeout)
	if file.UploadExpiresAt.Before(deadline) {
		deadline = *file.UploadExpiresAt
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for file.UploadExpiresAt != nil {
		select {
		case <-ctx.Done():
			return file, domain.ErrFileNotFound
		case <-ticker.C:
			current, err := s.repo.Files.FindFile(ctx, file.ConfigID, file.Path)
			if err != nil {
				return file, fmt.Errorf("wait for file registration: %w", err)
			}
			file = current
		}
	}
	return file, nil
}

func (s *FileService) Page(ctx context.Context, filter FileFilter) (pagination.PageResult[domain.FileObject], error) {
	if err := normalizeFileFilter(&filter); err != nil {
		return pagination.PageResult[domain.FileObject]{}, err
	}
	page, err := s.repo.Files.PageFiles(ctx, filter)
	if err != nil {
		return page, fmt.Errorf("page files: %w", err)
	}
	configs := make(map[int64]domain.FileConfig)
	for i := range page.List {
		file := &page.List[i]
		c, ok := configs[file.ConfigID]
		if !ok {
			c, err = s.repo.Configs.Config(ctx, file.ConfigID)
			if err != nil && !errors.Is(err, domain.ErrFileConfigNotFound) {
				return page, fmt.Errorf("resolve file config for list: %w", err)
			}
			configs[file.ConfigID] = c
		}
		if c.ID == 0 {
			file.URL = ""
			continue
		}
		file.URL = fileDownloadURL(c, *file, true)
	}
	return page, nil
}

func (s *FileService) Delete(ctx context.Context, ids []int64) error {
	ids, err := fileIDs(ids)
	if err != nil {
		return err
	}
	files, err := s.repo.Files.FilesByID(ctx, ids)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := s.deleteFile(ctx, file); err != nil {
			return err
		}
	}
	return nil
}

func (s *FileService) deleteFile(ctx context.Context, file domain.FileObject) error {
	ctx, cancel := context.WithTimeout(ctx, fileOperationTimeout)
	defer cancel()
	// Commit the tombstone before removing bytes. Failed storage operations
	// leave the file hidden and the same ID can be retried.
	if err := s.repo.FileWrites.MarkFileForDeletion(ctx, file.ID); err != nil {
		return err
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		c, err := s.repo.Configs.Config(ctx, file.ConfigID)
		if errors.Is(err, domain.ErrFileConfigNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		files, err := s.repo.Files.FilesByID(ctx, []int64{file.ID})
		if err != nil || len(files) == 0 {
			return err
		}
		engine, err := s.storage.Engine(ctx, c)
		if err != nil {
			return err
		}
		if err := engine.Delete(ctx, file.Path); err != nil {
			return fmt.Errorf("delete object for file %d: %w", file.ID, err)
		}
		return s.repo.FileWrites.FinishFileDeletion(ctx, file.ID)
	})
}

func (s *FileService) directStorage(ctx context.Context, c domain.FileConfig) (FileStorageEngine, DirectFileStorage, error) {
	engine, err := s.storage.Engine(ctx, c)
	if err != nil {
		return nil, nil, err
	}
	direct, ok := engine.(DirectFileStorage)
	if !ok {
		return nil, nil, domain.ErrFileDirectUpload
	}
	return engine, direct, nil
}

func (s *FileService) discardUncommittedUpload(ctx context.Context, remove func(context.Context, string) error, f domain.FileObject) error {
	ctx, cancel := context.WithTimeout(ctxmeta.CloneContext(ctx), 2*time.Second)
	defer cancel()
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// A commit error may be ambiguous. Coordinate with registration retries
		// before removing bytes from storage.
		if _, err := s.repo.Configs.Config(ctx, f.ConfigID); err != nil && !errors.Is(err, domain.ErrFileConfigNotFound) {
			return err
		}
		current, err := s.repo.Files.FindFile(ctx, f.ConfigID, f.Path)
		if err == nil && current.UploadExpiresAt == nil {
			return nil
		}
		if err != nil && !errors.Is(err, domain.ErrFileNotFound) {
			return err
		}
		return remove(ctx, f.Path)
	})
}

func newFile(ctx context.Context, name, directory string) (domain.FileObject, error) {
	user, ok := ctxmeta.User(ctx)
	tenant := ctxmeta.TenantID(ctx)
	if !ok || user.ID <= 0 || tenant <= 0 {
		return domain.FileObject{}, domain.ErrInvalidArgument
	}
	name = strings.TrimSpace(name)
	path, err := domain.NewFilePath(time.Now(), directory, name)
	return domain.FileObject{Name: name, Path: path, OwnerUserID: user.ID, OwnerTenantID: tenant}, err
}

func detectFileType(data []byte) (string, error) {
	if len(data) == 0 {
		return "", domain.ErrInvalidArgument
	}
	if len(data) > domain.MaxFileSize {
		return "", domain.ErrFileTooLarge
	}
	return http.DetectContentType(data), nil
}
