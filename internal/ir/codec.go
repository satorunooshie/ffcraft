package ir

import (
	"fmt"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const UnknownCoreFieldCode = "FFCRAFT_IR_UNKNOWN_CORE_FIELD"

type CoreValidationError struct {
	Code        string
	Path        string
	MessageType string
	FieldNumber protowire.Number
	Cause       error
}

func (e *CoreValidationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s at %s: %v", e.Code, e.Path, e.Cause)
	}
	return fmt.Sprintf("%s: unknown field %d in %s at %s", e.Code, e.FieldNumber, e.MessageType, e.Path)
}

func (e *CoreValidationError) Unwrap() error { return e.Cause }

// Marshal encodes a validated IR document using the normative protobuf wire
// format. Unknown core fields are never emitted by this package.
func Marshal(doc *irv1.Document) ([]byte, error) {
	if err := Validate(doc); err != nil {
		return nil, err
	}
	return proto.MarshalOptions{Deterministic: true}.Marshal(doc)
}

// Unmarshal decodes and validates a normalized IR document. Unknown fields in
// core messages fail closed; extension values remain opaque and are preserved
// by the protobuf runtime.
func Unmarshal(data []byte) (*irv1.Document, error) {
	doc := new(irv1.Document)
	if err := proto.Unmarshal(data, doc); err != nil {
		return nil, err
	}
	if err := rejectUnknownCore(doc); err != nil {
		return nil, err
	}
	if err := Validate(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func rejectUnknownCore(doc *irv1.Document) error {
	return rejectUnknownMessage(doc.ProtoReflect(), false, "$")
}

func rejectUnknownMessage(message protoreflect.Message, opaque bool, path string) error {
	if !message.IsValid() {
		return nil
	}
	if !opaque && len(message.GetUnknown()) != 0 {
		return &CoreValidationError{
			Code: UnknownCoreFieldCode, Path: path, MessageType: string(message.Descriptor().FullName()),
			FieldNumber: firstUnknownFieldNumber(message.GetUnknown()),
		}
	}
	var nestedErr error
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if nestedErr != nil {
			return false
		}
		check := func(child protoreflect.Message, childPath string) {
			nestedErr = rejectUnknownMessage(child, opaque || isExtensionMessage(child.Descriptor().FullName()), childPath)
		}
		switch {
		case field.IsMap():
			value.Map().Range(func(key protoreflect.MapKey, item protoreflect.Value) bool {
				if field.MapValue().Kind() == protoreflect.MessageKind {
					check(item.Message(), fmt.Sprintf("%s.%s[%q]", path, field.Name(), key.String()))
				}
				return nestedErr == nil
			})
		case field.IsList() && field.Message() != nil:
			for i := 0; i < value.List().Len() && nestedErr == nil; i++ {
				check(value.List().Get(i).Message(), fmt.Sprintf("%s.%s[%d]", path, field.Name(), i))
			}
		case field.Message() != nil:
			check(value.Message(), path+"."+string(field.Name()))
		}
		return nestedErr == nil
	})
	return nestedErr
}

func firstUnknownFieldNumber(data []byte) protowire.Number {
	for len(data) > 0 {
		number, _, consumed := protowire.ConsumeField(data)
		if consumed < 0 {
			return 0
		}
		return number
	}
	return 0
}

func isExtensionMessage(name protoreflect.FullName) bool {
	return name == "ffcraft.ir.v1.ExtensionValue" || name == "ffcraft.ir.v1.ExtensionObject" || name == "ffcraft.ir.v1.ExtensionList" || name == "ffcraft.ir.v1.ExtensionNull"
}
