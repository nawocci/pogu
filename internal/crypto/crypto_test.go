package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := LoadOrCreateKey(t.TempDir() + "/master.key")
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Fatalf("key length = %d", len(key))
	}
	sealed, err := Encrypt(key, []byte("sk-secret-123"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Decrypt(key, sealed)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "sk-secret-123" {
		t.Fatalf("round trip = %q", plain)
	}
	if _, err := Decrypt(key, sealed+"x"); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
	other, _ := LoadOrCreateKey(t.TempDir() + "/master.key")
	if _, err := Decrypt(other, sealed); err == nil {
		t.Fatal("wrong key must fail")
	}
}

func TestLoadOrCreateKeyPersists(t *testing.T) {
	path := t.TempDir() + "/master.key"
	a, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("key not stable across loads")
	}
}

func TestKeyFingerprint(t *testing.T) {
	master := bytes.Repeat([]byte{1}, 32)
	other := bytes.Repeat([]byte{2}, 32)
	a := KeyFingerprint(master, "secret")
	b := KeyFingerprint(master, "secret")
	c := KeyFingerprint(master, "other")
	d := KeyFingerprint(other, "secret")
	if !bytes.Equal(a, b) || bytes.Equal(a, c) || bytes.Equal(a, d) {
		t.Fatal("fingerprint must be deterministic per (key, secret)")
	}
}

func TestMaskSecret(t *testing.T) {
	cases := map[string]string{
		"":                 "",
		"abcd":             "****",
		"abcdefgh":         "...efgh",
		"sk-12345678":      "sk-...5678",
		"ver1234567890123": "ver...0123",
		"123456789":        "...6789",
	}
	for in, want := range cases {
		if got := MaskSecret(in); got != want {
			t.Errorf("MaskSecret(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPasswordHashVerify(t *testing.T) {
	hash, err := HashPassword("0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword("0123456789abcdef", hash) {
		t.Fatal("valid password rejected")
	}
	if CheckPassword("wrong password!!", hash) {
		t.Fatal("invalid password accepted")
	}
	if CheckPassword("x", "garbage") {
		t.Fatal("garbage hash accepted")
	}
	if _, err := HashPassword(""); err == nil {
		t.Fatal("empty password must fail")
	}
}
