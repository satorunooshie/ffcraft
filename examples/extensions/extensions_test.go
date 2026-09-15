package extensions_test

import (
	"bytes"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"github.com/satorunooshie/ffcraft/internal/authoring"
	"github.com/satorunooshie/ffcraft/internal/compiler/flagd"
	"github.com/satorunooshie/ffcraft/internal/compiler/gofeatureflag"
	"github.com/satorunooshie/ffcraft/internal/normalize"
)

func TestExtensionNamespacesDoNotChangeCoreOutputs(t *testing.T) {
	data, err := os.ReadFile("ffcompile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	authoringDoc, err := authoring.ParseYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := normalize.Normalize(authoringDoc)
	if err != nil {
		t.Fatal(err)
	}
	withoutExtensions := protoClone(normalized)
	stripExtensions(withoutExtensions)

	flagdWith, _, err := flagd.CompileIR(normalized, "prod", flagd.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	flagdWithout, _, err := flagd.CompileIR(withoutExtensions, "prod", flagd.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(flagdWith, flagdWithout) {
		t.Fatal("flagd output changed after adding extension namespaces")
	}

	goffWith, _, err := gofeatureflag.CompileIR(normalized, "prod", gofeatureflag.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	goffWithout, _, err := gofeatureflag.CompileIR(withoutExtensions, "prod", gofeatureflag.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(goffWith, goffWithout) {
		t.Fatal("GO Feature Flag output changed after adding extension namespaces")
	}
}

func protoClone(doc *irv1.Document) *irv1.Document {
	return proto.Clone(doc).(*irv1.Document)
}

func stripExtensions(doc *irv1.Document) {
	doc.Extensions = nil
	for _, flag := range doc.Flags {
		flag.Extensions = nil
		for _, environment := range flag.Environments {
			environment.Extensions = nil
		}
	}
}
