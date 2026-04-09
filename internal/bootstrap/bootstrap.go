package bootstrap

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/model/creds"
	"codeberg.org/megakuul/cloudjam/internal/model/role"
	"codeberg.org/megakuul/cloudjam/internal/model/user"
	"github.com/alexedwards/argon2id"
	"gocloud.dev/docstore"
)

// CreateAdministrator creates an administrator account 'admin' with credentials and admin role if not existing already.
// Returns the temporary authentication code if the user was generated.
func CreateAdministrator(ctx context.Context, email string, coll *docstore.Collection) (string, error) {
	// perform a "peek" to the credentials for graceful handling of the happy path.
	// problem is that the mongodb driver does not always yield understandable "AlreadyExist" errors on atomic transactions.
	if err := coll.Get(ctx, &creds.Data{
		PK: creds.Key.New(email),
		SK: creds.SortData.New(""),
	}); err == nil {
		return "", nil
	}

	admin := &user.Data{
		PK:          user.Key.New("0"),
		SK:          user.SortData.New(""),
		Username:    "admin",
		Description: "Administrator account",
		Email:       email,
		CreatedAt:   time.Now(),
		Privileged:  true,
		Role:        "0",
	}
	password := rand.Text()
	hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return "", fmt.Errorf("failed to create argon2id hash: %v", err)
	}
	adminCreds := &creds.Data{
		PK:             creds.Key.New(email),
		SK:             creds.SortData.New(""),
		Active:         false,
		UserId:         "0",
		Code:           hash,
		CodeExpiration: time.Now().Add(time.Hour * 8760),
	}
	adminRole := &role.Data{
		PK:             role.Key.New("0"),
		SK:             role.SortData.New(""),
		Name:           "admin",
		Description:    "Provides unlimited administrator access",
		Builtin:        true,
		ProcedureExprs: []string{"**/*"},
	}

	return password, coll.Actions().AtomicWrites().
		Create(admin).
		Create(adminCreds).
		Create(adminRole).Do(ctx)
}
