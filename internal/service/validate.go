package service

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ErrValidation    = errors.New("validation error")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrForbidden     = errors.New("forbidden")
	ErrUnknownRoute  = errors.New("unknown model or group")
	ErrAlreadyExists = errors.New("already exists")
	ErrNoCredentials = errors.New("no enabled provider credentials")
	ErrBuiltin       = errors.New("built-in provider cannot be changed this way")
)

var (
	prefixPattern  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,30}[a-z0-9])?$`)
	modelPattern   = regexp.MustCompile(`^[^/\\\x00-\x20]+$`)
	keyNamePattern = regexp.MustCompile(`^Key\s+(\d+)$`)
)

const reservedPrefix = "oc"

func validation(msg string) error {
	return &ValidationError{msg}
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string        { return e.Message }
func (e *ValidationError) Is(target error) bool { return target == ErrValidation }

func checkName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 200 {
		return "", validation("name must be between 1 and 200 characters")
	}
	return name, nil
}

func checkPrefix(prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if !prefixPattern.MatchString(prefix) {
		return "", validation("prefix must be 1-32 lowercase letters, digits, or hyphens without leading or trailing hyphens")
	}
	if prefix == reservedPrefix {
		return "", validation("prefix is reserved")
	}
	return prefix, nil
}

func checkProviderType(t ProviderType) error {
	if !t.Valid() {
		return validation("provider type must be openai or anthropic")
	}
	return nil
}

func checkBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(raw), "/"))
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return "", validation("base URL must be an absolute http(s) URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", validation("base URL must be an absolute http(s) URL")
	}
	if u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", validation("base URL must be an absolute http(s) URL without user, query, or fragment")
	}
	return raw, nil
}

func checkModelName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || !modelPattern.MatchString(name) || utf8.RuneCountInString(name) > 300 {
		return "", validation("model name must be 1-300 characters without slashes or whitespace")
	}
	return name, nil
}

func checkSelection(s KeySelection) (KeySelection, error) {
	if s == "" {
		return SelectionFirst, nil
	}
	if !s.Valid() {
		return "", validation(`key selection must be "first" or "round_robin"`)
	}
	if s == SelectionRound {
		return "", validation(`round_robin key selection is not available yet`)
	}
	return s, nil
}

func checkScheme(s string) (string, error) {
	if !Scheme(s).Valid() {
		return "", validation(`scheme must be "", "openai", or "anthropic"`)
	}
	return s, nil
}
