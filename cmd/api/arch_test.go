package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const modulePrefix = "oneweekonething.com/owot-pro"

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/actuator/health", nil)
	rec := httptest.NewRecorder()
	newHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"UP"`) {
		t.Fatalf("unexpected health body: %s", rec.Body.String())
	}
}

// TestArchitectureDependencyBoundaries 自动强制执行《规范 1.2》与根 AGENTS.md 的依赖方向铁律：
//  1. internal/*/domain 100% 仅允许导入 Go 标准库（不得导入任何第三方包或任何 internal/* 包）。
//  2. internal/*/app 仅允许导入标准库、同领域 domain，以及纯标准库契约包 internal/platform/{ctxmeta,pagination}。
//  3. internal/platform/{ctxmeta,pagination} 100% 仅允许导入 Go 标准库。
//  4. internal/platform/* 严禁反向导入 internal/{iam,system,infra} 业务领域。
//  5. internal/<domain>/adapter/<subA> 严禁导入同一领域的其他 Adapter 子包 internal/<domain>/adapter/<subB>（如 adapter/http -> adapter/persistence），也严禁跨领域导入其他领域的 adapter。
//  6. 仓库中严禁出现 pkg/、common/、utils/、shared/ 目录。
func TestArchitectureDependencyBoundaries(t *testing.T) {
	root := filepath.Clean("../..")
	if err := validateDirectoryTree(root); err != nil {
		t.Fatal(err)
	}
}

// TestArchitectureGuardRejectsViolations 使用隔离临时目录与违规样例表验证守卫能准确拦截所有违规导入（含 Adapter 互引）。
func TestArchitectureGuardRejectsViolations(t *testing.T) {
	cases := []struct {
		name    string
		relFile string
		impPath string
		wantErr bool
	}{
		{
			name:    "domain importing stdlib is allowed",
			relFile: "internal/iam/domain/user.go",
			impPath: "time",
			wantErr: false,
		},
		{
			name:    "domain importing bcrypt is rejected",
			relFile: "internal/iam/domain/user.go",
			impPath: "golang.org/x/crypto/bcrypt",
			wantErr: true,
		},
		{
			name:    "domain importing platform is rejected",
			relFile: "internal/iam/domain/user.go",
			impPath: modulePrefix + "/internal/platform/apperr",
			wantErr: true,
		},
		{
			name:    "app importing own domain and pure ctxmeta is allowed",
			relFile: "internal/iam/app/user_service.go",
			impPath: modulePrefix + "/internal/platform/ctxmeta",
			wantErr: false,
		},
		{
			name:    "app importing framework platform httpx is rejected",
			relFile: "internal/iam/app/user_service.go",
			impPath: modulePrefix + "/internal/platform/httpx",
			wantErr: true,
		},
		{
			name:    "app importing gorm is rejected",
			relFile: "internal/iam/app/user_service.go",
			impPath: "gorm.io/gorm",
			wantErr: true,
		},
		{
			name:    "ctxmeta importing gin is rejected",
			relFile: "internal/platform/ctxmeta/ctx.go",
			impPath: "github.com/gin-gonic/gin",
			wantErr: true,
		},
		{
			name:    "platform importing business domain is rejected",
			relFile: "internal/platform/httpx/auth.go",
			impPath: modulePrefix + "/internal/iam/domain",
			wantErr: true,
		},
		{
			name:    "adapter/http importing own app and domain is allowed",
			relFile: "internal/iam/adapter/http/handler.go",
			impPath: modulePrefix + "/internal/iam/app",
			wantErr: false,
		},
		{
			name:    "adapter/http importing sibling adapter/persistence is rejected",
			relFile: "internal/iam/adapter/http/handler.go",
			impPath: modulePrefix + "/internal/iam/adapter/persistence",
			wantErr: true,
		},
		{
			name:    "adapter/persistence importing sibling adapter/cache is rejected",
			relFile: "internal/iam/adapter/persistence/user_repo.go",
			impPath: modulePrefix + "/internal/iam/adapter/cache",
			wantErr: true,
		},
		{
			name:    "adapter importing another domain's adapter is rejected",
			relFile: "internal/iam/adapter/http/handler.go",
			impPath: modulePrefix + "/internal/system/adapter/persistence",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkImportRule(tc.relFile, tc.impPath)
			if tc.wantErr && err == nil {
				t.Fatalf("expected violation for %s importing %q, but got nil", tc.relFile, tc.impPath)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %s importing %q: %v", tc.relFile, tc.impPath, err)
			}
		})
	}

	// 同时在隔离临时目录中创建真实的 adapter/http -> adapter/persistence 违规 .go 文件，验证目录扫描器必定报错
	tmpRoot := t.TempDir()
	badFile := filepath.Join(tmpRoot, "internal", "iam", "adapter", "http", "bad.go")
	if err := os.MkdirAll(filepath.Dir(badFile), 0o755); err != nil {
		t.Fatal(err)
	}
	src := `package http
