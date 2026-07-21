package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AccessTokenTTL is the staff access token lifetime, per the approved
// decision (15 minutes). It is also reported as TokenPair.expires_in.
const AccessTokenTTL = 15 * time.Minute

// ErrAccessTokenInvalid covers every JWT verification failure: bad
// signature, wrong algorithm, expired, malformed. The specific reason is
// safe to log but must never reach the client as anything but a uniform 401.
var ErrAccessTokenInvalid = errors.New("auth: invalid or expired access token")

// AccessClaims is the payload of the staff access JWT. Role travels in the
// token for convenience/logging only — RequireAdminJWT always re-reads the
// authoritative role from PostgreSQL before trusting it for RBAC (see
// AdminIdentity's doc comment), so a stale claim can never grant access.
type AccessClaims struct {
	AdminID int64  `json:"admin_id"`
	Role    string `json:"role"`
	jwt.RegisteredClaims
}

// IssueAccessToken signs a new access JWT for adminID/role, expiring
// AccessTokenTTL after now.
func IssueAccessToken(secret []byte, adminID int64, role string, now time.Time) (string, error) {
	claims := AccessClaims{
		AdminID: adminID,
		Role:    role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signed, nil
}

// ParseAccessToken verifies tokenString's signature and expiry and returns
// its claims. It pins the signing method to HS256 so a token signed (or
// re-signed) with a different algorithm — e.g. "none", or an asymmetric
// algorithm an attacker could satisfy with a public key — is rejected
// outright, closing the classic JWT "alg confusion" hole.
func ParseAccessToken(secret []byte, tokenString string) (*AccessClaims, error) {
	var claims AccessClaims
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid {
		return nil, ErrAccessTokenInvalid
	}
	return &claims, nil
}
