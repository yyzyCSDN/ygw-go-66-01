package store

import (
	"testing"
)

func TestDirLockAcquireRelease(t *testing.T) {
	root := t.TempDir()
	lock, err := AcquireDirLock(root)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, err := AcquireDirLock(root); err == nil {
		t.Fatal("second acquire should fail while held")
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	again, err := AcquireDirLock(root)
	if err != nil {
		t.Fatalf("reacquire: %v", err)
	}
	_ = again.Release()
}

func TestDirLockReleaseIdempotent(t *testing.T) {
	lock, err := AcquireDirLock(t.TempDir())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("double release: %v", err)
	}
}
