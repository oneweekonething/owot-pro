// Package security implements password hashing outside the business core.
package security

import "golang.org/x/crypto/bcrypt"

type PasswordHasher struct{}

func NewPasswordHasher() *PasswordHasher { return &PasswordHasher{} }
func (*PasswordHasher) Hash(plain string) (string, error) {
	value, err := bcrypt.GenerateFromPassword([]byte(plain), 10)
	return string(value), err
}
func (*PasswordHasher) Compare(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
