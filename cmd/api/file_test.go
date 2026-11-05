package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	infraapp "oneweekonething.com/owot-pro/internal/infra/app"
	infradomain "oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/outbox"
)

const (
	fileAdmin       = "/admin-api/infra/file/"
	fileConfigAdmin = "/admin-api/infra/file-config/"
)

func TestInfraFileHTTP(t *testing.T) {
	f := newIAMFixture(t)
	call := func(method, path string, body any, code int) json.RawMessage {
		return configCall(t, f, method, path, f.admin, 1, body, code)
	}
	read := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w
	}
	upload := func(name, directory, content, token string, code int) string {
		t.Helper()
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, err := form.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
		if err := form.WriteField("directory", directory); err != nil {
			t.Fatal(err)
		}
		if err := form.Close(); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", fileAdmin+"upload", &body)
		req.Header.Set("Content-Type", form.FormDataContentType())
		req.Header.Set("tenant-id", "1")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		var result iamEnvelope
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Code != code {
			t.Fatalf("upload %s returned %s, want code %d", name, w.Body, code)
		}
		if code != 0 {
			return ""
		}
		var link string
		if err := json.Unmarshal(result.Data, &link); err != nil {
			t.Fatal(err)
		}
		return link
	}

	upload("test.txt", "", "anonymous", "", 401)
	upload("../bad.txt", "", "unsafe", f.admin, 400)
	upload("bad.txt", "sub/../dir", "unsafe", f.admin, 400)
	const html = "<html><script>alert(1)</script></html>"
	dbURL := upload("前端 #说明.html", "folder/sub", html, f.admin, 0)
	if !strings.HasPrefix(dbURL, fileAdmin+"1/get/folder/sub/") {
		t.Fatalf("default DB URL was %s", dbURL)
	}
	w := read("GET", dbURL)
	if w.Body.String() != html || w.Header().Get("X-Content-Type-Options") != "nosniff" ||
		!strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") ||
		w.Header().Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("unsafe or incorrect file response: %v, %s", w.Header(), w.Body)
	}
	if head := read("HEAD", dbURL); head.Code != 200 || head.Body.Len() != 0 || head.Header().Get("Content-Length") != fmt.Sprint(len(html)) {
		t.Fatalf("unexpected HEAD response: %v, %s", head.Header(), head.Body)
	}
	call("GET", fileAdmin+"presigned-url?name=test.txt", nil, 400)
	call("DELETE", fileConfigAdmin+"delete?id=1", nil, 1_001_002_001)
	call("POST", fileConfigAdmin+"create", map[string]any{
		"name": "非法本地路径", "storage": 10, "config": map[string]any{"basePath": "../other"},
	}, 400)
	id := string(call("POST", fileConfigAdmin+"create", map[string]any{
		"name": "本地存储", "storage": 10, "master": true, "config": map[string]any{"basePath": "uploads"},
	}, 0))
	detail := call("GET", fileConfigAdmin+"get?id="+id, nil, 0)
	if !bytes.Contains(detail, []byte(`"master":false`)) {
		t.Fatalf("create switched the master: %s", detail)
	}
	call("PUT", fileConfigAdmin+"update-master?id="+id, nil, 0)
	if version, err := f.redis.Get(rootIAMContext(1), outbox.VersionKey(infraapp.FileConfigRefreshTopic, 0)).Result(); err != nil || version == "" {
		t.Fatalf("missing file config revision: %q, %v", version, err)
	}
	localURL := upload("new.txt", "", "new master", f.admin, 0)
	if !strings.HasPrefix(localURL, fileAdmin+id+"/get/") || read("GET", localURL).Body.String() != "new master" ||
		read("GET", dbURL).Body.String() != html {
		t.Fatalf("master switch lost existing file or chose wrong engine: %s", localURL)
	}

	var files struct {
		List []struct {
			ID              int64
			Name, URL, Type string
		}
		Total int
	}
	if err := json.Unmarshal(call("GET", fileAdmin+"page?type=text%2Fhtml", nil, 0), &files); err != nil ||
		files.Total != 1 || len(files.List) != 1 || files.List[0].URL != dbURL || files.List[0].Name != "前端 #说明.html" {
		t.Fatalf("unexpected file page: %+v, %v", files, err)
	}
	call("DELETE", fileConfigAdmin+"delete?id=1", nil, 409)
	call("DELETE", fileAdmin+fmt.Sprintf("delete-list?ids=%d,%d", files.List[0].ID, files.List[0].ID), nil, 0)
	configCall(t, f, "GET", dbURL, "", 0, nil, 1_001_003_000)
	call("DELETE", fileConfigAdmin+"delete?id=1", nil, 0)
	// Switching still works after the originally seeded config is deleted.
	call("PUT", fileConfigAdmin+"update-master?id="+id, nil, 0)

	data := f.ok("GET", "dict-data/type?type=infra_file_storage", nil, f.admin, 1)
	for _, code := range []int{infradomain.StorageDB, infradomain.StorageLocal, infradomain.StorageS3} {
		if !bytes.Contains(data, []byte(fmt.Sprintf(`"value":"%d"`, code))) {
			t.Fatalf("missing storage type %d in %s", code, data)
		}
	}
}

func TestInfraFileUnpagedLists(t *testing.T) {
	f := newIAMFixture(t)
	for i := range 101 {
		if err := f.db.Exec("INSERT INTO infra_file_config (name, storage, config) VALUES (?, ?, '{}')",
			"bulk file config", infradomain.StorageDB).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.db.Exec("INSERT INTO infra_file (config_id, owner_tenant_id, owner_user_id, name, path, url) VALUES (1, 1, 1, ?, ?, ?)",
			"bulk file", fmt.Sprintf("file-%d", i), "/files/sample").Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		fileAdmin + "page?name=bulk+file&pageSize=-1",
		fileConfigAdmin + "page?name=bulk+file+config&pageSize=-1",
	} {
		raw := configCall(t, f, "GET", path, f.admin, 1, nil, 0)
		var page struct {
			List  []json.RawMessage
			Total int
		}
		if err := json.Unmarshal(raw, &page); err != nil || len(page.List) != 101 || page.Total != 101 {
			t.Fatalf("%s returned %d rows, total %d, error %v", path, len(page.List), page.Total, err)
		}
	}
}
