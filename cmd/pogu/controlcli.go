package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nawocci/pogu/internal/config"
	"github.com/nawocci/pogu/internal/control"
	"github.com/nawocci/pogu/internal/service"
	"github.com/nawocci/pogu/internal/telemetry"
)

func controlClient(dataDir string) (*control.Client, error) {
	cfg, err := config.Load(dataDir)
	if err != nil {
		return nil, err
	}
	return control.NewClient(cfg.ControlSock), nil
}

func controlStatus(dataDir string) error {
	client, err := controlClient(dataDir)
	if err != nil {
		return err
	}
	var result control.Status
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Call(ctx, control.Request{Op: "status"}, &result); err != nil {
		return fmt.Errorf("connect to pogu daemon: %w", err)
	}
	return printJSON(result)
}

func controlResource(dataDir, resource string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%s: operation is required", resource)
	}
	client, err := controlClient(dataDir)
	if err != nil {
		return err
	}
	req, visited, err := parseControlRequest(resource, args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := fillUpdateDefaults(ctx, client, resource, visited, &req); err != nil {
		return err
	}
	var result any
	if err := client.Call(ctx, req, &result); err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	return printJSON(result)
}

// fillUpdateDefaults makes CLI updates partial: omitted flags inherit stored
// values instead of blanking fields (which also tripped the built-in freeze).
func fillUpdateDefaults(ctx context.Context, client *control.Client, resource string, visited map[string]bool, req *control.Request) error {
	if !strings.HasSuffix(req.Op, ".update") {
		return nil
	}
	switch resource {
	case "provider":
		var current service.Provider
		if err := client.Call(ctx, control.Request{Op: "provider.get", ID: req.ID}, &current); err != nil {
			return err
		}
		if !visited["name"] {
			req.Name = current.Name
		}
		if !visited["type"] {
			req.Type = current.Type
		}
		if !visited["prefix"] {
			req.Prefix = current.Prefix
		}
		if !visited["base-url"] {
			req.BaseURL = current.BaseURL
		}
		if !visited["key-selection"] {
			req.KeySelection = current.KeySelection
		}
	case "model":
		var current service.Model
		if err := client.Call(ctx, control.Request{Op: "model.get", ID: req.ID}, &current); err != nil {
			return err
		}
		if !visited["name"] {
			req.Name = current.Name
		}
	}
	return nil
}

func controlProviderKey(dataDir string, args []string) error {
	if len(args) == 0 {
		return errors.New("provider key: operation is required (list, create, get, update, enable, disable, primary, delete, test, import)")
	}
	op := args[0]
	client, err := controlClient(dataDir)
	if err != nil {
		return err
	}
	fs := newFlagSet("pogu provider key " + op)
	id := int64(0)
	providerID := int64(0)
	name := ""
	secret := ""
	file := ""
	preview := false
	enabled := true
	enabledSet := false

	fs.Int64Var(&id, "id", 0, "key id")
	fs.Int64Var(&providerID, "provider-id", 0, "provider id")
	fs.StringVar(&name, "name", "", "key name")
	fs.StringVar(&secret, "secret", "", "provider API key secret")
	fs.StringVar(&file, "file", "", "input file for batch import")
	fs.BoolVar(&preview, "preview", false, "preview import without committing")
	fs.BoolFunc("enabled", "enable or disable the key", func(value string) error {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		enabled, enabledSet = parsed, true
		return nil
	})

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	enabledPtr := (*bool)(nil)
	if enabledSet {
		enabledPtr = &enabled
	}
	if op == "enable" {
		t := true
		enabledPtr = &t
		op = "update"
	} else if op == "disable" {
		f := false
		enabledPtr = &f
		op = "update"
	}

	req := control.Request{
		Op:         "provider_key." + op,
		ID:         id,
		ProviderID: providerID,
		Name:       name,
		Enabled:    enabledPtr,
	}
	if secret != "" {
		req.Secret = &secret
	}
	if op == "update" && name == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		var current service.ProviderKey
		err := client.Call(ctx, control.Request{Op: "provider_key.get", ID: id}, &current)
		cancel()
		if err != nil {
			return err
		}
		req.Name = current.Name
	}

	if op == "import" {
		var textBytes []byte
		if file != "" {
			textBytes, err = os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("read import file: %w", err)
			}
		} else {
			textBytes, err = io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read import from stdin: %w", err)
			}
		}
		req.Text = string(textBytes)
		req.Preview = preview
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var result any
	if err := client.Call(ctx, req, &result); err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	return printJSON(result)
}

