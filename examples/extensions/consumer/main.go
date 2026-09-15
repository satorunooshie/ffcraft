// Command consumer demonstrates how an external tool can read only its own
// extension namespace from the public ffcraft.ir.v1 protobuf.
package main

import (
	"flag"
	"fmt"
	"os"

	irv1 "github.com/satorunooshie/ffcraft/gen/ffcraft/ir/v1"
	"google.golang.org/protobuf/proto"
)

func main() {
	in := flag.String("in", "featureflags.ir.v1.pb", "path to ffcraft.ir.v1 protobuf")
	flag.Parse()

	data, err := os.ReadFile(*in)
	if err != nil {
		fatal(err)
	}
	doc := new(irv1.Document)
	if err := proto.Unmarshal(data, doc); err != nil {
		fatal(err)
	}

	flagIR := findFlag(doc, "checkout")
	if flagIR == nil {
		fatal(fmt.Errorf("flag %q not found", "checkout"))
	}

	fmt.Printf("client exposure: %v\n", objectField(flagIR.Extensions["com.example.client.v1"], "exposure"))
	fmt.Printf("backend audit: %v\n", objectField(flagIR.Extensions["com.example.backend.v1"], "audit"))
	fmt.Printf("team owner: %v\n", objectField(flagIR.Extensions["com.example.team.v1"], "owner"))
	if prod := flagIR.Environments["prod"]; prod != nil {
		fmt.Printf("client cache_ttl_seconds: %v\n", objectField(prod.Extensions["com.example.client.v1"], "cache_ttl_seconds"))
		fmt.Printf("backend rollout_ticket: %v\n", objectField(prod.Extensions["com.example.backend.v1"], "rollout_ticket"))
		fmt.Printf("team oncall: %v\n", objectField(prod.Extensions["com.example.team.v1"], "oncall"))
	}
}

func findFlag(doc *irv1.Document, key string) *irv1.Flag {
	return doc.Flags[key]
}

func objectField(value *irv1.ExtensionValue, key string) any {
	if value == nil || value.GetObjectValue() == nil {
		return nil
	}
	field := value.GetObjectValue().Fields[key]
	if field == nil {
		return nil
	}
	switch kind := field.Kind.(type) {
	case *irv1.ExtensionValue_BoolValue:
		return kind.BoolValue
	case *irv1.ExtensionValue_StringValue:
		return kind.StringValue
	case *irv1.ExtensionValue_IntValue:
		return kind.IntValue
	case *irv1.ExtensionValue_DoubleValue:
		return kind.DoubleValue
	default:
		return field.Kind
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
