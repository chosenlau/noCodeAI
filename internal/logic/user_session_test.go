package logic

import (
	"context"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestUserService_UserLogout_DeletesSessionAndIndex(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := NewUserService(nil, rdb)

	ctx := context.Background()
	userId := int64(123)
	sessionId := "session:testtoken"

	if err := rdb.Set(ctx, sessionId, "{}", sessionTTL).Err(); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := rdb.SAdd(ctx, svc.userSessionsKey(userId), sessionId).Err(); err != nil {
		t.Fatalf("seed index: %v", err)
	}

	if err := svc.UserLogout(ctx, sessionId, userId); err != nil {
		t.Fatalf("UserLogout: %v", err)
	}

	if _, err := rdb.Get(ctx, sessionId).Result(); err != redis.Nil {
		t.Fatalf("expected session deleted, got err=%v", err)
	}
	if members, err := rdb.SMembers(ctx, svc.userSessionsKey(userId)).Result(); err != nil || len(members) != 0 {
		t.Fatalf("expected index updated, members=%v err=%v", members, err)
	}
}

func TestUserService_invalidateUserSessions_DeletesAllSessions(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	svc := NewUserService(nil, rdb)

	ctx := context.Background()
	userId := int64(123)
	sessionA := "session:a"
	sessionB := "session:b"

	if err := rdb.Set(ctx, sessionA, "{}", sessionTTL).Err(); err != nil {
		t.Fatalf("seed sessionA: %v", err)
	}
	if err := rdb.Set(ctx, sessionB, "{}", sessionTTL).Err(); err != nil {
		t.Fatalf("seed sessionB: %v", err)
	}
	if err := rdb.SAdd(ctx, svc.userSessionsKey(userId), sessionA, sessionB).Err(); err != nil {
		t.Fatalf("seed index: %v", err)
	}

	if err := svc.invalidateUserSessions(ctx, userId); err != nil {
		t.Fatalf("invalidateUserSessions: %v", err)
	}

	for _, sid := range []string{sessionA, sessionB} {
		if _, err := rdb.Get(ctx, sid).Result(); err != redis.Nil {
			t.Fatalf("expected %s deleted, got err=%v", sid, err)
		}
	}
	if exists, err := rdb.Exists(ctx, svc.userSessionsKey(userId)).Result(); err != nil || exists != 0 {
		t.Fatalf("expected index key deleted, exists=%v err=%v", exists, err)
	}
}

func TestUserService_generateSessionID_IsRandomAndPrefixed(t *testing.T) {
	svc := NewUserService(nil, redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"}))

	a, err := svc.generateSessionID()
	if err != nil {
		t.Fatalf("generateSessionID: %v", err)
	}
	b, err := svc.generateSessionID()
	if err != nil {
		t.Fatalf("generateSessionID: %v", err)
	}
	if a == b {
		t.Fatalf("expected different session IDs")
	}
	if len(a) <= len(sessionKeyPrefix) || a[:len(sessionKeyPrefix)] != sessionKeyPrefix {
		t.Fatalf("expected session ID prefixed with %q, got %q", sessionKeyPrefix, a)
	}
}
