package dal

import (
	"testing"

	"github.com/chosenlau/noCodeAI/config"
)

func TestInitRedis_UsesHostPortAddress(t *testing.T) {
	cfg := &config.Config{
		Redis: config.RedisConfig{
			Host: "localhost",
			Port: 6379,
		},
	}

	client := InitRedis(cfg)
	if got, want := client.Options().Addr, "localhost:6379"; got != want {
		t.Fatalf("redis address = %q, want %q", got, want)
	}
}
