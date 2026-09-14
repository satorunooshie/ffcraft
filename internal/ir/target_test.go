package ir_test

import (
	"bytes"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/flagd"
	"github.com/satorunooshie/ffcraft/internal/gofeatureflag"
)

func TestExtensionsDoNotChangeCoreTargetOutput(t *testing.T) {
	base := minimalIR()
	withExtensions := minimalIR()
	withExtensions.Extensions = map[string]*irv1.ExtensionValue{"vendor": {Kind: &irv1.ExtensionValue_ObjectValue{ObjectValue: &irv1.ExtensionObject{Fields: map[string]*irv1.ExtensionValue{"enabled": {Kind: &irv1.ExtensionValue_BoolValue{BoolValue: true}}}}}}}
	flagdBase, _, err := flagd.CompileIR(base, "prod", flagd.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	flagdWith, _, err := flagd.CompileIR(withExtensions, "prod", flagd.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(flagdBase, flagdWith) {
		t.Fatal("flagd output changed when extensions were added")
	}
	goffBase, _, err := gofeatureflag.CompileIR(base, "prod", gofeatureflag.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	goffWith, _, err := gofeatureflag.CompileIR(withExtensions, "prod", gofeatureflag.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(goffBase, goffWith) {
		t.Fatal("GO Feature Flag output changed when extensions were added")
	}
}
