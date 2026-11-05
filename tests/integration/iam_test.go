package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	iamcache "oneweekonething.com/owot-pro/internal/iam/adapter/cache"
	"oneweekonething.com/owot-pro/internal/iam/adapter/persistence"
	"oneweekonething.com/owot-pro/internal/iam/adapter/security"
	"oneweekonething.com/owot-pro/internal/iam/app"
	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/outbox"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type iamFaultBroadcaster struct {
	failed atomic.Bool
	next   outbox.Broadcaster
}

func (b *iamFaultBroadcaster) Broadcast(ctx context.Context, event outbox.Event) error {
	if b.failed.Load() {
		return errors.New("test: broadcaster offline")
	}
	return b.next.Broadcast(ctx, event)
}

type iamRejectPublisher struct{}

func (iamRejectPublisher) Publish(context.Context, string, []byte) error {
	return errors.New("test: outbox insert failed")
}

type iamObservedLocker struct {
	app.MutationLocker
	before func(int64)
}

func (l *iamObservedLocker) LockTenant(ctx context.Context, id int64) error {
	l.before(id)
	return l.MutationLocker.LockTenant(ctx, id)
}

func realIAM(t *testing.T) (*gorm.DB, *redis.Client, *app.Service, *outbox.Store, *iamFaultBroadcaster) {
	t.Helper()
	base, client := realSystemDependencies(t)
	// An isolated schema proves the checked-in DDL itself, independently of
	// AutoMigrate or earlier runs. The test DSN needs CREATE/DROP DATABASE rights.
	name := fmt.Sprintf("owot_iam_%d", time.Now().UnixNano())
	if err := base.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin").Error; err != nil {
		t.Fatal(err)
	}
	config, err := mysqldriver.ParseDSN(os.Getenv("OWOT_TEST_MYSQL_DSN"))
	if err != nil {
		t.Fatal("invalid test DSN")
	}
	config.DBName, config.ParseTime, config.Loc = name, true, time.UTC
	db, err := gorm.Open(mysql.Open(config.FormatDSN()), &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent), NowFunc: func() time.Time { return time.Now().UTC() }})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
		if err := base.Exec("DROP DATABASE `" + name + "`").Error; err != nil {
			t.Error(err)
		}
	})
	if err := dbx.Install(db); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{"001_system.sql", "002_iam.sql"} {
		ddl, err := os.ReadFile("../../migrations/mysql/" + filename)
		if err != nil {
			t.Fatal(err)
		}
		lines := []string{}
		for _, line := range strings.Split(string(ddl), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "--") {
				lines = append(lines, line)
			}
		}
		for _, statement := range strings.Split(strings.Join(lines, "\n"), ";") {
			if strings.TrimSpace(statement) != "" {
				if err := db.Exec(statement).Error; err != nil {
					t.Fatalf("apply %s: %v", filename, err)
				}
			}
		}
	}
	if err := persistence.CheckSchema(db); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	broadcaster := &iamFaultBroadcaster{next: iamcache.NewBroadcaster(client)}
	store := outbox.NewStore(db, broadcaster, log)
	cache, err := iamcache.NewPermissions(client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cache.Close)
	s := app.NewService(app.Dependencies{Repositories: app.Repositories{Users: persistence.NewUserRepository(db), Depts: persistence.NewDeptRepository(db), Posts: persistence.NewPostRepository(db), Roles: persistence.NewRoleRepository(db), Menus: persistence.NewMenuRepository(db), Tenants: persistence.NewTenantRepository(db), Packages: persistence.NewPackageRepository(db), Clients: persistence.NewClientRepository(db), Tokens: persistence.NewTokenRepository(db), Links: persistence.NewAssociations(db), Locks: persistence.NewLocker(db)}, Tx: dbx.NewTxManager(db), Outbox: store, Hasher: security.NewPasswordHasher(), TokenCache: iamcache.NewTokens(client, log), Permissions: cache})
	if err := s.SeedDefaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedDefaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db, client, s, store, broadcaster
}
func iamRoot(tenant int64) context.Context {
	return ctxmeta.WithLoginUser(ctxmeta.WithTenantID(context.Background(), tenant), ctxmeta.LoginUser{ID: 1, TenantID: 1, UserType: 1, SuperAdmin: true})
}
func iamAll() app.Query { return app.Query{PageQuery: pagination.PageQuery{PageSize: -1}} }
func iamTenant(t *testing.T, s *app.Service, quota int) (int64, int64, []int64) {
	t.Helper()
	ctx := iamRoot(1)
	menus, err := s.Menus.List(ctx, iamAll())
	if err != nil {
		t.Fatal(err)
	}
	ids := []int64{}
	for _, m := range menus.List {
		if m.Permission == "system:user:query" || m.Permission == "system:post:query" {
			ids = append(ids, m.ID)
		}
	}
	pack, err := s.SavePackage(ctx, domain.TenantPackage{Name: "测试套餐", MenuIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := s.SaveTenant(ctx, domain.Tenant{Name: "测试租户", ContactName: "管理员", PackageID: pack, AccountCount: quota, ExpireTime: time.Now().Add(time.Hour), Websites: []string{}}, "tenantadmin", "password")
	if err != nil {
		t.Fatal(err)
	}
	return tenant, pack, ids
}

func TestIAMRealTransactionsQuotaAndIsolation(t *testing.T) {
	db, client, s, _, _ := realIAM(t)
	tenant, pack, _ := iamTenant(t, s, 2)
	ctx := iamRoot(tenant)
	if _, err := s.Users.List(context.Background(), iamAll()); !errors.Is(err, dbx.ErrTenantRequired) {
		t.Fatal("missing tenant did not fail closed", err)
	}
	if _, err := s.Users.Get(ctx, 1); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatal("cross-tenant read", err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for i := range 8 {
		wg.Go(func() {
			_, err := s.SaveUser(ctx, domain.AdminUser{Username: fmt.Sprintf("concurrent%d", i), Nickname: "并发账号"}, "password")
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, domain.ErrQuotaExceeded) {
				failures <- err
			}
		})
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if successes.Load() != 1 {
		t.Fatalf("quota race: %d accounts created", successes.Load())
	}
	users, err := s.Users.List(ctx, iamAll())
	if err != nil || users.Total != 2 {
		t.Fatal(users, err)
	}
	// Data-scope OR predicates must remain inside the tenant AND predicate.
	dataCtx := ctxmeta.WithDataScope(ctx, ctxmeta.DataScope{Self: true, UserID: 1, DeptIDs: []int64{users.List[0].DeptID}})
	scoped, err := s.Users.List(dataCtx, iamAll())
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range scoped.List {
		if u.TenantID != tenant {
			t.Fatal("data-scope OR escaped tenant", u)
		}
	}
	copyService := app.NewService(s.Dependencies)
	copyService.Outbox = iamRejectPublisher{}
	var before int64
	if err := db.Table("sys_outbox_event").Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	_, err = copyService.SaveTenant(iamRoot(1), domain.Tenant{Name: "应当回滚", ContactName: "管理员", PackageID: pack, AccountCount: 2, ExpireTime: time.Now().Add(time.Hour)}, "rollbackuser", "password")
	if err == nil {
		t.Fatal("outbox failure ignored")
	}
	q := iamAll()
	q.Name = "应当回滚"
	tenants, err := s.Tenants.List(iamRoot(1), q)
	if err != nil || tenants.Total != 0 {
		t.Fatal("partial tenant commit", tenants, err)
	}
	var after int64
	if err := db.Table("sys_outbox_event").Count(&after).Error; err != nil || before != after {
		t.Fatal("outbox/tenant not atomic", before, after, err)
	}
	// Empty contacts do not collide; active usernames can be reused repeatedly.
	for range 3 {
		id, err := s.SaveUser(iamRoot(1), domain.AdminUser{Username: "reusable", Nickname: "复用账号"}, "password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.SaveUser(iamRoot(1), domain.AdminUser{Username: "reusable", Nickname: "重复账号"}, "password"); !errors.Is(err, domain.ErrDuplicate) {
			t.Fatal("unique constraint missing", err)
		}
		if err := s.DeleteUsers(iamRoot(1), []int64{id}); err != nil {
			t.Fatal(err)
		}
	}
	login, err := s.Login(ctx, app.LoginRequest{Username: "tenantadmin", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	admin := users.List[0]
	if err := s.UpdateUserStatus(ctx, admin.ID, 1); err != nil {
		t.Fatal(err)
	}
	if client.Exists(ctx, "oauth2_access_token:"+login.AccessToken).Val() != 0 {
		t.Fatal("disabled session survived in real Redis")
	}
	if _, err := s.Verify(context.Background(), login.AccessToken); err == nil {
		t.Fatal("disabled token accepted")
	}
}

func TestIAMRealOutboxRecoveryAndCacheVersions(t *testing.T) {
	db, client, s, store, broadcaster := realIAM(t)
	tenant, _, menus := iamTenant(t, s, 10)
	ctx := iamRoot(tenant)
	userID, err := s.SaveUser(ctx, domain.AdminUser{Username: "cacheduser", Nickname: "缓存用户"}, "password")
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.SaveRole(ctx, domain.Role{Name: "缓存角色", Code: "cache"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AssignRoleMenus(ctx, role, menus); err != nil {
		t.Fatal(err)
	}
	if err := s.AssignUserRoles(ctx, userID, []int64{role}); err != nil {
		t.Fatal(err)
	}
	user := ctxmeta.LoginUser{ID: userID, TenantID: tenant, UserType: 1}
	for range 3 {
		if ok, err := s.HasPermission(ctx, user, "system:user:query"); err != nil || !ok {
			t.Fatal(ok, err)
		}
	}
	// The second node deliberately has no Pub/Sub subscriber.
	secondCache, err := iamcache.NewPermissions(client)
	if err != nil {
		t.Fatal(err)
	}
	defer secondCache.Close()
	second := app.NewService(s.Dependencies)
	second.Permissions = secondCache
	if ok, err := second.HasPermission(ctx, user, "system:user:query"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	epochKey := fmt.Sprintf("iam:perm_ver:%d", tenant)
	before := client.Get(ctx, epochKey).Val()
	broadcaster.failed.Store(true)
	if err := s.AssignRoleMenus(ctx, role, []int64{}); err != nil {
		t.Fatal(err)
	}
	var pending int64
	if err := db.Table("sys_outbox_event").Where("topic = ? AND tenant_id = ? AND status = 0", "iam.perm.invalidate", tenant).Count(&pending).Error; err != nil || pending == 0 {
		t.Fatal("missing durable invalidation", pending, err)
	}
	broadcaster.failed.Store(false)
	relayCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { store.Run(relayCtx); close(done) }()
	defer func() { cancel(); <-done }()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("permission revocation was not replayed within two seconds")
		case <-tick.C:
			if client.Get(ctx, epochKey).Val() == before {
				continue
			}
			for _, node := range []*app.Service{s, second} {
				if ok, err := node.HasPermission(ctx, user, "system:user:query"); err != nil || ok {
					t.Fatal("recovered epoch retained revoked permission", ok, err)
				}
			}
			return
		}
	}
}

func TestIAMPackageSyncReadsRoleCommittedWhileWaiting(t *testing.T) {
	_, _, s, _, _ := realIAM(t)
	tenant, pack, menus := iamTenant(t, s, 10)
	ctx, cancel := context.WithTimeout(iamRoot(tenant), 10*time.Second)
	defer cancel()
	locked, writing, waiting := make(chan struct{}), make(chan struct{}), make(chan struct{})
	roleResult := make(chan int64, 1)
	roleErr := make(chan error, 1)
	go func() {
		err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
			if err := s.Locks.LockTenant(ctx, tenant); err != nil {
				return err
			}
			close(locked)
			select {
			case <-writing:
			case <-ctx.Done():
				return ctx.Err()
			}
			role, err := s.SaveRole(ctx, domain.Role{Name: "等待期间新增", Code: "during_wait"})
			if err != nil {
				return err
			}
			if err := s.AssignRoleMenus(ctx, role, menus); err != nil {
				return err
			}
			roleResult <- role
			return nil
		})
		roleErr <- err
	}()
	select {
	case <-locked:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	updater := app.NewService(s.Dependencies)
	var once sync.Once
	updater.Locks = &iamObservedLocker{MutationLocker: s.Locks, before: func(id int64) {
		if id == tenant {
			once.Do(func() { close(waiting) })
		}
	}}
	updateErr := make(chan error, 1)
	go func() {
		_, err := updater.SavePackage(iamRoot(1), domain.TenantPackage{ID: pack, Name: "测试套餐", MenuIDs: menus[:1]})
		updateErr <- err
	}()
	select {
	case <-waiting:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	close(writing)
	if err := <-roleErr; err != nil {
		t.Fatal(err)
	}
	if err := <-updateErr; err != nil {
		t.Fatal(err)
	}
	role := <-roleResult
	ids, err := s.RoleMenuIDs(ctx, role)
	if err != nil || !slices.Equal(ids, menus[:1]) {
		t.Fatal("repeatable-read snapshot missed newly committed role", ids, err)
	}
}