func parseControlRequest(resource string, args []string) (control.Request, map[string]bool, error) {
	op := args[0]
	if op == "list" {
		return control.Request{Op: resource + ".list"}, map[string]bool{}, nil
	}
	fs := newFlagSet("pogu " + resource + " " + op)
	id := int64(0)
	name, typ, prefix, baseURL, apiKey := "", service.ProviderOpenAI, "", "", ""
	keySelection := service.KeySelectionFirst
	keySelectionSet := false
	providerID := int64(0)
	enabled := true
	enabledSet := false
	fs.Int64Var(&id, "id", 0, "resource id")
	fs.StringVar(&name, "name", "", "name")
	fs.Var(providerTypeValue{value: &typ}, "type", "provider protocol (openai, openai-responses, or anthropic)")
	fs.StringVar(&prefix, "prefix", "", "provider prefix")
	fs.StringVar(&baseURL, "base-url", "", "provider base URL")
	fs.StringVar(&apiKey, "api-key", "", "provider API key")
	fs.Var(keySelectionValue{value: &keySelection, set: &keySelectionSet}, "key-selection", "key selection mode (first or round_robin)")
	fs.Int64Var(&providerID, "provider-id", 0, "provider id")
	fs.BoolFunc("enabled", "enable or disable the resource", func(value string) error {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		enabled, enabledSet = parsed, true
		return nil
	})
	if err := fs.Parse(args[1:]); err != nil {
		return control.Request{}, nil, err
	}
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	enabledPtr := (*bool)(nil)
	if enabledSet {
		enabledPtr = &enabled
	}
	apiKeyPtr := (*string)(nil)
	if fs.Lookup("api-key") != nil && (apiKey != "" || op == "create") {
		apiKeyPtr = &apiKey
	}
	req := control.Request{
		Op:           resource + "." + op,
		ID:           id,
		Name:         name,
		Type:         typ,
		Prefix:       prefix,
		BaseURL:      baseURL,
		KeySelection: keySelection,
		APIKey:       apiKeyPtr,
		ProviderID:   providerID,
		Enabled:      enabledPtr,
	}
	if op == "create" && resource == "provider" && apiKeyPtr == nil {
		req.APIKey = &apiKey
	}
	return req, visited, nil
}

func telemetryCommand(dataDir string, args []string) error {
	if len(args) == 0 || args[0] != "list" {
		return errors.New("telemetry: operation must be \"list\"")
	}
	args = args[1:]
	fs := newFlagSet("pogu telemetry list")
	limit := control.TelemetryLimit
	fs.IntVar(&limit, "limit", control.TelemetryLimit, "maximum number of requests to show")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if limit < 1 || limit > 1000 {
		return errors.New("--limit must be between 1 and 1000")
	}
	client, err := controlClient(dataDir)
	if err != nil {
		return err
	}
	var result []telemetry.RequestWithAttempts
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := client.Do(ctx, control.Request{Op: "telemetry.list"})
	if err != nil {
		return err
	}
	if len(resp.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return err
	}
	if limit < len(result) {
		result = result[:limit]
	}
	return printJSON(result)
}

type providerTypeValue struct{ value *service.ProviderType }

func (v providerTypeValue) String() string {
	if v.value == nil {
		return ""
	}
	return string(*v.value)
}

func (v providerTypeValue) Set(value string) error {
	typ := service.ProviderType(value)
	if !typ.Valid() {
		return errors.New("type must be openai, openai-responses, or anthropic")
	}
	*v.value = typ
	return nil
}

type keySelectionValue struct {
	value *service.KeySelection
	set   *bool
}

func (v keySelectionValue) String() string {
	if v.value == nil {
		return ""
	}
	return string(*v.value)
}

func (v keySelectionValue) Set(value string) error {
	ks := service.KeySelection(value)
	if !ks.Valid() {
		return errors.New("key-selection must be 'first' or 'round_robin'")
	}
	*v.value = ks
	if v.set != nil {
		*v.set = true
	}
	return nil
}

func printJSON(value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Println(string(encoded))
	return err
}
