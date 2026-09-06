package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const keyPrefix = "v1:"

func LoadOrCreateKey(path string) ([]byte, error) {
	if key, err := readKey(path); err == nil {
		return key, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	fresh := make([]byte, 32)
	if _, err := rand.Read(fresh); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".master-key-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	_ = tmp.Chmod(0o600)
	if _, err := tmp.Write(fresh); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return nil, err
	}
	if err := os.Link(tmpName, path); err != nil {
		if !os.IsExist(err) {
			_ = os.Remove(tmpName)
			return nil, err
		}
		_ = os.Remove(tmpName)
		return readKey(path)
	}
	_ = os.Remove(tmpName)
	return fresh, nil
}

func LoadKey(path string) ([]byte, error) {
	return readKey(path)
}

func readKey(path string) ([]byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errors.New("master key must be a regular file")
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("master key file permissions must not be accessible by group or others")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) != 32 {
		return nil, errors.New("invalid master key length")
	}
	return raw, nil
}

func Encrypt(master, plaintext []byte) (string, error) {
	if len(master) != 32 {
		return "", errors.New("invalid master key")
	}
	block, err := aes.NewCipher(master)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return keyPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func Decrypt(master []byte, sealed string) ([]byte, error) {
	if len(master) != 32 {
		return nil, errors.New("invalid master key")
	}
	if len(sealed) < len(keyPrefix) || subtle.ConstantTimeCompare([]byte(sealed[:len(keyPrefix)]), []byte(keyPrefix)) != 1 {
		return nil, errors.New("decrypt ciphertext")
	}
	raw, err := base64.RawURLEncoding.DecodeString(sealed[len(keyPrefix):])
	if err != nil {
		return nil, errors.New("decrypt ciphertext")
	}
	block, err := aes.NewCipher(master)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, errors.New("decrypt ciphertext")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return nil, errors.New("decrypt ciphertext")
	}
	return plain, nil
}

func KeyFingerprint(master []byte, secret string) []byte {
	mac := hmac.New(sha256.New, master)
	mac.Write([]byte("pogu:provider_key:" + secret))
	return mac.Sum(nil)
}

func MaskSecret(secret string) string {
	s := strings.TrimSpace(secret)
	switch {
	case s == "":
		return ""
	case len(s) <= 4:
		return "****"
	case len(s) <= 8:
		return "..." + s[len(s)-4:]
	case strings.HasPrefix(s, "sk-") && len(s) >= 10:
		return "sk-..." + s[len(s)-4:]
	case len(s) > 12:
		return s[:3] + "..." + s[len(s)-4:]
	default:
		return "..." + s[len(s)-4:]
	}
}
