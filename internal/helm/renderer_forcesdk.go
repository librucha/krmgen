//go:build forcesdk

package helm

// This file exists solely so the golden differential test
// (test/golden/harness_test.go, TestGolden_BothHelmRenderersAgree) can build
// a krmgen binary that genuinely renders every chart through the helm Go
// library, before task 4 of this phase wires real branching into
// selectRenderer (renderer.go).
//
// selectRenderer still returns the binary renderer unconditionally today, so
// setting KRMGEN_HELM_EXECUTABLE only changes which `helm` executable the
// binary renderer shells out to - it can never select the library renderer.
// A binary built with `go build -tags forcesdk` sets forceRenderer
// (processor.go) here, in init, which makes templateHelm use the SDK
// renderer unconditionally regardless of environment. The default,
// tag-free build never compiles this file, so it does not change what
// `go build .` or a release produces.
func init() {
	forceRenderer = newSDKRenderer()
}
