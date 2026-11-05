package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type LoginRequest struct{ Username, Password, UserIP, UserAgent string }
type LoginResponse struct {
	UserID                    int64
	AccessToken, RefreshToken string
	ExpiresTime               time.Time
}

func (s *Service) Login(ctx context.Context, req LoginRequest) (LoginResponse, error) {
	if len(req.Username) > 30 || req.Username == "" || len(req.Password) > 72 {
		return LoginResponse{}, domain.ErrCredentials
	}
	ctx = ctxmeta.WithoutDataScope(ctx)
	u, err := s.checkCredentials(ctx, req)
	log := LoginLog{UserID: u.ID, Username: req.Username, UserIP: req.UserIP, UserAgent: req.UserAgent, LogType: 100}
	var access domain.AccessToken
	if err == nil {
		err = s.mutate(ctx, false, func(ctx context.Context) error {
			if err := s.ValidateTenant(ctx, ctxmeta.TenantID(ctx)); err != nil {
				return err
			}
			current, err := s.Users.Get(ctx, u.ID)
			if errors.Is(err, domain.ErrUserNotFound) {
				return domain.ErrCredentials
			}
			if err != nil {
				return err
			}
			// The expensive bcrypt check ran without the tenant write lock. Re-read
			// the account under that lock before issuing a session.
			if current.Username != u.Username || current.PasswordHash != u.PasswordHash {
				return domain.ErrCredentials
			}
			if current.Status != domain.StatusEnabled {
				return domain.ErrDisabled
			}
			client, err := s.client(ctx, "default")
			if err != nil {
				return err
			}
			ctx = ctxmeta.WithLoginUser(ctx, ctxmeta.LoginUser{ID: u.ID, TenantID: u.TenantID, UserType: 1})
			current.LoginIP, current.LoginDate = req.UserIP, time.Now()
			if err := s.Users.Save(ctx, &current); err != nil {
				return err
			}
			if s.LoginLogs != nil {
				if err := s.LoginLogs.RecordLogin(ctx, log); err != nil {
					return wrap("record successful login", err)
				}
			}
			access, err = s.issue(ctx, u.ID, client)
			return err
		})
	}
	if err != nil {
		if access.AccessToken != "" {
			err = errors.Join(err, s.discardIssued(ctx, access.AccessToken))
		}
		if s.LoginLogs != nil && (errors.Is(err, domain.ErrCredentials) || errors.Is(err, domain.ErrDisabled)) {
			log.Result = 10
			if errors.Is(err, domain.ErrDisabled) {
				log.Result = 20
			}
			logCtx, cancel := context.WithTimeout(ctxmeta.CloneContext(ctx), 2*time.Second)
			_ = s.LoginLogs.RecordLogin(logCtx, log)
			cancel()
		}
		return LoginResponse{}, err
	}
	return response(access), nil
}

func (s *Service) checkCredentials(ctx context.Context, req LoginRequest) (domain.AdminUser, error) {
	if err := s.ValidateTenant(ctx, ctxmeta.TenantID(ctx)); err != nil {
		return domain.AdminUser{}, err
	}
	q := all()
	q.ExactUsername = req.Username
	users, err := s.Users.List(ctx, q)
	if err != nil {
		return domain.AdminUser{}, wrap("lookup login account", err)
	}
	if len(users.List) != 1 {
		return domain.AdminUser{}, domain.ErrCredentials
	}
	u := users.List[0]
	if s.Hasher.Compare(u.PasswordHash, req.Password) != nil {
		return u, domain.ErrCredentials
	}
	if u.Status != domain.StatusEnabled {
		return u, domain.ErrDisabled
	}
	return u, nil
}

func (s *Service) discardIssued(ctx context.Context, token string) error {
	cleanup, cancel := context.WithTimeout(ctxmeta.CloneContext(ctx), time.Second)
	defer cancel()
	return s.TokenCache.Delete(cleanup, []string{token})
}

func response(t domain.AccessToken) LoginResponse {
	return LoginResponse{UserID: t.UserID, AccessToken: t.AccessToken, RefreshToken: t.RefreshToken, ExpiresTime: t.ExpiresTime}
}
func randomToken() (string, error) {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}

