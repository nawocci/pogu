package control

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nawocci/pogu/internal/service"
	"github.com/nawocci/pogu/internal/store"
)

func (s *Server) execute(ctx context.Context, r Request) (any, error) {
	if s.Service == nil {
		return nil, errors.New("service unavailable")
	}
	switch r.Op {
	case "status":
		ps, err := s.Service.ListProviders(ctx)
		if err != nil {
			return nil, err
		}
		ms, err := s.Service.ListModels(ctx)
		if err != nil {
			return nil, err
		}
		ks, err := s.Service.ListAPIKeys(ctx)
		if err != nil {
			return nil, err
		}
		return Status{Providers: len(ps), Models: len(ms), Keys: len(ks)}, nil
	case "provider.list":
		return s.Service.ListProviders(ctx)
	case "provider.get":
		return s.Service.GetProvider(ctx, r.ID)
	case "provider.create":
		enabled := true
		if r.Enabled != nil {
			enabled = *r.Enabled
		}
		return s.Service.CreateProvider(ctx, r.Name, r.Type, r.Prefix, r.BaseURL, valueOrEmpty(r.APIKey), enabled, r.KeySelection)
	case "provider.update":
		enabled := true
		if r.Enabled != nil {
			enabled = *r.Enabled
		}
		return s.Service.UpdateProvider(ctx, r.ID, r.Name, r.Type, r.Prefix, r.BaseURL, enabled, r.KeySelection)
	case "provider.delete":
		return nil, s.Service.DeleteProvider(ctx, r.ID)
	case "provider.test":
		if s.tester == nil {
			return nil, errors.New("provider connectivity test unavailable")
		}
		p, err := s.Service.GetProvider(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		secret, err := s.Service.ProviderPrimarySecret(ctx, p.ID)
		if err != nil {
			return ProviderTestResult{OK: false, Message: "no enabled provider credentials"}, nil
		}
		if err := s.tester(ctx, p, secret); err != nil {
			return ProviderTestResult{OK: false, Message: "provider connection failed"}, nil
		}
		return ProviderTestResult{OK: true, Message: "connection successful"}, nil
	case "provider_key.list":
		return s.Service.ListProviderKeys(ctx, r.ProviderID)
	case "provider_key.get":
		return s.Service.GetProviderKey(ctx, r.ID)
	case "provider_key.create":
		sec := valueOrEmpty(r.Secret)
		if sec == "" && r.APIKey != nil {
			sec = *r.APIKey
		}
		return s.Service.CreateProviderKey(ctx, r.ProviderID, r.Name, sec)
	case "provider_key.update":
		enabled := true
		if r.Enabled != nil {
			enabled = *r.Enabled
		}
		return s.Service.UpdateProviderKey(ctx, r.ID, r.Name, enabled)
	case "provider_key.delete":
		return nil, s.Service.DeleteProviderKey(ctx, r.ID)
	case "provider_key.primary":
		return nil, s.Service.SetProviderKeyPrimary(ctx, r.ProviderID, r.ID)
	case "provider_key.test":
		if s.tester == nil {
			return nil, errors.New("provider connectivity test unavailable")
		}
		key, err := s.Service.GetProviderKey(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		p, err := s.Service.GetProvider(ctx, key.ProviderID)
		if err != nil {
			return nil, err
		}
		secret, err := s.Service.ProviderKeySecret(ctx, r.ID)
		if err != nil {
			return nil, errors.New("provider credential unavailable")
		}
		if err := s.tester(ctx, p, secret); err != nil {
			return ProviderTestResult{OK: false, Message: "provider connection failed"}, nil
		}
		return ProviderTestResult{OK: true, Message: "connection successful"}, nil
	case "model.list":
		return s.Service.ListModels(ctx)
	case "model.get":
		return s.Service.GetModel(ctx, r.ID)
	case "model.create":
		enabled := true
		if r.Enabled != nil {
			enabled = *r.Enabled
		}
		return s.Service.CreateModel(ctx, r.ProviderID, r.Name, enabled)
	case "model.update":
		enabled := true
		if r.Enabled != nil {
			enabled = *r.Enabled
		}
		return s.Service.UpdateModel(ctx, r.ID, r.Name, enabled)
	case "model.delete":
		return nil, s.Service.DeleteModel(ctx, r.ID)
	case "key.list":
		return s.Service.ListAPIKeys(ctx)
	case "key.create":
		key, secret, err := s.Service.CreateAPIKey(ctx, r.Name)
		if err != nil {
			return nil, err
		}
		return KeyCreateResult{Key: key, Secret: secret}, nil
	case "key.revoke":
		return nil, s.Service.RevokeAPIKey(ctx, r.ID)
	case "telemetry.list":
		if s.Telemetry == nil {
			return nil, errors.New("telemetry unavailable")
		}
		return s.Telemetry.List(ctx, TelemetryLimit)
	default:
		return nil, fmt.Errorf("unknown operation: %s", r.Op)
	}
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func failure(code, message string) Response {
	return Response{OK: false, Error: &Error{Code: code, Message: message}}
}

func failureFor(err error) Response {
	if err == nil {
		return Response{OK: true}
	}
	code, message := "internal", "internal control-plane error"
	switch {
	case errors.Is(err, service.ErrValidation):
		code, message = "validation", err.Error()
	case errors.Is(err, service.ErrBuiltin):
		code, message = "builtin", err.Error()
	case errors.Is(err, service.ErrAlreadyExists):
		code, message = "already_exists", err.Error()
	case errors.Is(err, store.ErrNotFound):
		code, message = "not_found", "resource not found"
	case errors.Is(err, service.ErrUnauthorized):
		code, message = "unauthorized", "unauthorized"
	case errors.Is(err, service.ErrForbidden):
		code, message = "forbidden", "forbidden"
	case strings.HasPrefix(err.Error(), "unknown operation:"):
		code, message = "unknown_operation", "unknown operation"
	case strings.Contains(err.Error(), "unavailable"):
		code, message = "unavailable", err.Error()
	}
	return failure(code, message)
}
