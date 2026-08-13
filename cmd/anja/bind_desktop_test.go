package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfiguredDesktopJNIIncludeDirUsesSingleRoot(t *testing.T) {
	prev := bindJNIInclude
	bindJNIInclude = ""
	t.Cleanup(func() {
		bindJNIInclude = prev
	})

	jniRoot := writeJNIIncludeTree(t, "darwin")
	bindJNIInclude = jniRoot

	got, configured, err := configuredDesktopJNIIncludeDir("darwin")
	if err != nil {
		t.Fatalf("configuredDesktopJNIIncludeDir returned error: %v", err)
	}
	if !configured {
		t.Fatal("configuredDesktopJNIIncludeDir did not report configured entry")
	}
	want := jniRoot
	if got != want {
		t.Fatalf("include dir = %q, want %q", got, want)
	}
}

func TestConfiguredDesktopJNIIncludeDirFallsBackWhenHeadersMissing(t *testing.T) {
	prev := bindJNIInclude
	bindJNIInclude = ""
	t.Cleanup(func() {
		bindJNIInclude = prev
	})

	bindJNIInclude = t.TempDir()

	got, configured, err := configuredDesktopJNIIncludeDir("linux")
	if err != nil {
		t.Fatalf("configuredDesktopJNIIncludeDir returned error: %v", err)
	}
	if configured {
		t.Fatalf("configuredDesktopJNIIncludeDir = %q, want fallback", got)
	}
}

func TestDesktopJNIIncludeDirAcceptsRootAndOSSubdir(t *testing.T) {
	jniRoot := writeJNIIncludeTree(t, "linux")

	got, err := desktopJNIIncludeDir(jniRoot, "linux")
	if err != nil {
		t.Fatalf("desktopJNIIncludeDir returned error: %v", err)
	}
	if got != jniRoot {
		t.Fatalf("include dir = %q, want %q", got, jniRoot)
	}
}

func TestDesktopJNIIncludeDirRejectsDirectOSDir(t *testing.T) {
	jniRoot := writeJNIIncludeTree(t, "darwin")
	directOSDir := filepath.Join(jniRoot, "darwin")

	if _, err := desktopJNIIncludeDir(directOSDir, "darwin"); err == nil {
		t.Fatal("desktopJNIIncludeDir succeeded for direct os dir, want error")
	}
}

func TestDesktopJNICFlagsUsesConfiguredOSDir(t *testing.T) {
	prev := bindJNIInclude
	bindJNIInclude = ""
	t.Cleanup(func() {
		bindJNIInclude = prev
	})

	jniRoot := writeJNIIncludeTree(t, "linux")
	bindJNIInclude = jniRoot
	t.Setenv("CGO_CFLAGS", "")

	got, err := desktopJNICFlags(desktopTarget{goos: "linux", goarch: "amd64"})
	if err != nil {
		t.Fatalf("desktopJNICFlags returned error: %v", err)
	}

	want := "-I" + jniRoot + " -I" + filepath.Join(jniRoot, "linux")
	if got != want {
		t.Fatalf("CGO_CFLAGS = %q, want %q", got, want)
	}
}

func TestDesktopJNICFlagsFallsBackToJavaHomeWhenJNIIncludeMisses(t *testing.T) {
	prev := bindJNIInclude
	bindJNIInclude = ""
	t.Cleanup(func() {
		bindJNIInclude = prev
	})

	javaHome := writeJavaHomeTree(t, "linux")
	bindJNIInclude = t.TempDir()
	t.Setenv("JAVA_HOME", javaHome)
	t.Setenv("CGO_CFLAGS", "")

	got, err := desktopJNICFlags(desktopTarget{goos: "linux", goarch: "amd64"})
	if err != nil {
		t.Fatalf("desktopJNICFlags returned error: %v", err)
	}

	includeDir := filepath.Join(javaHome, "include")
	want := "-I" + includeDir + " -I" + filepath.Join(includeDir, "linux")
	if got != want {
		t.Fatalf("CGO_CFLAGS = %q, want %q", got, want)
	}
}

func writeJNIIncludeTree(t *testing.T, osIncludeDir string) string {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, osIncludeDir), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	writeFileForTest(t, filepath.Join(root, "jni.h"))
	writeFileForTest(t, filepath.Join(root, osIncludeDir, "jni_md.h"))
	return root
}

func writeJavaHomeTree(t *testing.T, osIncludeDir string) string {
	t.Helper()

	root := t.TempDir()
	includeDir := filepath.Join(root, "include")
	if err := os.MkdirAll(filepath.Join(includeDir, osIncludeDir), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	writeFileForTest(t, filepath.Join(includeDir, "jni.h"))
	writeFileForTest(t, filepath.Join(includeDir, osIncludeDir, "jni_md.h"))
	return root
}

func writeFileForTest(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) returned error: %v", path, err)
	}
}

func TestExportDesktopNativeCopiesIntoTargetSubdir(t *testing.T) {
	prev := bindNativesOut
	t.Cleanup(func() {
		bindNativesOut = prev
	})
	outDir := t.TempDir()
	bindNativesOut = outDir

	src := filepath.Join(t.TempDir(), "libgojni.so")
	if err := os.WriteFile(src, []byte("native payload"), 0o644); err != nil {
		t.Fatalf("failed to write source library: %v", err)
	}

	target := desktopTarget{goos: "linux", goarch: "amd64"}
	if err := exportDesktopNative(src, target); err != nil {
		t.Fatalf("exportDesktopNative returned error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(outDir, "linux-amd64", "libgojni.so"))
	if err != nil {
		t.Fatalf("exported library missing: %v", err)
	}
	if string(got) != "native payload" {
		t.Fatalf("exported library content = %q, want %q", got, "native payload")
	}
}

func TestExportDesktopNativeNoopWithoutFlag(t *testing.T) {
	prev := bindNativesOut
	t.Cleanup(func() {
		bindNativesOut = prev
	})
	bindNativesOut = ""

	missing := filepath.Join(t.TempDir(), "does-not-exist.so")
	if err := exportDesktopNative(missing, desktopTarget{goos: "linux", goarch: "amd64"}); err != nil {
		t.Fatalf("exportDesktopNative without -nativesout should be a no-op, got: %v", err)
	}
}
