package ir

import (
	"errors"
	"strings"
	"testing"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func TestIRCodecRoundTripPreservesOpaqueExtension(t *testing.T) {
	doc := minimalCodecDocument()
	extension := &irv1.ExtensionValue{}
	extension.ProtoReflect().SetUnknown(unknownVarintField(99))
	doc.Extensions = map[string]*irv1.ExtensionValue{"vendor": extension}
	encoded, err := Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Unmarshal(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Extensions["vendor"].ProtoReflect().GetUnknown(); len(got) == 0 {
		t.Fatal("opaque extension unknown fields were lost")
	}
	if !proto.Equal(doc, decoded) {
		t.Fatal("IR changed across protobuf round trip")
	}
}

func TestIRCodecRejectsUnknownCoreWithPathAndCode(t *testing.T) {
	doc := minimalCodecDocument()
	doc.Flags["f"].ProtoReflect().SetUnknown(unknownVarintField(77))
	if _, err := Marshal(doc); err == nil || !strings.Contains(err.Error(), "FFCRAFT_IR_UNKNOWN_CORE_FIELD") || !strings.Contains(err.Error(), "$.flags[\"f\"]") {
		t.Fatalf("Marshal() error = %v, want code and path", err)
	}
	if _, err := Unmarshal(unknownVarintField(88)); err == nil || !strings.Contains(err.Error(), "FFCRAFT_IR_UNKNOWN_CORE_FIELD") {
		t.Fatalf("Unmarshal() error = %v, want unknown core error", err)
	}
}

func TestCoreValidationErrorContracts(t *testing.T) {
	cause := errors.New("cause")
	errorWithCause := &CoreValidationError{Code: "CODE", Path: "$.x", Cause: cause}
	if !strings.Contains(errorWithCause.Error(), "CODE at $.x: cause") || !errors.Is(errorWithCause, cause) {
		t.Fatalf("CoreValidationError with cause = %v", errorWithCause)
	}
	errorWithoutCause := &CoreValidationError{Code: "CODE", Path: "$.x", FieldNumber: 7, MessageType: "Type"}
	if got := errorWithoutCause.Error(); got != "CODE: unknown field 7 in Type at $.x" {
		t.Fatalf("CoreValidationError without cause = %q", got)
	}
	if got := firstUnknownFieldNumber([]byte{0x80}); got != 0 {
		t.Fatalf("firstUnknownFieldNumber(invalid) = %d", got)
	}
	if got := firstUnknownFieldNumber(unknownVarintField(123)); got != 123 {
		t.Fatalf("firstUnknownFieldNumber(valid) = %d", got)
	}
	if !isExtensionMessage("ffcraft.ir.v1.ExtensionObject") || isExtensionMessage("ffcraft.ir.v1.Flag") {
		t.Fatal("isExtensionMessage() contract violated")
	}
}

func unknownVarintField(number protowire.Number) []byte {
	return protowire.AppendVarint(protowire.AppendTag(nil, number, protowire.VarintType), 1)
}

func minimalCodecDocument() *irv1.Document {
	return &irv1.Document{Flags: map[string]*irv1.Flag{"f": {
		Variants: map[string]*irv1.VariantValue{
			"on": {Kind: &irv1.VariantValue_BoolValue{BoolValue: true}},
		},
		Environments: map[string]*irv1.Environment{"prod": {
			Base: &irv1.Evaluation{DefaultAction: &irv1.Action{Kind: &irv1.Action_Serve{Serve: "on"}}},
		}},
	}}}
}
