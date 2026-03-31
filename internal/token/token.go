// package token provides an issuer used to issue and verify access tokens.
package token

import (
	"context"
	"fmt"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Issuer struct {
	name           string
	lifetime       time.Duration
	method         jwt.SigningMethod
	public         any
	privateFactory func(context.Context) any
}

func New(name string, lifetime time.Duration, method jwt.SigningMethod, public any, privateFactory func(context.Context) any) *Issuer {
	return &Issuer{
		name:           name,
		lifetime:       lifetime,
		method:         method,
		public:         public,
		privateFactory: privateFactory,
	}
}

type TokenClaims struct {
	jwt.RegisteredClaims
	Email   string `json:"email,omitempty"`
	Refresh bool   `json:"refresh,omitempty"`
}

func (i *Issuer) Issue(ctx context.Context, subject, email string) (string, error) {
	token := jwt.NewWithClaims(i.method, &TokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.New().String(),
			Audience:  jwt.ClaimStrings{i.name}, // rp and resource server are the same entity, so aud == iss
			Issuer:    i.name,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(i.lifetime)),
			Subject:   subject,
		},
		Email:   email,
		Refresh: false,
	})
	signedToken, err := token.SignedString(i.privateFactory(ctx))
	if err != nil {
		return "", connect.NewError(connect.CodeInternal, err)
	}
	return signedToken, nil
}

func (i *Issuer) Verify(ctx context.Context, token string) (*TokenClaims, error) {
	claims := &TokenClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if i.method.Alg() != t.Method.Alg() {
			return nil, fmt.Errorf("token algorithm mismatch detected")
		}
		return i.public, nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, err)
	}
	if !slices.Contains(claims.Audience, i.name) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("token was not issued for this audience"))
	}
	return claims, nil
}
