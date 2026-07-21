package middleware

import (
	"context"
	"errors"

	"github.com/nglong14/llmgateway/internal/models"
	"github.com/nglong14/llmgateway/internal/repository"
)

type apiKeyLookup interface {
	GetByHash(ctx context.Context, keyHash string) (*models.APIKey, error)
}

type DatabaseKeyValidator struct {
	keys apiKeyLookup
}

func NewDatabaseKeyValidator(keys apiKeyLookup) *DatabaseKeyValidator {
	return &DatabaseKeyValidator{keys: keys}
}

func (v *DatabaseKeyValidator) Validate(ctx context.Context, token string) (*KeyInfo, error) {
	key, err := v.keys.GetByHash(ctx, hashAPIKey(token))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &KeyInfo{
		Name:   key.Name,
		UserID: key.UserID,
	}, nil
}
