package integrity

import "testing"

func TestSignRoundTrip(t *testing.T) {
	key := []byte("dev-key")
	sig := Sign(key, "run", "1", "100", "hello")
	if !Verify(key, "run", "1", "100", "hello", sig) {
		t.Fatal("expected verify to pass")
	}
	if Verify(key, "run", "1", "100", "hello", sig[:len(sig)-1]+"0") {
		t.Fatal("expected a flipped signature to fail")
	}
	if Verify([]byte("other"), "run", "1", "100", "hello", sig) {
		t.Fatal("expected a different key to fail")
	}
}