func (s *Service) client(ctx context.Context, id string) (domain.OAuth2Client, error) {
	q := all()
	q.ClientID = id
	clients, err := s.Clients.List(ctx, q)
	if err != nil {
		return domain.OAuth2Client{}, err
	}
	if len(clients.List) != 1 || clients.List[0].Status != 0 {
		return domain.OAuth2Client{}, domain.ErrClientNotFound
	}
	return clients.List[0], nil
}
func (s *Service) issue(ctx context.Context, userID int64, c domain.OAuth2Client) (domain.AccessToken, error) {
	a, err := randomToken()
	if err != nil {
		return domain.AccessToken{}, err
	}
	r, err := randomToken()
	if err != nil {
		return domain.AccessToken{}, err
	}
	now := time.Now()
	access := domain.AccessToken{TenantID: ctxmeta.TenantID(ctx), UserID: userID, UserType: 1, ClientID: c.ClientID, AccessToken: a, RefreshToken: r, ExpiresTime: now.Add(time.Duration(c.AccessTokenValiditySeconds) * time.Second)}
	refresh := domain.RefreshToken{TenantID: ctxmeta.TenantID(ctx), UserID: userID, UserType: 1, ClientID: c.ClientID, RefreshToken: r, ExpiresTime: now.Add(time.Duration(c.RefreshTokenValiditySeconds) * time.Second)}
	err = s.Tokens.SaveTokens(ctx, &access, &refresh)
	// The tenant lock stays held until both stores have been written. Verification
	// also checks the durable row, so a failed SQL commit cannot create a session.
	if err == nil {
		err = s.TokenCache.Set(ctx, access)
	}
	return access, err
}

// Verify always reads Redis, then confirms the durable session and current user.
// No database fallback may resurrect a revoked Redis session.
func (s *Service) Verify(ctx context.Context, token string) (ctxmeta.LoginUser, error) {
	t, err := s.TokenCache.Get(ctx, token)
	if err != nil {
		return ctxmeta.LoginUser{}, err
	}
	if !t.ExpiresTime.After(time.Now()) || t.UserType != 1 {
		return ctxmeta.LoginUser{}, domain.ErrTokenInvalid
	}
	ctx = ctxmeta.WithTenantID(ctxmeta.WithoutDataScope(ctx), t.TenantID)
	stored, err := s.Tokens.FindAccess(ctx, AccessSelector{Token: token})
	if err != nil {
		return ctxmeta.LoginUser{}, wrap("read access token", err)
	}
	if stored.UserID != t.UserID || stored.TenantID != t.TenantID || !stored.ExpiresTime.After(time.Now()) {
		return ctxmeta.LoginUser{}, domain.ErrTokenInvalid
	}
	u, err := s.Users.Get(ctx, t.UserID)
	if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
		return ctxmeta.LoginUser{}, wrap("read token user", err)
	}
	if err != nil || u.Status != 0 {
		return ctxmeta.LoginUser{}, domain.ErrTokenInvalid
	}
	if err := s.ValidateTenant(ctx, t.TenantID); err != nil {
		if !errors.Is(err, domain.ErrTenantNotFound) && !errors.Is(err, domain.ErrTenantUnavailable) && !errors.Is(err, domain.ErrPackageNotFound) {
			return ctxmeta.LoginUser{}, wrap("validate token tenant", err)
		}
		return ctxmeta.LoginUser{}, domain.ErrTokenInvalid
	}
	if _, err := s.client(ctx, t.ClientID); err != nil {
		if !errors.Is(err, domain.ErrClientNotFound) {
			return ctxmeta.LoginUser{}, wrap("validate token client", err)
		}
		return ctxmeta.LoginUser{}, domain.ErrTokenInvalid
	}
	info, err := s.Snapshot(ctx, t.UserID)
	if err != nil {
		return ctxmeta.LoginUser{}, err
	}
	user := ctxmeta.LoginUser{ID: t.UserID, TenantID: t.TenantID, UserType: t.UserType}
	for _, r := range info.Roles {
		if r.Code == "super_admin" && r.BuiltIn() && t.TenantID == 1 {
			user.SuperAdmin = true
		}
	}
	return user, nil
}

func (s *Service) revoke(ctx context.Context, selection TokenSelection) error {
	tokens, err := s.Tokens.Revoke(ctx, selection)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(tokens))
	for _, t := range tokens {
		keys = append(keys, t.AccessToken)
	}
	return s.TokenCache.DeleteAfterCommit(ctx, keys)
}
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" || len(token) > 255 {
		return nil
	}
	// Revocation uses the durable row even after Redis expires the access key.
	// The unverified credential can only delete its own tenant-scoped session.
	selection := AccessSelector{Token: token}
	_, err := s.Tokens.FindAccess(ctx, selection)
	if errors.Is(err, domain.ErrTokenInvalid) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.mutate(ctx, false, func(ctx context.Context) error {
		t, err := s.Tokens.FindAccess(ctx, selection)
		if errors.Is(err, domain.ErrTokenInvalid) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.revoke(ctx, TokenSelection{AccessToken: token}); err != nil {
			return err
		}
		return s.recordSessionEnd(ctx, t.UserID, 200)
	})
}

