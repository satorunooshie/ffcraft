package compiler_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIArrayMembershipPreservesCodegenAndRejectsProviderOutput(t *testing.T) {
	temp := t.TempDir()
	for _, name := range []string{"ffcompile", "ffcodegen"} {
		build := exec.Command("go", "build", "-o", filepath.Join(temp, name), "./cmd/"+name)
		build.Dir = "../../.."
		if output, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", name, err, output)
		}
	}
	for _, condition := range []string{
		`contains: [{var: user.role_ids}, 3]`,
		`contains: [{var: user.tags}, beta]`,
		`contains: [{var: user.value}, 1.5]`,
		`contains: [{var: user.value}, true]`,
		`in: [3, {var: user.role_ids}]`,
		`in: [beta, {var: user.tags}]`,
	} {
		for _, wrapper := range []string{"direct", "not", "all", "schedule"} {
			t.Run(condition+"/"+wrapper, func(t *testing.T) {
				wrapped := condition
				if wrapper == "not" {
					wrapped = "not: {" + condition + "}"
				}
				if wrapper == "all" {
					wrapped = "all_of: [{literal_bool: true}, {" + condition + "}]"
				}
				source := fmt.Sprintf(`version: v1
variant_sets:
  boolean: {on: true, off: false}
rules:
  membership: {%s}
flags:
  - key: f
    variant_set: boolean
    default_variant: off
    environments:
      prod:
        rules:
          - if: {rule: membership}
            serve: on
        default_action: {serve: off}
`, wrapped)
				if wrapper == "schedule" {
					source = strings.Replace(source, "        rules:\n", "        default_action: {serve: off}\n        scheduled_rollouts:\n          - date: \"2030-01-01T00:00:00Z\"\n            rules:\n", 1)
					source = strings.Replace(source, "          - if: {rule: membership}\n            serve: on\n        default_action:", "              - if: {rule: membership}\n                serve: on\n            default_action:", 1)
				}
				input := filepath.Join(t.TempDir(), "flags.yaml")
				if err := os.WriteFile(input, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				// Normalize once, then exercise both direct authoring and IR input.
				normalized := filepath.Join(filepath.Dir(input), "flags.pb")
				command := exec.Command(filepath.Join(temp, "ffcompile"), "normalize", "--in", input, "--format", "protobuf", "--out", normalized)
				if output, err := command.CombinedOutput(); err != nil {
					t.Fatalf("normalize array condition: %v\n%s", err, output)
				}
				command = exec.Command(filepath.Join(temp, "ffcodegen"), "go", "--in", input)
				output, err := command.CombinedOutput()
				if err != nil || !bytes.Contains(output, []byte("[]")) {
					t.Fatalf("array code generation failed: %v\n%s", err, output)
				}
				for _, target := range []string{"flagd", "gofeatureflag"} {
					for _, mode := range []string{"build", "compile"} {
						sourcePath := input
						if mode == "compile" {
							sourcePath = normalized
						}
						outputPath := filepath.Join(filepath.Dir(input), mode+"-"+target+".out")
						const existing = "existing configuration must survive a failed compile"
						if err := os.WriteFile(outputPath, []byte(existing), 0600); err != nil {
							t.Fatal(err)
						}
						command := exec.Command(filepath.Join(temp, "ffcompile"), mode, target, "--in", sourcePath, "--env", "prod", "--out", outputPath)
						var stderr bytes.Buffer
						command.Stderr = &stderr
						output, err := command.Output()
						if err == nil || len(output) != 0 || !strings.Contains(stderr.String(), "FFCRAFT_TARGET_CONDITION_UNSUPPORTED") {
							t.Errorf("%s %s: output %s, error %v, stderr %s", mode, target, output, err, &stderr)
						}
						preserved, err := os.ReadFile(outputPath)
						if err != nil || string(preserved) != existing {
							t.Fatalf("failed compile changed existing output: %q, %v", preserved, err)
						}
					}
				}
			})
		}
	}
}
