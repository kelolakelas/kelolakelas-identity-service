package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type RedisService struct {
	client *redis.Client
}

func NewRedisService(client *redis.Client) *RedisService {
	return &RedisService{client: client}
}

func (s *RedisService) SaveRolePermissions(ctx context.Context, tenantID, roleID uuid.UUID, permissions []string, ttl time.Duration) error {
	key := fmt.Sprintf("tenant:%s:role:%s:permissions", tenantID, roleID)
	data, err := json.Marshal(permissions)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, key, data, ttl).Err()
}

func (s *RedisService) GetRolePermissions(ctx context.Context, tenantID, roleID uuid.UUID) ([]string, error) {
	key := fmt.Sprintf("tenant:%s:role:%s:permissions", tenantID, roleID)
	val, err := s.client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var permissions []string
	if err := json.Unmarshal([]byte(val), &permissions); err != nil {
		return nil, err
	}
	return permissions, nil
}
