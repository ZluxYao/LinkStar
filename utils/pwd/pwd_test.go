package pwd

import (
	"errors"
	"testing"
)

func TestArgon2ConcurrencyLimit(t *testing.T) {
	for i := 0; i < argon2MaxConcurrent; i++ {
		if !tryAcquireArgon2() {
			t.Fatalf("slot %d should be available", i+1)
		}
	}
	defer func() {
		for i := 0; i < argon2MaxConcurrent; i++ {
			releaseArgon2()
		}
	}()

	if tryAcquireArgon2() {
		releaseArgon2()
		t.Fatal("an Argon2 operation above the concurrency limit was accepted")
	}
	if _, err := hashArgon2("test-password"); !errors.Is(err, ErrArgon2Busy) {
		t.Fatalf("hashArgon2() error = %v, want ErrArgon2Busy", err)
	}

	releaseArgon2()
	if !tryAcquireArgon2() {
		t.Fatal("released Argon2 slot was not reusable")
	}
}
