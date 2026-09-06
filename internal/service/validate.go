package service

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ErrValidation     = errors.New("validation error")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrForbidden      = errors.New("forbidden")
	ErrUnknownRoute   = errors.New("unknown model or group")
	ErrAlreadyExists  = errors.New("already exists")
	ErrNoCredentials  = errors.New("no enabled provider credentials")
	ErrNoGroupTargets = errors.New("group has no available targets")
	ErrBuiltin        = errors.New("built-in provider cannot be changed this way")
)

var (
	prefixPattern  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,30}[a-z0-9])?$`)
	modelPattern   = regexp.MustCompile(`^[^/\\\x00-\x20]+$`)
	keyNamePattern = regexp.MustCompile(`^Key\s+(\d+)$`)
)

const reservedPrefix = "oc"

func IsReservedPrefix(prefix string) bool { return prefix == reservedPrefix }

func validateProviderInput(name string, typ ProviderType, prefix, baseURL string) error {
	if err := validateProviderNameTypeURL(name, typ, baseURL); err != nil {
		return err
	}
	if !prefixPattern.MatchString(prefix) {
		return fmt.Errorf("%w: prefix must use lowercase letters, numbers, and hyphens", ErrValidation)
	}
	if IsReservedPrefix(prefix) {
		return fmt.Errorf("%w: prefix %q is reserved for the built-in provider", ErrValidation, prefix)
	}
	return nil
}

func validateProviderNameTypeURL(name string, typ ProviderType, baseURL string) error {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(strings.TrimSpace(name)) > 200 {
		return fmt.Errorf("%w: name is required and must be at most 200 characters", ErrValidation)
	}
	if !typ.Valid() {
		return fmt.Errorf("%w: type must be openai or anthropic", ErrValidation)
	}
	u, err := url.ParseRequestURI(strings.TrimSpace(baseURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%w: base_url must be an absolute http(s) URL without credentials or query parameters", ErrValidation)
	}
	return nil
}

func validateModelName(name string) error {
	if !modelPattern.MatchString(name) || utf8.RuneCountInString(name) > 300 {
		return fmt.Errorf("%w: model name must be 1-300 non-whitespace characters without slash", ErrValidation)
	}
	return nil
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}
