package domain

import "time"

type OAuth2Client struct {
	ID                                                                         int64
	ClientID, Secret, Name, Logo, Description                                  string
	Status, AccessTokenValiditySeconds, RefreshTokenValiditySeconds            int
	RedirectURIs, AuthorizedGrantTypes, Scopes, AutoApproveScopes, Authorities []string
	AdditionalInformation                                                      string
	CreateTime                                                                 time.Time
}

func (c OAuth2Client) Validate() error {
	if !text(c.ClientID, 255) || !text(c.Secret, 255) || !text(c.Name, 255) || !ValidStatus(c.Status) || c.AccessTokenValiditySeconds <= 0 || c.RefreshTokenValiditySeconds < c.AccessTokenValiditySeconds || c.RefreshTokenValiditySeconds > 315360000 || !optionalText(c.Description, 500) || !optionalText(c.Logo, 255) {
		return ErrInvalidArgument
	}
	return nil
}

type AccessToken struct {
	ID, TenantID, UserID                int64
	UserType                            int
	ClientID, AccessToken, RefreshToken string
	CreateTime, ExpiresTime             time.Time
}
type RefreshToken struct {
	ID, TenantID, UserID    int64
	UserType                int
	ClientID, RefreshToken  string
	CreateTime, ExpiresTime time.Time
}
