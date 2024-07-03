package gengateway

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/internal/descriptor"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func newExampleFileDescriptorWithGoPkg(gp *descriptor.GoPackage, filenamePrefix string) *descriptor.File {
	msgdesc := &descriptorpb.DescriptorProto{
		Name: proto.String("ExampleMessage"),
	}
	msg := &descriptor.Message{
		DescriptorProto: msgdesc,
	}
	msg1 := &descriptor.Message{
		DescriptorProto: msgdesc,
		File: &descriptor.File{
			GoPkg: descriptor.GoPackage{
				Path: "github.com/golang/protobuf/ptypes/empty",
				Name: "emptypb",
			},
		},
	}
	meth := &descriptorpb.MethodDescriptorProto{
		Name:       proto.String("Example"),
		InputType:  proto.String("ExampleMessage"),
		OutputType: proto.String("ExampleMessage"),
	}
	meth1 := &descriptorpb.MethodDescriptorProto{
		Name:       proto.String("ExampleWithoutBindings"),
		InputType:  proto.String("empty.Empty"),
		OutputType: proto.String("empty.Empty"),
	}
	svc := &descriptorpb.ServiceDescriptorProto{
		Name:   proto.String("ExampleService"),
		Method: []*descriptorpb.MethodDescriptorProto{meth, meth1},
	}
	return &descriptor.File{
		FileDescriptorProto: &descriptorpb.FileDescriptorProto{
			Name:        proto.String("example.proto"),
			Package:     proto.String("example"),
			Dependency:  []string{"a.example/b/c.proto", "a.example/d/e.proto"},
			MessageType: []*descriptorpb.DescriptorProto{msgdesc},
			Service:     []*descriptorpb.ServiceDescriptorProto{svc},
		},
		GoPkg:                   *gp,
		GeneratedFilenamePrefix: filenamePrefix,
		Messages:                []*descriptor.Message{msg},
		Services: []*descriptor.Service{
			{
				ServiceDescriptorProto: svc,
				Methods: []*descriptor.Method{
					{
						MethodDescriptorProto: meth,
						RequestType:           msg,
						ResponseType:          msg,
						Bindings: []*descriptor.Binding{
							{
								HTTPMethod: "GET",
								Body:       &descriptor.Body{FieldPath: nil},
							},
						},
					},
					{
						MethodDescriptorProto: meth1,
						RequestType:           msg1,
						ResponseType:          msg1,
					},
				},
			},
		},
	}
}

func newExampleFileDescriptorWithGoPkgWithoutBinding(gp *descriptor.GoPackage, filenamePrefix string) *descriptor.File {
	file := newExampleFileDescriptorWithGoPkg(gp, filenamePrefix)
	for _, service := range file.Services {
		for _, method := range service.Methods {
			if method != nil {
				method.Bindings = nil
			}
		}
	}
	return file
}

func TestGenerator_Generate(t *testing.T) {
	g := new(generator)
	g.reg = descriptor.NewRegistry()
	result, err := g.Generate([]*descriptor.File{
		crossLinkFixture(newExampleFileDescriptorWithGoPkg(&descriptor.GoPackage{
			Path: "example.com/path/to/example",
			Name: "example_pb",
		}, "path/to/example")),
	})
	if err != nil {
		t.Fatalf("failed to generate stubs: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected to generate one file, got: %d", len(result))
	}
	expectedName := "path/to/example.pb.gw.go"
	gotName := result[0].GetName()
	if gotName != expectedName {
		t.Fatalf("invalid name %q, expected %q", gotName, expectedName)
	}
}

