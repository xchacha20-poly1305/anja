# gomobile

Added JVM desktop binding support.

## New desktop/JVM packaging

`gomobile bind` now supports building desktop JVM artifacts in two ways:

1. `-target=android -desktop`:
also builds a desktop JAR in addition to the Android AAR.

2. `-target=jvm`:
builds only the desktop JAR (no AAR output).

### Basic usage

Build only desktop JAR:

```bash
gomobile bind -target=jvm ./your/pkg
```

Set JAR output path:

```bash
gomobile bind -target=jvm -o mylib.jar ./your/pkg
```

Build Android AAR + desktop JAR together:

```bash
gomobile bind -target=android -desktop -o mylib.aar ./your/pkg
```

### Desktop target selection

Use `-desktoptargets` to control native platforms included in the desktop JAR.

Default:

```bash
-desktoptargets=host
```

Example:

```bash
gomobile bind -target=jvm \
  -desktoptargets host,linux/amd64,darwin/arm64,windows/amd64 \
  -jniinclude /path/to/jni-headers \
  -desktopo mylib-desktop.jar \
  ./your/pkg
```

### Custom JNI headers

Use `-jniinclude` when the JNI headers for desktop builds should come from a specific directory instead of the default `JAVA_HOME` lookup.

Directory layout:

```bash
/path/to/jni-headers/
├── jni.h
├── darwin
│   └── jni_md.h
├── linux
│   └── jni_md.h
└── win32
    └── jni_md.h
```

Example:

```bash
gomobile bind -target=jvm \
  -jniinclude /path/to/jni-headers \
  ./your/pkg
```

Lookup order for each desktop target:

- First check `<jniinclude>/jni.h` and `<jniinclude>/<os>/jni_md.h`.
- `os` maps to `linux`, `darwin`, or `win32`.
- If the configured directory does not contain matching JNI headers, fall back to the existing `JAVA_HOME` / `javac` detection.

Supported target syntax:

- `host`
- `linux` or `linux/<arch>`
- `darwin` or `darwin/<arch>`
- `windows` or `windows/<arch>`

Supported desktop arches by platform:

- Linux: `386`, `amd64`, `arm`, `arm64`
- macOS: `amd64`, `arm64`
- Windows: `386`, `amd64`, `arm64`

## Notes

- Desktop builds require a full JDK (JNI headers), usually via `JAVA_HOME`.
- Use `-jniinclude` to point desktop builds at a custom JNI headers directory. `gomobile` checks `<root>/<os>/` first and falls back to the existing `JAVA_HOME` detection when the platform headers are not present.
- Cross-platform desktop builds require matching C cross-compilers/toolchains installed.
