package hooks

import (
	"context"
	"fmt"

	"github.com/open-feature/go-sdk/openfeature"
)

type LoggingHook struct{}

func (LoggingHook) Before(_ context.Context, hookContext openfeature.HookContext, _ openfeature.HookHints) (*openfeature.EvaluationContext, error) {
	fmt.Printf("hook before: %s\n", hookContext.FlagKey())
	return nil, nil
}

func (LoggingHook) After(_ context.Context, hookContext openfeature.HookContext, _ openfeature.InterfaceEvaluationDetails, _ openfeature.HookHints) error {
	fmt.Printf("hook after: %s\n", hookContext.FlagKey())
	return nil
}

func (LoggingHook) Error(_ context.Context, hookContext openfeature.HookContext, err error, _ openfeature.HookHints) {
	fmt.Printf("hook error: %s: %v\n", hookContext.FlagKey(), err)
}

func (LoggingHook) Finally(_ context.Context, hookContext openfeature.HookContext, _ openfeature.InterfaceEvaluationDetails, _ openfeature.HookHints) {
	fmt.Printf("hook finally: %s\n", hookContext.FlagKey())
}