func TestGenerator_GenerateSeparatePackage(t *testing.T) {
	reg := descriptor.NewRegistry()
	reg.SetSeparatePackage(true)
	reg.SetStandalone(true)
	g := New(reg, true, "Handler", true, true, true)
	targets := []*descriptor.File{
		crossLinkFixture(newExampleFileDescriptorWithGoPkg(&descriptor.GoPackage{
			Path:  "example.com/mymodule/foo/bar/v1",
			Name:  "v1" + "gateway", // Name is appended with "gateway" with standalone set to true.
			Alias: "extalias",
		}, "foo/bar/v1/example")),
	}
	// Set ForcePrefixedName (usually set when standalone=true).
	for _, f := range targets {
		for _, msg := range f.Messages {
			msg.ForcePrefixedName = true
			for _, field := range msg.Fields {
				field.ForcePrefixedName = true
			}
		}
		for _, enum := range f.Enums {
			enum.ForcePrefixedName = true
		}
		for _, svc := range f.Services {
			packageName := strings.TrimSuffix(svc.File.GoPkg.Name, "gateway") + "grpc"
			svc.ForcePrefixedName = true
			// replicates behavior in internal/descriptor/services.go (loadServices)
			svc.GRPCFile = &descriptor.File{
				GoPkg: descriptor.GoPackage{
					Path: strings.Replace(
						filepath.Join(svc.File.GoPkg.Path, packageName),
						"protocolbuffers/go",
						"grpc/go",
						1,
					),
					Name: strings.Replace(packageName, "protocolbuffers/go", "grpc/go", 1),
				},
			}
			reg.IncludeAdditionalImports(svc, f.GoPkg)
		}
	}
	result, err := g.Generate(targets)
	if err != nil {
		t.Fatalf("failed to generate stubs: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected to generate 2 files, got: %d", len(result))
	}
	expectedName := "foo/bar/v1/v1gateway/example.pb.gw.go"
	expectedGoPkgPath := "example.com/mymodule/foo/bar/v1/v1gateway"
	expectedGoPkgName := "v1gateway"
	correctFile := result[0]
	if correctFile == nil {
		t.Fatal("result is nil")
	}
	if correctFile.GetName() != expectedName {
		t.Errorf("invalid name %q, expected %q", correctFile.GetName(), expectedName)
	}
	if correctFile.GoPkg.Path != expectedGoPkgPath {
		t.Errorf("invalid path %q, expected %q", result[0].GoPkg.Path, expectedGoPkgPath)
	}
	if correctFile.GoPkg.Name != expectedGoPkgName {
		t.Errorf("invalid name %q, expected %q", result[0].GoPkg.Name, expectedGoPkgName)
	}
	// Require the two dependencies to be declared as imported packages
	correctFileContent := correctFile.GetContent()
	for _, expectedImport := range []string{
		`extalias "example.com/mymodule/foo/bar/v1"`,
		`"example.com/mymodule/foo/bar/v1/v1grpc"`,
	} {
		if !strings.Contains(correctFileContent, expectedImport) {
			t.Errorf("expected to find import %q in the generated file: %s", expectedImport, correctFileContent[:400])
		}
	}

	expectedName = "foo/bar/v1/example/v1gateway/example.pb.gw.go"
	// wrong path but correct go package
	aliasFile := result[1]
	if aliasFile == nil {
		t.Fatal("result is nil")
	}
	if aliasFile.GetName() != expectedName {
		t.Errorf("invalid name %q, expected %q", aliasFile.GetName(), expectedName)
	}
	if aliasFile.GoPkg.Path != expectedGoPkgPath {
		t.Errorf("invalid path %q, expected %q", aliasFile.GoPkg.Path, expectedGoPkgPath)
	}
	if aliasFile.GoPkg.Name != expectedGoPkgName {
		t.Errorf("invalid name %q, expected %q", aliasFile.GoPkg.Name, expectedGoPkgName)
	}
	aliasFileContent := aliasFile.GetContent()
	// Require the two dependencies to be declared as imported packages
	expectedImport := `aliased "example.com/mymodule/foo/bar/v1/v1gateway"`
	if !strings.Contains(aliasFileContent, expectedImport) {
		t.Errorf("expected to find import %q in the generated file: %s...", expectedImport, aliasFileContent[:500])
	}
	aliasedFunctions := []string{
		"RegisterExampleServiceHandlerServer",
		"RegisterExampleServiceHandlerClient",
		"RegisterExampleServiceHandlerFromEndpoint",
		"RegisterExampleServiceHandler",
	}
	for _, aliasedFunction := range aliasedFunctions {
		aliasDefinition := fmt.Sprintf("%[1]s = aliased.%[1]s", aliasedFunction)
		if !strings.Contains(aliasFileContent, aliasDefinition) {
			t.Fatalf("expected %q in the alias file: %s", aliasDefinition, aliasFileContent)
		}
		if strings.Contains(correctFileContent, aliasDefinition) {
			t.Fatalf("unexpected alias %q in the correct file: %s", aliasDefinition, correctFileContent)
		}
	}
}

