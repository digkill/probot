package config

import "testing"

func TestTelegramConfigOptionalAndValidated(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("TELEGRAM_CLIENT_ENABLED", "false")
	cfg, err := Load()
	if err != nil || cfg.TelegramClient.Enabled {
		t.Fatalf("disabled config: %v", err)
	}
	t.Setenv("TELEGRAM_CLIENT_ENABLED", "true")
	t.Setenv("TELEGRAM_APP_ID", "100")
	t.Setenv("TELEGRAM_APP_HASH", "test-only-hash")
	t.Setenv("ENCRYPTION_KEY", "test-only-key-012345678901234567")
	cfg, err = Load()
	if err != nil || cfg.TelegramClient.RPCRate != 1 {
		t.Fatalf("enabled config: %v", err)
	}
	for _, key := range []string{"TELEGRAM_APP_ID", "TELEGRAM_RPC_RATE", "TELEGRAM_MAX_ACCOUNTS", "TELEGRAM_CLIENT_ENABLED"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "invalid-secret-value")
			if _, err := Load(); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
	t.Setenv("TELEGRAM_RPC_RATE", "NaN")
	if _, err := Load(); err == nil {
		t.Fatal("NaN accepted")
	}
}
