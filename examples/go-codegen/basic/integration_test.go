package basic_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"buf.build/gen/go/open-feature/flagd/connectrpc/go/flagd/evaluation/v2/evaluationv2connect"
	evaluation "buf.build/gen/go/open-feature/flagd/protocolbuffers/go/flagd/evaluation/v2"
	"connectrpc.com/connect"
	"github.com/fsnotify/fsnotify"
	flagdservice "github.com/open-feature/flagd/core/pkg/service"
	flagsync "github.com/open-feature/flagd/core/pkg/sync"
	flagd "github.com/open-feature/go-sdk-contrib/providers/flagd/pkg"
	gofeatureflag "github.com/open-feature/go-sdk-contrib/providers/go-feature-flag/pkg"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/satorunooshie/backoff"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/satorunooshie/ffcraft/examples/go-codegen/basic/adapter"
	featureflags "github.com/satorunooshie/ffcraft/examples/go-codegen/basic/gen"
)

type inProcessSync struct {
	initial string
	updates chan string
	fail    atomic.Bool
}

func (s *inProcessSync) Init(context.Context) error { return nil }
func (s *inProcessSync) IsReady() bool              { return true }
func (s *inProcessSync) Sync(ctx context.Context, out chan<- flagsync.DataSync) error {
	if s.fail.CompareAndSwap(true, false) {
		return errors.New("in-process initial sync failure")
	}
	select {
	case out <- flagsync.DataSync{FlagData: s.initial, Source: "test://in-process"}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case update := <-s.updates:
		select {
		case out <- flagsync.DataSync{FlagData: update, Source: "test://in-process"}:
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (s *inProcessSync) ReSync(ctx context.Context, out chan<- flagsync.DataSync) error {
	return s.Sync(ctx, out)
}

var _ flagsync.ISync = (*inProcessSync)(nil)

// Evidence: B8-FLAGD-INPROCESS-HOTRELOAD-001.
func TestB8FlagdInProcessHotReload001(t *testing.T) {
	initial, err := json.Marshal(flagdConfig(false))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := json.Marshal(flagdConfig(true))
	if err != nil {
		t.Fatal(err)
	}
	syncer := &inProcessSync{initial: string(initial), updates: make(chan string, 1)}
	provider, err := flagd.NewProvider(
		flagd.WithInProcessResolver(),
		flagd.WithCustomSyncProvider(syncer),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := openfeature.SetProviderAndWait(provider); err != nil {
		t.Fatal(err)
	}
	defer openfeature.Shutdown()

	evaluator := featureflags.New(adapter.NewClientAdapter(openfeature.NewDefaultClient()))
	ctx := context.Background()
	if got, err := evaluator.EnableNewHome(ctx, featureflags.EvalContext{DevicePlatform: "ios"}); err != nil || got {
		t.Fatalf("initial in-process evaluation = %v, %v; want false, nil", got, err)
	}
	syncer.updates <- string(updated)
	waitFor(t, 5*time.Second, func() bool {
		got, err := evaluator.EnableNewHome(ctx, featureflags.EvalContext{DevicePlatform: "ios"})
		return err == nil && got
	})
	shutdownWithin(t)
}

// Evidence: B8-FLAGD-INPROCESS-INITFAIL-RECOVER-001,
// B8-FLAGD-INPROCESS-INITFAIL-RECOVER-UPDATE-001,
// B8-FLAGD-INPROCESS-SHUTDOWN-AFTER-RECOVERY-001.
func TestB8FlagdInProcessInitialFailureRecovery001(t *testing.T) {
	initial, err := json.Marshal(flagdConfig(false))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := json.Marshal(flagdConfig(true))
	if err != nil {
		t.Fatal(err)
	}
	syncer := &inProcessSync{initial: string(initial), updates: make(chan string, 1)}
	syncer.fail.Store(true)
	provider, err := flagd.NewProvider(flagd.WithInProcessResolver(), flagd.WithCustomSyncProvider(syncer))
	if err != nil {
		t.Fatal(err)
	}
	if err := openfeature.SetProviderAndWait(provider); err == nil {
		t.Fatal("expected in-process initial sync failure")
	}
	recoveredProvider, err := flagd.NewProvider(flagd.WithInProcessResolver(), flagd.WithCustomSyncProvider(syncer))
	if err != nil {
		t.Fatal(err)
	}
	if err := openfeature.SetProviderAndWait(recoveredProvider); err != nil {
		t.Fatalf("in-process provider did not recover: %v", err)
	}
	evaluator := featureflags.New(adapter.NewClientAdapter(openfeature.NewDefaultClient()))
	ctx := context.Background()
	if got, err := evaluator.EnableNewHome(ctx, featureflags.EvalContext{DevicePlatform: "ios"}); err != nil || got {
		t.Fatalf("recovered in-process evaluation = %v, %v; want false, nil", got, err)
	}
	syncer.updates <- string(updated)
	waitFor(t, 5*time.Second, func() bool {
		got, err := evaluator.EnableNewHome(ctx, featureflags.EvalContext{DevicePlatform: "ios"})
		return err == nil && got
	})
	shutdownWithin(t)
}

// Evidence: B8-FLAGD-FILE-HOTRELOAD-001.
func TestB8FlagdFileHotReloadAndB9GeneratedAccessor(t *testing.T) {
	initial := flagdConfig(false)
	updated := flagdConfig(true)
	path := filepath.Join(t.TempDir(), "flags.json")
	writeJSON(t, path, initial)

	provider, err := flagd.NewProvider(flagd.WithFileResolver(), flagd.WithOfflineFilePath(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := openfeature.SetProviderAndWait(provider); err != nil {
		t.Fatal(err)
	}
	defer openfeature.Shutdown()

	evaluator := featureflags.New(adapter.NewClientAdapter(openfeature.NewDefaultClient()))
	ctx := context.Background()
	if got, err := evaluator.EnableNewHome(ctx, featureflags.EvalContext{DevicePlatform: "ios"}); err != nil || got {
		t.Fatalf("initial generated evaluation = %v, %v; want false, nil", got, err)
	}

	writeFileAndWaitForWrite(t, path, []byte("{invalid json"))
	if got, err := evaluator.EnableNewHome(ctx, featureflags.EvalContext{DevicePlatform: "ios"}); err != nil || got {
		t.Fatalf("evaluation after invalid update = %v, %v; want previous false, nil", got, err)
	}

	writeJSON(t, path, updated)
	waitFor(t, 5*time.Second, func() bool {
		got, err := evaluator.EnableNewHome(ctx, featureflags.EvalContext{DevicePlatform: "ios"})
		return err == nil && got
	})
	shutdownWithin(t)
}

// Evidence: file-backed recovery behavior is covered here; the in-process
// lifecycle IDs are covered by TestB8FlagdInProcessInitialFailureRecovery001.
func TestB8FlagdFileInitialFailureRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flags.json")
	if err := os.WriteFile(path, []byte("{invalid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider, err := flagd.NewProvider(flagd.WithFileResolver(), flagd.WithOfflineFilePath(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := openfeature.SetProviderAndWait(provider); err == nil {
		t.Fatal("expected initial provider setup to fail")
	}
	writeJSON(t, path, flagdConfig(false))
	if err := openfeature.SetProviderAndWait(provider); err != nil {
		t.Fatalf("provider did not recover after initial failure: %v", err)
	}
	defer openfeature.Shutdown()

	value, err := openfeature.NewDefaultClient().BooleanValue(context.Background(), "enable-new-home", true, openfeature.NewEvaluationContext("ios", map[string]any{}))
	if err != nil || value {
		t.Fatalf("recovered provider evaluation = %v, %v; want false, nil", value, err)
	}
	shutdownWithin(t)
}

func TestB8FlagdObjectListRepresentation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flags.json")
	writeJSON(t, path, map[string]any{
		"$schema": "https://flagd.dev/schema/v0/flags.json",
		"flags": map[string]any{
			"structured": map[string]any{
				"state":          "ENABLED",
				"variants":       map[string]any{"value": map[string]any{"users": []any{"anonymous", "google"}}},
				"defaultVariant": "value",
			},
		},
	})
	provider, err := flagd.NewProvider(flagd.WithFileResolver(), flagd.WithOfflineFilePath(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := openfeature.SetProviderAndWait(provider); err != nil {
		t.Fatal(err)
	}
	defer openfeature.Shutdown()

	value, err := openfeature.NewDefaultClient().ObjectValue(context.Background(), "structured", map[string]any{}, openfeature.NewTargetlessEvaluationContext(nil))
	if err != nil {
		t.Fatal(err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("structured value type = %T, want map[string]any", value)
	}
	users, ok := object["users"].([]any)
	if !ok || len(users) != 2 || users[0] != "anonymous" || users[1] != "google" {
		t.Fatalf("structured users = %#v, want object list", object["users"])
	}
	shutdownWithin(t)
}

// Evidence: B8-GOFF-REMOTE-RELOAD-001, B8-GOFF-SHUTDOWN-001.
func TestB8GoffRemoteAndB9GeneratedAccessor(t *testing.T) {
	var enabled atomic.Bool
	data, err := os.ReadFile("gen/prod.goff.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var flags map[string]any
	if err := yaml.Unmarshal(data, &flags); err != nil {
		t.Fatal(err)
	}
	response, err := json.Marshal(map[string]any{"flags": flags})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ofrep/v1/evaluate/flags/enable-new-home" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(ofrepResponse(enabled.Load()))
			return
		}
		if r.URL.Path == "/v1/flag/configuration" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(response)
			return
		}
		if r.URL.Path == "/v1/data/collector" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	provider, err := gofeatureflag.NewProviderWithContext(context.Background(), gofeatureflag.ProviderOptions{
		Endpoint:              server.URL,
		HTTPClient:            server.Client(),
		EvaluationType:        gofeatureflag.EvaluationTypeRemote,
		DataCollectorDisabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := openfeature.SetProviderAndWait(provider); err != nil {
		t.Fatal(err)
	}
	defer openfeature.Shutdown()

	evaluator := featureflags.New(adapter.NewClientAdapter(openfeature.NewDefaultClient()))
	ctx := context.Background()
	if got, err := evaluator.EnableNewHome(ctx, featureflags.EvalContext{DevicePlatform: "ios"}); err != nil || got {
		t.Fatalf("initial generated GOFF evaluation = %v, %v; want false, nil", got, err)
	}
	enabled.Store(true)
	waitFor(t, 5*time.Second, func() bool {
		got, err := evaluator.EnableNewHome(ctx, featureflags.EvalContext{DevicePlatform: "ios"})
		return err == nil && got
	})
	shutdownWithin(t)
}

// Evidence: B8-GOFF-REMOTE-DISCONNECT-RECONNECT-001.
func TestB8GoffRemoteDisconnectReconnect001(t *testing.T) {
	var enabled atomic.Bool
	var available atomic.Bool
	available.Store(true)
	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ofrep/v1/evaluate/flags/enable-new-home" {
			http.NotFound(w, r)
			return
		}
		if !available.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(ofrepResponse(enabled.Load()))
	}))
	provider, err := gofeatureflag.NewProviderWithContext(context.Background(), gofeatureflag.ProviderOptions{
		Endpoint:              server.URL,
		HTTPClient:            server.Client(),
		EvaluationType:        gofeatureflag.EvaluationTypeRemote,
		DataCollectorDisabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := openfeature.SetProviderAndWait(provider); err != nil {
		t.Fatal(err)
	}
	defer openfeature.Shutdown()

	client := openfeature.NewDefaultClient()
	ctx := openfeature.NewEvaluationContext("ios", map[string]any{})
	if got, err := client.BooleanValue(context.Background(), "enable-new-home", true, ctx); err != nil || got {
		t.Fatalf("initial remote evaluation = %v, %v; want false, nil", got, err)
	}
	available.Store(false)
	if _, err := client.BooleanValue(context.Background(), "enable-new-home", true, ctx); err == nil {
		t.Fatal("expected remote disconnect evaluation to fail")
	}
	available.Store(true)
	enabled.Store(true)
	waitFor(t, 5*time.Second, func() bool {
		got, err := client.BooleanValue(context.Background(), "enable-new-home", false, ctx)
		return err == nil && got
	})
	shutdownWithin(t)
}

// Evidence: B8-GOFF-INPROCESS-HOTRELOAD-001.
func TestB8GoffInProcessHotReloadAndB9GeneratedAccessor(t *testing.T) {
	var enabled atomic.Bool
	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/flag/configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(goffConfig(enabled.Load()))
	}))
	provider, err := gofeatureflag.NewProviderWithContext(context.Background(), gofeatureflag.ProviderOptions{
		Endpoint:                  server.URL,
		HTTPClient:                server.Client(),
		FlagChangePollingInterval: 25 * time.Millisecond,
		DataCollectorDisabled:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := openfeature.SetProviderAndWait(provider); err != nil {
		t.Fatal(err)
	}
	defer openfeature.Shutdown()

	evaluator := featureflags.New(adapter.NewClientAdapter(openfeature.NewDefaultClient()))
	ctx := context.Background()
	if got, err := evaluator.EnableNewHome(ctx, featureflags.EvalContext{DevicePlatform: "ios"}); err != nil || got {
		t.Fatalf("initial GOFF evaluation = %v, %v; want false, nil", got, err)
	}

	enabled.Store(true)
	waitFor(t, 5*time.Second, func() bool {
		got, err := evaluator.EnableNewHome(ctx, featureflags.EvalContext{DevicePlatform: "ios"})
		return err == nil && got
	})
	shutdownWithin(t)
}

// Evidence: B8-GOFF-INPROCESS-RECOVERY-001.
func TestB8GoffInProcessInitialFailureRecovery(t *testing.T) {
	var available atomic.Bool
	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/flag/configuration" {
			http.NotFound(w, r)
			return
		}
		if !available.Load() {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(goffConfig(false))
	}))
	provider, err := gofeatureflag.NewProviderWithContext(context.Background(), gofeatureflag.ProviderOptions{
		Endpoint:                  server.URL,
		HTTPClient:                server.Client(),
		FlagChangePollingInterval: 25 * time.Millisecond,
		DataCollectorDisabled:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := openfeature.SetProviderAndWait(provider); err == nil {
		t.Fatal("expected initial provider setup to fail")
	}
	available.Store(true)
	if err := openfeature.SetProviderAndWait(provider); err != nil {
		t.Fatalf("provider did not recover after initial failure: %v", err)
	}
	defer openfeature.Shutdown()

	value, err := openfeature.NewDefaultClient().BooleanValue(context.Background(), "enable-new-home", true, openfeature.NewEvaluationContext("ios", map[string]any{}))
	if err != nil || value {
		t.Fatalf("recovered provider evaluation = %v, %v; want false, nil", value, err)
	}
	shutdownWithin(t)
}

// Evidence: B8-FLAGD-RPC-DISCONNECT-RECONNECT-001.
func TestB8FlagdRPCDisconnectReconnectAndUpdate(t *testing.T) {
	serverState := &flagdRPCServer{releaseReconnect: make(chan struct{})}
	mountPath, handler := evaluationv2connect.NewServiceHandler(serverState)
	mux := http.NewServeMux()
	mux.Handle(mountPath, handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := flagd.NewProvider(
		flagd.WithRPCResolver(),
		flagd.WithHost(host),
		flagd.WithPort(uint16(port)),
		flagd.WithEventStreamConnectionMaxAttempts(10),
		flagd.WithDeadline(500),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := openfeature.SetProviderAndWait(provider); err != nil {
		t.Fatal(err)
	}
	defer openfeature.Shutdown()

	evaluator := featureflags.New(adapter.NewClientAdapter(openfeature.NewDefaultClient()))
	ctx := context.Background()
	if got, err := evaluator.EnableNewHome(ctx, featureflags.EvalContext{DevicePlatform: "ios"}); err != nil || got {
		t.Fatalf("initial RPC evaluation = %v, %v; want false, nil", got, err)
	}

	waitFor(t, 5*time.Second, func() bool {
		return serverState.streams.Load() >= 2
	})
	serverState.enabled.Store(true)
	close(serverState.releaseReconnect)
	waitFor(t, 5*time.Second, func() bool {
		got, err := evaluator.EnableNewHome(ctx, featureflags.EvalContext{DevicePlatform: "ios"})
		return err == nil && got
	})
	shutdownWithin(t)
}

func flagdConfig(enabled bool) map[string]any {
	variant := "off"
	if enabled {
		variant = "on"
	}
	return map[string]any{
		"$schema": "https://flagd.dev/schema/v0/flags.json",
		"flags": map[string]any{
			"enable-new-home": map[string]any{
				"state":          "ENABLED",
				"variants":       map[string]any{"on": true, "off": false},
				"defaultVariant": variant,
			},
		},
	}
}

func goffConfig(enabled bool) []byte {
	variant := false
	if enabled {
		variant = true
	}
	value, _ := json.Marshal(map[string]any{
		"flags": map[string]any{
			"enable-new-home": map[string]any{
				"variations":  map[string]any{"on": true, "off": false},
				"defaultRule": map[string]any{"variation": map[bool]string{true: "on", false: "off"}[variant]},
				"targeting":   []any{map[string]any{"query": "device.platform eq \"ios\"", "variation": map[bool]string{true: "on", false: "off"}[variant]}},
			},
		},
	})
	return value
}

func ofrepResponse(enabled bool) []byte {
	variant := "off"
	if enabled {
		variant = "on"
	}
	value, _ := json.Marshal(map[string]any{
		"value":   enabled,
		"key":     "enable-new-home",
		"reason":  "STATIC",
		"variant": variant,
	})
	return value
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFileAndWaitForWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = watcher.Close() })
	if err := watcher.Add(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				t.Fatal("file watcher events channel closed before write event")
			}
			if filepath.Clean(event.Name) == filepath.Clean(path) && event.Op&fsnotify.Write != 0 {
				return
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				t.Fatal("file watcher errors channel closed before write event")
			}
			t.Fatalf("file watcher error: %v", err)
		case <-timer.C:
			t.Fatalf("file write event for %q was not observed", path)
		}
	}
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	policy := backoff.Policy{
		Schedule: backoff.Schedule{
			Base:   25 * time.Millisecond,
			Max:    25 * time.Millisecond,
			Factor: 1,
			Jitter: backoff.JitterNone,
		},
		Budget: backoff.Budget{MaxElapsed: timeout},
	}
	_, err := policy.Do(context.Background(), func(context.Context, backoff.Attempt) (struct{}, error) {
		if condition() {
			return struct{}{}, nil
		}
		return struct{}{}, errors.New("condition was not met")
	})
	if err != nil {
		t.Fatalf("condition was not met before timeout: %v", err)
	}
}

type flagdRPCServer struct {
	evaluationv2connect.UnimplementedServiceHandler
	enabled          atomic.Bool
	streams          atomic.Int32
	releaseReconnect chan struct{}
}

func (s *flagdRPCServer) ResolveBoolean(_ context.Context, _ *connect.Request[evaluation.ResolveBooleanRequest]) (*connect.Response[evaluation.ResolveBooleanResponse], error) {
	value := s.enabled.Load()
	variant := "off"
	if value {
		variant = "on"
	}
	return connect.NewResponse(&evaluation.ResolveBooleanResponse{
		Value:   &value,
		Variant: &variant,
		Reason:  "STATIC",
	}), nil
}

func (s *flagdRPCServer) EventStream(ctx context.Context, _ *connect.Request[evaluation.EventStreamRequest], stream *connect.ServerStream[evaluation.EventStreamResponse]) error {
	connection := s.streams.Add(1)
	if err := stream.Send(&evaluation.EventStreamResponse{Type: string(flagdservice.ProviderReady)}); err != nil {
		return err
	}
	if connection == 1 {
		return errors.New("simulated disconnect")
	}
	select {
	case <-s.releaseReconnect:
		data, err := structpb.NewStruct(map[string]any{"flags": map[string]any{"enable-new-home": map[string]any{}}})
		if err != nil {
			return err
		}
		if err := stream.Send(&evaluation.EventStreamResponse{Type: string(flagdservice.ConfigurationChange), Data: data}); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	case <-ctx.Done():
		return nil
	}
}

func shutdownWithin(t *testing.T) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		openfeature.Shutdown()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("provider shutdown did not complete before timeout")
	}
}
