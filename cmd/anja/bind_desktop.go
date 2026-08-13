// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/tools/go/packages"
)

type desktopTarget struct {
	goos   string
	goarch string
}

func (t desktopTarget) String() string {
	return t.goos + "/" + t.goarch
}

func (t desktopTarget) jarSubdir() string {
	return t.goos + "-" + t.goarch
}

var desktopPlatformArchs = map[string][]string{
	"linux":   {"386", "amd64", "arm", "arm64"},
	"darwin":  {"amd64", "arm64"},
	"windows": {"386", "amd64", "arm64"},
}

func hostDesktopTarget() (desktopTarget, error) {
	archs, ok := desktopPlatformArchs[runtime.GOOS]
	if !ok {
		return desktopTarget{}, fmt.Errorf("unsupported host platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	for _, arch := range archs {
		if arch == runtime.GOARCH {
			return desktopTarget{goos: runtime.GOOS, goarch: runtime.GOARCH}, nil
		}
	}
	return desktopTarget{}, fmt.Errorf("unsupported host platform: %s/%s", runtime.GOOS, runtime.GOARCH)
}

func parseDesktopTargets(spec string) ([]desktopTarget, error) {
	if spec == "" {
		spec = "host"
	}

	var targets []desktopTarget
	targetsAdded := map[desktopTarget]bool{}
	addTarget := func(t desktopTarget) {
		if targetsAdded[t] {
			return
		}
		targets = append(targets, t)
		targetsAdded[t] = true
	}

	for _, rawToken := range strings.Split(spec, ",") {
		token := strings.TrimSpace(rawToken)
		if token == "" {
			continue
		}
		if token == "host" {
			target, err := hostDesktopTarget()
			if err != nil {
				return nil, err
			}
			addTarget(target)
			continue
		}

		parts := strings.Split(token, "/")
		switch len(parts) {
		case 1:
			archs, ok := desktopPlatformArchs[parts[0]]
			if !ok {
				return nil, fmt.Errorf("unsupported desktop platform: %q", parts[0])
			}
			for _, arch := range archs {
				addTarget(desktopTarget{goos: parts[0], goarch: arch})
			}
		case 2:
			archs, ok := desktopPlatformArchs[parts[0]]
			if !ok {
				return nil, fmt.Errorf("unsupported desktop platform: %q", parts[0])
			}
			if !contains(archs, parts[1]) {
				return nil, fmt.Errorf("unsupported desktop target: %q", token)
			}
			addTarget(desktopTarget{goos: parts[0], goarch: parts[1]})
		default:
			return nil, fmt.Errorf("invalid desktop target: %q", token)
		}
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("no desktop target specified")
	}
	return targets, nil
}

func goDesktopBind(libName string, anjb string, pkgs []*packages.Package, jvmOnly bool) error {
	targets, err := parseDesktopTargets(bindDesktopTargets)
	if err != nil {
		return fmt.Errorf("invalid -desktoptargets=%q: %v", bindDesktopTargets, err)
	}

	var javaSrcDir string
	nativeLibs := map[desktopTarget]string{}
	for _, target := range targets {
		outDir := filepath.Join(tmpdir, "desktop", strings.ReplaceAll(target.String(), "/", "_"))
		if err := runDesktopGobind(anjb, libName, pkgs, target, outDir); err != nil {
			return fmt.Errorf("failed to generate desktop bindings for %s: %w", target, err)
		}
		if javaSrcDir == "" {
			javaSrcDir = filepath.Join(outDir, "java")
		}

		libPath := filepath.Join(tmpdir, "desktop", "natives", target.jarSubdir(), desktopLibraryFilename(libName, target.goos))
		if err := buildDesktopSO(libName, target, outDir, libPath); err != nil {
			return fmt.Errorf("failed to build desktop shared library for %s: %w", target, err)
		}
		if err := exportDesktopNative(libPath, target); err != nil {
			return err
		}
		nativeLibs[target] = libPath
	}

	return buildDesktopJar(javaSrcDir, nativeLibs, pkgs, jvmOnly)
}

// exportDesktopNative copies a built desktop shared library into the
// -nativesout directory, mirroring the JAR's natives/<goos>-<goarch>/
// layout. Packagers can then ship the library as a plain file — loaded by
// the JVM via the anja.natives.dir system property, or by any other host
// process — without unpacking the JAR.
func exportDesktopNative(libPath string, target desktopTarget) error {
	if bindNativesOut == "" {
		return nil
	}
	dst := filepath.Join(bindNativesOut, target.jarSubdir(), filepath.Base(libPath))
	if err := copyFile(dst, libPath); err != nil {
		return fmt.Errorf("failed to export desktop shared library for %s: %w", target, err)
	}
	return nil
}

func runDesktopGobind(anjb string, libName string, pkgs []*packages.Package, target desktopTarget, outDir string) error {
	cmd := exec.Command(
		anjb,
		"-lang=go,java",
		"-javaruntime=jvm",
		"-outdir="+outDir,
	)
	cmd.Env = append(cmd.Env, "GOOS="+target.goos)
	cmd.Env = append(cmd.Env, "GOARCH="+target.goarch)
	cmd.Env = append(cmd.Env, "CGO_ENABLED=1")
	if target.goarch == "arm" {
		cmd.Env = append(cmd.Env, "GOARM=7")
	}
	if len(buildTags) > 0 {
		cmd.Args = append(cmd.Args, "-tags="+strings.Join(buildTags, ","))
	}
	if bindJavaPkg != "" {
		cmd.Args = append(cmd.Args, "-javapkg="+bindJavaPkg)
	}
	if bindClasspath != "" {
		cmd.Args = append(cmd.Args, "-classpath="+bindClasspath)
	}
	if libName != "" {
		cmd.Args = append(cmd.Args, "-libname="+libName)
	}
	if bindLinkOnly != "" {
		cmd.Args = append(cmd.Args, "-linkonly="+bindLinkOnly)
	}
	for _, p := range pkgs {
		cmd.Args = append(cmd.Args, p.PkgPath)
	}
	return runCmd(cmd)
}

func buildDesktopSO(libName string, target desktopTarget, outDir string, outPath string) error {
	jniCflags, err := desktopJNICFlags(target)
	if err != nil {
		return err
	}

	env := []string{
		"GOOS=" + target.goos,
		"GOARCH=" + target.goarch,
		"CGO_ENABLED=1",
		"CGO_CFLAGS=" + jniCflags,
	}
	if target.goarch == "arm" {
		env = append(env, "GOARM=7")
	}

	gopath := fmt.Sprintf("GOPATH=%s%c%s", outDir, filepath.ListSeparator, goEnv("GOPATH"))
	env = append(env, gopath)

	modulesUsed, err := areGoModulesUsed()
	if err != nil {
		return err
	}

	srcDir := filepath.Join(outDir, "src", "anjb")
	if modulesUsed {
		newSrcDir, _ := filepath.Abs(filepath.Join(".", "build", target.goos+"_"+target.goarch, "lib"+libName))
		if err := os.MkdirAll(newSrcDir, 0755); err != nil {
			return err
		}
		if !buildN {
			if err := doCopyAll(newSrcDir, srcDir); err != nil {
				return err
			}
		}
		srcDir = newSrcDir
		defer os.RemoveAll(srcDir)
	}

	if !buildN {
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}
	}
	return goBuildAt(
		srcDir,
		".",
		env,
		"-buildmode=c-shared",
		"-o="+outPath,
	)
}