func (s *Service) recordSessionEnd(ctx context.Context, userID int64, logType int) error {
	if s.LoginLogs == nil {
		return nil
	}
	user, err := s.Users.Get(ctxmeta.WithoutDataScope(ctx), userID)
	if err != nil {
		return err
	}
	return s.LoginLogs.RecordLogin(ctx, LoginLog{UserID: user.ID, Username: user.Username, LogType: logType, Result: 0})
}
func (s *Service) Refresh(ctx context.Context, token string) (LoginResponse, error) {
	if token == "" || len(token) > 255 {
		return LoginResponse{}, domain.ErrTokenInvalid
	}
	var access domain.AccessToken
	err := s.mutate(ctx, false, func(ctx context.Context) error {
		r, err := s.Tokens.FindRefresh(ctx, token)
		if err != nil {
			return err
		}
		if !r.ExpiresTime.After(time.Now()) || r.UserType != 1 {
			return domain.ErrTokenInvalid
		}
		u, err := s.Users.Get(ctxmeta.WithoutDataScope(ctx), r.UserID)
		if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
			return wrap("read refresh token user", err)
		}
		if err != nil || u.Status != 0 {
			return domain.ErrTokenInvalid
		}
		if err := s.ValidateTenant(ctx, r.TenantID); err != nil {
			return err
		}
		client, err := s.client(ctx, r.ClientID)
		if err != nil {
			return err
		}
		if err := s.revoke(ctx, TokenSelection{RefreshToken: token}); err != nil {
			return err
		}
		access, err = s.issue(ctx, u.ID, client)
		return err
	})
	if err != nil {
		if access.AccessToken != "" {
			err = errors.Join(err, s.discardIssued(ctx, access.AccessToken))
		}
		return LoginResponse{}, err
	}
	return response(access), nil
}
func (s *Service) PageTokens(ctx context.Context, q Query) (pagination.PageResult[domain.AccessToken], error) {
	if !validQuery(q) {
		return pagination.PageResult[domain.AccessToken]{}, domain.ErrInvalidArgument
	}
	q.PageQuery = q.Normalize()
	return s.Tokens.PageTokens(ctx, q)
}
func (s *Service) ForceLogout(ctx context.Context, selection AccessSelector) error {
	if selection.ID <= 0 && selection.Token == "" {
		return domain.ErrInvalidArgument
	}
	return s.mutate(ctx, false, func(ctx context.Context) error {
		t, err := s.Tokens.FindAccess(ctx, selection)
		if errors.Is(err, domain.ErrTokenInvalid) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.revoke(ctx, TokenSelection{AccessToken: t.AccessToken}); err != nil {
			return err
		}
		return s.recordSessionEnd(ctx, t.UserID, 202)
	})
}

func (s *Service) PageClients(ctx context.Context, q Query) (pagination.PageResult[domain.OAuth2Client], error) {
	if err := requireSuper(ctx); err != nil {
		return pagination.PageResult[domain.OAuth2Client]{}, err
	}
	return checkedPage(ctx, s.Clients, q)
}
func (s *Service) GetClient(ctx context.Context, id int64) (domain.OAuth2Client, error) {
	if err := requireSuper(ctx); err != nil {
		return domain.OAuth2Client{}, err
	}
	return checkedGet(ctx, s.Clients, id)
}
func (s *Service) SaveClient(ctx context.Context, v domain.OAuth2Client) (int64, error) {
	err := s.mutate(ctx, true, func(ctx context.Context) error {
		if v.ID > 0 {
			old, err := s.Clients.Get(ctx, v.ID)
			if err != nil {
				return err
			}
			if old.ClientID == "default" && (v.ClientID != old.ClientID || v.Status != domain.StatusEnabled) {
				return domain.ErrProtected
			}
		}
		if err := v.Validate(); err != nil {
			return err
		}
		return s.Clients.Save(ctx, &v)
	})
	return v.ID, err
}
func (s *Service) DeleteClient(ctx context.Context, id int64) error {
	return s.mutate(ctx, true, func(ctx context.Context) error {
		v, err := s.Clients.Get(ctx, id)
		if err != nil {
			return err
		}
		if v.ClientID == "default" {
			return domain.ErrProtected
		}
		return s.Clients.Delete(ctx, id)
	})
}
