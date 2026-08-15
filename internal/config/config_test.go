package config

import (
	"testing"
	"time"
)

func TestGetdurRejectsZeroAndNegative(t *testing.T) {
	for _, v := range []string{"0", "-5m", "invalid"} {
		t.Setenv("CLOAK_TEST_DUR", v)
		if got := getdur("CLOAK_TEST_DUR", time.Minute); got != time.Minute {
			t.Fatalf("getdur(%q) = %s, want default 1m", v, got)
		}
	}
	t.Setenv("CLOAK_TEST_DUR", "30s")
	if got := getdur("CLOAK_TEST_DUR", time.Minute); got != 30*time.Second {
		t.Fatalf("getdur(30s) = %s", got)
	}
}