func desktopJNICFlags(target desktopTarget) (string, error) {
	osIncludeDir, err := desktopJNIPlatformIncludeDir(target.goos)
	if err != nil {
		return "", err
	}

	customIncludeDir, configured, err := configuredDesktopJNIIncludeDir(osIncludeDir)
	if err != nil {
		return "", err
	}
	if configured {
		flags := "-I" + customIncludeDir + " -I" + filepath.Join(customIncludeDir, osIncludeDir)
		if existing := os.Getenv("CGO_CFLAGS"); existing != "" {
			flags = existing + " " + flags
		}
		return flags, nil
	}

	javaHome, err := findJavaHome(osIncludeDir)
	if err != nil {
		return "", err
	}

	includeDir := filepath.Join(javaHome, "include")
	includeOSDir := filepath.Join(includeDir, osIncludeDir)
	flags := "-I" + includeDir + " -I" + includeOSDir
	if existing := os.Getenv("CGO_CFLAGS"); existing != "" {
		flags = existing + " " + flags
	}
	return flags, nil
}

func desktopJNIPlatformIncludeDir(goos string) (string, error) {
	switch goos {
	case "linux":
		return "linux", nil
	case "darwin":
		return "darwin", nil
	case "windows":
		return "win32", nil
	default:
		return "", fmt.Errorf("unsupported desktop platform: %q", goos)
	}
}