func TestGenerator_GenerateSeparatePackage_WithoutBinding(t *testing.T) {
	reg := descriptor.NewRegistry()
	reg.SetSeparatePackage(true)
	reg.SetStandalone(true)
	g := New(reg, true, "Handler", true, true, true)
	targets := []*descriptor.File{
		crossLinkFixture(newExampleFileDescriptorWithGoPkgWithoutBinding(&descriptor.GoPackage{
			Path:  "example.com/mymodule/foo/bar/v1",
			Name:  "v1" + "gateway",
			Alias: "extalias",
		}, "foo/bar/v1/example")),
	}
	result, err := g.Generate(targets)
	if err != nil {
		t.Fatalf("failed to generate stubs: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected to generate 0 file, got: %d", len(result))
	}
}

func TestGenerator_GenerateSeparatePackage_WithOmitPackageDoc(t *testing.T) {
	reg := descriptor.NewRegistry()
	reg.SetSeparatePackage(true)
	reg.SetStandalone(true)
	reg.SetOmitPackageDoc(true)
	g := New(reg, true, "Handler", true, true, true)
	targets := []*descriptor.File{
		crossLinkFixture(newExampleFileDescriptorWithGoPkg(&descriptor.GoPackage{
			Path:  "example.com/mymodule/foo/bar/v1",
			Name:  "v1" + "gateway",
			Alias: "extalias",
		}, "foo/bar/v1/example")),
	}
	result, err := g.Generate(targets)
	if err != nil {
		t.Fatalf("failed to generate stubs: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected to generate 2 files, got: %d", len(result))
	}
	correctFileContent := result[0].GetContent()
	if strings.Contains(correctFileContent, "Deprecated:") {
		t.Errorf("the correct file should not be deprecated: %s...", correctFileContent[:500])
	}
	deprecationDoc := `/*
Deprecated: This package has moved to "example.com/mymodule/foo/bar/v1/v1gateway". Use that import path instead.
*/`
	aliasFileContent := result[1].GetContent()
	// Even though omit_package_doc is set, we still need to deprecate the package.
	if !strings.Contains(aliasFileContent, deprecationDoc) {
		t.Errorf("expected to find deprecation doc in the alias file: %s...", aliasFileContent[:500])
	}
}

func TestGenerator_GenerateSeparatePackage_WithoutService(t *testing.T) {
	reg := descriptor.NewRegistry()
	reg.SetSeparatePackage(true)
	reg.SetStandalone(true)
	g := New(reg, true, "Handler", true, true, true)
	targets := []*descriptor.File{
		{
			FileDescriptorProto: &descriptorpb.FileDescriptorProto{
				Name:    proto.String("example.proto"),
				Package: proto.String("example"),
			},
			GoPkg: descriptor.GoPackage{
				Path: "foo/bar/baz/gen/v1",
				Name: "v1",
			},
			GeneratedFilenamePrefix: "gen/v1/example",
		},
	}
	result, err := g.Generate(targets)
	if err != nil {
		t.Fatalf("failed to generate stubs: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected to generate 0 file, got: %d", len(result))
	}
}