import _ "oneweekonething.com/owot-pro/internal/iam/adapter/persistence"
`
	if err := os.WriteFile(badFile, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateDirectoryTree(tmpRoot); err == nil {
		t.Fatal("expected validateDirectoryTree to fail on adapter/http -> adapter/persistence import, but it passed")
	}
}

func validateDirectoryTree(root string) error {
	for _, forbiddenDir := range []string{"pkg", "common", "utils", "shared", "internal/common", "internal/utils", "internal/shared"} {
		if _, err := os.Stat(filepath.Join(root, forbiddenDir)); err == nil {
			return fmt.Errorf("forbidden directory exists: %s", forbiddenDir)
		}
	}

	internalDir := filepath.Join(root, "internal")
	if _, err := os.Stat(internalDir); os.IsNotExist(err) {
		return nil
	}

	fset := token.NewFileSet()
	return filepath.WalkDir(internalDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}

		for _, imp := range f.Imports {
			impPath := strings.Trim(imp.Path.Value, `"`)
			if ruleErr := checkImportRule(rel, impPath); ruleErr != nil {
				return ruleErr
			}
		}
		return nil
	})
}

func checkImportRule(relFile, impPath string) error {
	parts := strings.Split(filepath.ToSlash(relFile), "/")
	if len(parts) < 3 || parts[0] != "internal" {
		return nil
	}
	topScope := parts[1] // "platform" or domain name ("iam", "system", "infra")

	// 规则 1：internal/platform/* 规则
	if topScope == "platform" {
		if len(parts) >= 3 && (parts[2] == "ctxmeta" || parts[2] == "pagination") {
			if !isStdLib(impPath) {
				return fmt.Errorf("[ArchViolation] pure platform contract file %s imports non-stdlib package %q", relFile, impPath)
			}
		}
		for _, biz := range []string{"/internal/iam", "/internal/system", "/internal/infra"} {
			if impPath == modulePrefix+biz || strings.HasPrefix(impPath, modulePrefix+biz+"/") {
				return fmt.Errorf("[ArchViolation] platform file %s imports business domain package %q", relFile, impPath)
			}
		}
		return nil
	}

	layer := parts[2] // "domain", "app", "adapter"
	domainPrefix := modulePrefix + "/internal/" + topScope

	switch layer {
	case "domain":
		// 规则 2：internal/<domain>/domain 仅允许 Go 标准库
		if !isStdLib(impPath) {
			return fmt.Errorf("[ArchViolation] domain file %s imports non-stdlib package %q", relFile, impPath)
		}

	case "app":
		// 规则 3：internal/<domain>/app 仅允许标准库、本领域 domain、internal/platform/{ctxmeta,pagination}
		allowedDomainPkg := domainPrefix + "/domain"
		allowedCtxPkg := modulePrefix + "/internal/platform/ctxmeta"
		allowedPagePkg := modulePrefix + "/internal/platform/pagination"
		if !isStdLib(impPath) && impPath != allowedDomainPkg && impPath != allowedCtxPkg && impPath != allowedPagePkg {
			return fmt.Errorf("[ArchViolation] app file %s imports disallowed package %q", relFile, impPath)
		}

	case "adapter":
		// 规则 4：Adapter 子包之间严禁互相引用（例如 adapter/http 严禁导入 adapter/persistence），也严禁导入其他领域的 adapter
		if strings.HasPrefix(impPath, modulePrefix+"/internal/") && strings.Contains(impPath, "/adapter") {
			if len(parts) >= 4 {
				adapterKind := parts[3] // e.g., "http", "persistence", "cache"
				ownAdapterPrefix := domainPrefix + "/adapter/" + adapterKind
				if impPath != ownAdapterPrefix && !strings.HasPrefix(impPath, ownAdapterPrefix+"/") {
					return fmt.Errorf("[ArchViolation] adapter file %s (%s) imports sibling or foreign adapter package %q", relFile, adapterKind, impPath)
				}
			} else {
				return fmt.Errorf("[ArchViolation] adapter file %s imports adapter package %q", relFile, impPath)
			}
		}
	}

	return nil
}

// isStdLib 通过包路径首段是否包含 '.' 判断是否为 Go 标准库包。
func isStdLib(impPath string) bool {
	first := impPath
	if idx := strings.IndexByte(impPath, '/'); idx >= 0 {
		first = impPath[:idx]
	}
	return !strings.Contains(first, ".")
}
