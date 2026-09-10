package ratelimit

import (
	"testing"
	"time"
)

func TestAllowsUpToLimitThenBlocks(t *testing.T) {
	l := New(3, time.Minute)
	for i := 1; i <= 3; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("request %d should have been allowed", i)
		}
	}
	if l.Allow("1.2.3.4") {
		t.Error("the 4th request in the window should be blocked")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l := New(1, time.Minute)
	if !l.Allow("a") || !l.Allow("b") {
		t.Error("one key's budget must not consume another's")
	}
	if l.Allow("a") {
		t.Error("key a should be exhausted")
	}
}

func TestWindowResets(t *testing.T) {
	l := New(1, 20*time.Millisecond)
	if !l.Allow("k") {
		t.Fatal("first request should be allowed")
	}
	if l.Allow("k") {
		t.Fatal("second request in-window should be blocked")
	}
	time.Sleep(30 * time.Millisecond)
	if !l.Allow("k") {
		t.Error("a new window should allow again")
	}
}

func TestEmptyKeyAlwaysAllowed(t *testing.T) {
	l := New(1, time.Minute)
	for i := 0; i < 5; i++ {
		if !l.Allow("") {
			t.Fatal("an unidentifiable client must not be blocked outright")
		}
	}
}
