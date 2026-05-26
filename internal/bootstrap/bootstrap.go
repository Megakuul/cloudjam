package bootstrap

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/model"
	"github.com/alexedwards/argon2id"
	"github.com/google/uuid"
	"github.com/megakuul/dynamitedb"
)

// CreateAdministrator creates an administrator account 'admin' with credentials and admin role if not existing already.
// Returns the temporary authentication code if the user was generated.
func CreateAdministrator(ctx context.Context, email string, bucket *dynamitedb.Bucket) (string, error) {
	password := rand.Text()
	hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return "", fmt.Errorf("failed to create argon2id hash: %v", err)
	}

	err = dynamitedb.Create(ctx, bucket, &model.User{
		UserID:       dynamitedb.Key("0"),
		PubId:        dynamitedb.Set(uuid.NewString()),
		Username:     dynamitedb.Set("admin"),
		Description:  dynamitedb.Set("Administrator Account"),
		Organization: dynamitedb.Set("Admin"),
		Email:        dynamitedb.Set(email),
		CreatedAt:    dynamitedb.Set(time.Now()),
		Privileged:   dynamitedb.Set(true),
		Role:         dynamitedb.Set("0"),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrAlreadyExists) {
			return "", nil
		}
		return "", fmt.Errorf("user creation: %v", err)
	}

	println(email)
	err = dynamitedb.Put(ctx, bucket, &model.Creds{
		Email:          dynamitedb.Key(email),
		Active:         dynamitedb.Set(false),
		UserId:         dynamitedb.Set("0"),
		Code:           dynamitedb.Set(hash),
		CodeExpiration: dynamitedb.Set(time.Now().Add(time.Hour * 8760)),
		Scope:          dynamitedb.Set(model.ScopeAdmin),
	})
	if err != nil {
		return "", fmt.Errorf("cred insertion: %v", err)
	}

	err = dynamitedb.Put(ctx, bucket, &model.Role{
		RoleID:      dynamitedb.Key("0"),
		Name:        dynamitedb.Set("admin"),
		Description: dynamitedb.Set("Provides unlimited administrator access"),
		Builtin:     dynamitedb.Set(true),
		Permissions: dynamitedb.Set(map[string]string{
			model.ScopeAdmin: "**",
			model.ScopeSelf:  "**",
		}),

		Scope: dynamitedb.Set(model.ScopeAdmin),
	})
	if err != nil {
		return "", fmt.Errorf("role insertion: %v", err)
	}
	return password, nil
}
