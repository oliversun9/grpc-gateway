package descriptor

import (
	"path/filepath"
	"strings"
)

// SetSeparatePackage sets separatePackage
func (r *Registry) SetSeparatePackage(use bool) {
	r.separatePackage = use
}

// IncludeAdditionalImports adds additionalImports to the registry on a per-package basis
func (r *Registry) IncludeAdditionalImports(svc *Service, goPkg GoPackage) {
	if !r.separatePackage {
		return
	}
	if r.additionalImports == nil {
		r.additionalImports = make(map[string][]string)
	}
	// when generating a separate package for the gateway, we need to generate an import statement
	// for the gRPC stubs that are no longer in the same package. This is done by adding the grpc
	// package to the additionalImports list. In order to prepare a valid import statement, we'll replace
	// the source package name, something like: ../pet/v1/v1petgateway with ../pet/v1/v1petgrpc
	const (
		baseTypePackageName    = "protocolbuffers"
		baseTypePackageSubPath = baseTypePackageName + "/go"
		grpcPackageName        = "grpc"
		grpcPackageSubPath     = grpcPackageName + "/go"
	)
	packageName := strings.TrimSuffix(goPkg.Name, "gateway") + grpcPackageName
	svc.GRPCFile = &File{
		GoPkg: GoPackage{
			// additionally, as the `go_package` option is passed through from the generator, and can only be
			// set the one time, without making major changes, we'll use the package name sent through the
			// options as a basis, and replace the source package name with the grpc package name.
			Path: strings.Replace(
				filepath.Join(goPkg.Path, packageName),
				baseTypePackageSubPath,
				grpcPackageSubPath,
				1,
			),
			Name: strings.Replace(packageName, baseTypePackageSubPath, grpcPackageSubPath, 1),
		},
	}
	r.additionalImports[goPkg.Path] = append(r.additionalImports[goPkg.Path], svc.GRPCFile.GoPkg.Path)
}

// GetAdditionalImports returns additionalImports
func (r *Registry) GetAdditionalImports(goPkg GoPackage) []string {
	if !r.separatePackage || r.additionalImports == nil {
		return nil
	}
	return r.additionalImports[goPkg.Path]
}