func configuredDesktopJNIIncludeDir(osIncludeDir string) (string, bool, error) {
	if bindJNIInclude == "" {
		return "", false, nil
	}

	includeDir, err := desktopJNIIncludeDir(filepath.Clean(bindJNIInclude), osIncludeDir)
	if err == nil {
		return includeDir, true, nil
	}
	return "", false, nil
}

func desktopJNIIncludeDir(root string, osIncludeDir string) (string, error) {
	includeDir := filepath.Clean(root)
	if _, err := os.Stat(filepath.Join(includeDir, "jni.h")); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(includeDir, osIncludeDir, "jni_md.h")); err != nil {
		return "", err
	}
	return includeDir, nil
}

func findJavaHome(osIncludeDir string) (string, error) {
	candidates := map[string]bool{}
	addCandidate := func(path string) {
		if path == "" {
			return
		}
		candidates[filepath.Clean(path)] = true
	}

	addCandidate(os.Getenv("JAVA_HOME"))

	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("/usr/libexec/java_home").Output(); err == nil {
			addCandidate(strings.TrimSpace(string(out)))
		}
	}

	if javacPath, err := exec.LookPath("javac"); err == nil {
		if evaled, evalErr := filepath.EvalSymlinks(javacPath); evalErr == nil {
			javacPath = evaled
		}
		addCandidate(filepath.Join(filepath.Dir(javacPath), ".."))
	}

	for candidate := range candidates {
		includeDir := filepath.Join(candidate, "include")
		if _, err := os.Stat(filepath.Join(includeDir, "jni.h")); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(includeDir, osIncludeDir, "jni_md.h")); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("unable to locate JAVA_HOME with matching JNI headers; set JAVA_HOME to a full JDK for the target platform")
}

func desktopLibraryFilename(libName string, goos string) string {
	switch goos {
	case "windows":
		return libName + ".dll"
	case "darwin":
		return "lib" + libName + ".dylib"
	default:
		return "lib" + libName + ".so"
	}
}

func desktopJarOutput(pkgs []*packages.Package, jvmOnly bool) string {
	if bindDesktopO != "" {
		return bindDesktopO
	}
	if buildO != "" {
		if jvmOnly {
			return buildO
		}
		ext := filepath.Ext(buildO)
		if ext == "" {
			return buildO + "-desktop.jar"
		}
		return buildO[:len(buildO)-len(ext)] + "-desktop.jar"
	}
	if jvmOnly {
		return pkgs[0].Name + ".jar"
	}
	return pkgs[0].Name + "-desktop.jar"
}

func buildDesktopJar(srcDir string, nativeLibs map[desktopTarget]string, pkgs []*packages.Package, jvmOnly bool) (err error) {
	var out io.Writer = io.Discard
	output := desktopJarOutput(pkgs, jvmOnly)

	if !buildN {
		f, err := os.Create(output)
		if err != nil {
			return err
		}
		defer func() {
			if cerr := f.Close(); err == nil {
				err = cerr
			}
		}()
		out = f
	}

	var srcFiles []string
	if buildN {
		srcFiles = []string{"*.java"}
	} else {
		if err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if filepath.Ext(path) == ".java" {
				srcFiles = append(srcFiles, filepath.Join(".", path[len(srcDir):]))
			}
			return nil
		}); err != nil {
			return err
		}
	}

	dst := filepath.Join(tmpdir, "javac-output-desktop")
	if !buildN {
		if err := os.MkdirAll(dst, 0700); err != nil {
			return err
		}
	}

	args := []string{
		"-g", "-parameters",
		"-d", dst,
		"-source", javacTargetVer,
		"-target", javacTargetVer,
	}
	if bindClasspath != "" {
		args = append(args, "-classpath", bindClasspath)
	}
	args = append(args, srcFiles...)

	javac := exec.Command("javac", args...)
	javac.Dir = srcDir
	if err := runCmd(javac); err != nil {
		return err
	}

	extraFiles := map[string]string{}
	for target, path := range nativeLibs {
		entry := filepath.ToSlash(filepath.Join("natives", target.jarSubdir(), filepath.Base(path)))
		extraFiles[entry] = path
	}

	return writeJarWithFiles(out, dst, extraFiles)
}
