package gordma

import "testing"

// TestSupportedMatchesPlatform verifies that the build-tag-selected supported
// flag agrees with Supported(). On non-Linux/non-cgo builds (such as CI on
// macOS) it must report false; on Linux+cgo it must report true.
func TestSupportedMatchesPlatform(t *testing.T) {
	if Supported() != supported {
		t.Fatalf("Supported() = %v, want %v", Supported(), supported)
	}
}

// TestErrNotSupportedMessage ensures the sentinel error is non-nil and carries
// an identifiable message, since callers on unsupported platforms match on it.
func TestErrNotSupported(t *testing.T) {
	if ErrNotSupported == nil {
		t.Fatal("ErrNotSupported must not be nil")
	}
	if ErrNotSupported.Error() == "" {
		t.Fatal("ErrNotSupported must have a message")
	}
}
