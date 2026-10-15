#!/bin/bash
set -euo pipefail

# ──────────────────────────────────────────────────────────────────────────────
# webcurl-desktop build script
# Usage: ./scripts/build.sh [flags]
#
#   --version X.Y.Z    Set version (accepts v1.2.3 / 1.2.3)
#                      Injected into the binary via -ldflags -X, so the version
#                      travels with the executable. Omitting it falls back to
#                      `git describe`. See cmd/versionldflags.
#   --dev              Development build (Vite dev server)
#   --package          Create distributable (NSIS/DMG/AppImage)
#   --server           Server mode (no GUI, HTTP server only)
#   --docker           Build Docker image for server mode
#   --release          Build release packages: zip + SHA256SUMS [+ sign]
#   --platform LIST    Comma-separated: windows/amd64,darwin/arm64,linux/amd64
#                      or "all" for all 5 common platforms.
#                      Default: current platform only.
#   --sign-key PATH    Ed25519 private key for signing (requires --release)
#   --help             Show this help
# ──────────────────────────────────────────────────────────────────────────────

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="$ROOT_DIR/bin"
CONFIG_YML="$ROOT_DIR/build/config.yml"
RELEASE_DIR="$ROOT_DIR/release"
APP_NAME="webcurl-desktop"

ALL_PLATFORMS=(
    "windows/amd64"
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
)

# ── Parse flags ──────────────────────────────────────────────────────────────
VERSION=""
BUILD_ARGS=()
PACKAGE=false
RELEASE=false
SIGN_KEY=""
PLATFORMS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --version)  VERSION="$2"; shift 2 ;;
        --dev)      BUILD_ARGS+=("DEV=true"); shift ;;
        --package)  PACKAGE=true; shift ;;
        --server)   BUILD_ARGS+=("TAGS=server"); shift ;;
        --docker)   cd "$ROOT_DIR"; task build:docker; exit 0 ;;
        --release)  RELEASE=true; shift ;;
        --sign-key) SIGN_KEY="$2"; shift 2 ;;
        --platform)
            IFS=',' read -ra parts <<< "$2"
            for p in "${parts[@]}"; do
                if [ "$p" = "all" ]; then
                    PLATFORMS=("${ALL_PLATFORMS[@]}")
                    break
                fi
                PLATFORMS+=("$p")
            done
            shift 2 ;;
        --help)
            sed -n '/^ROOT_DIR/q; 3,$p' "$0" | sed -n '/^#/{s/^# \?//; p}'
            exit 0 ;;
        *) echo "Unknown flag: $1"; exit 1 ;;
    esac
done

# Default to current platform if none specified.
if [ ${#PLATFORMS[@]} -eq 0 ]; then
    case "$(uname -s)" in
        MINGW*|MSYS*|CYGWIN*) CUR_OS="windows" ;;
        Darwin)                CUR_OS="darwin" ;;
        Linux)                 CUR_OS="linux" ;;
        *) echo "Unknown OS: $(uname -s)"; exit 1 ;;
    esac
    case "$(uname -m)" in
        x86_64|amd64)  CUR_ARCH="amd64" ;;
        arm64|aarch64) CUR_ARCH="arm64" ;;
        *) echo "Unknown arch: $(uname -m)"; exit 1 ;;
    esac
    PLATFORMS=("$CUR_OS/$CUR_ARCH")
fi

# ── Dependencies ──────────────────────────────────────────────────────────────
require() { command -v "$1" &>/dev/null || { echo "❌ $1 not found"; exit 1; }; }
require wails3

# Cross-platform zip and sha256 helpers.

# 把 MSYS/Git Bash 的 /c/foo/bar 形式转成 Windows 的 C:\foo\bar。
to_win_path() {
    echo "$1" | sed 's|^/\([a-z]\)/|\1:/|' | sed 's|/|\\|g'
}

# do_zip <目标 zip> <文件>...
# 所有文件都平铺在压缩包根目录下（不带外层目录）：更新助手解压后要直接看到负载，
# 多一层目录会让它在定位主程序时找错地方。
do_zip() {
    local dst="$1"; shift
    if [ $# -eq 0 ]; then
        echo "❌ do_zip: 没有要打包的文件"; exit 1
    fi

    local dir names=() f n
    dir="$(cd "$(dirname "$1")" && pwd)"
    for f in "$@"; do
        names+=("$(basename "$f")")
    done

    if command -v zip &>/dev/null; then
        (cd "$dir" && zip -q -r "$dst" "${names[@]}")
    elif command -v powershell &>/dev/null; then
        local wdst wpaths=""
        wdst=$(to_win_path "$dst")
        for n in "${names[@]}"; do
            wpaths="${wpaths:+$wpaths,}'$(to_win_path "$dir")\\$n'"
        done
        powershell -Command "Compress-Archive -Path $wpaths -DestinationPath '$wdst' -Force"
    else
        echo "❌ zip or powershell required"; exit 1
    fi
}

do_sha256() {
    local file="$1"
    if command -v sha256sum &>/dev/null; then
        sha256sum "$file"
    elif command -v shasum &>/dev/null; then
        shasum -a 256 "$file" | sed 's| .*/|  |'
    elif command -v certutil &>/dev/null; then
        # certutil -hashfile outputs lines; grab the hash
        local hash
        hash=$(certutil -hashfile "$file" SHA256 | sed -n '2p' | tr -d ' ')
        echo "$hash  $(basename "$file")"
    else
        echo "❌ sha256sum / shasum / certutil required"; exit 1
    fi
}

# ── Version ───────────────────────────────────────────────────────────────────
# 版本号的来源只有一个：internal/buildinfo（$GOWS_VERSION 优先，否则 git describe）。
# 它有两个出口：
#   1. GOWS_VERSION → Taskfile 的 -ldflags -X → internal/version。
#      这是二进制**自身携带**的版本，也是更新检查与界面展示采用的权威值。
#   2. build/config.yml 的 info.version：Wails 据此生成各平台元数据
#      （macOS 的 Info.plist、iOS 的工程配置等），只能在构建前写进文件。
#      Windows 的版本资源**不在这里写** —— 它由构建任务里的 cmd/wininfo 在每次
#      构建时从同一个版本推导，见 build/windows/Taskfile.yml 的 generate:wininfo。
#
# 这里刻意**不再**改写 configs/config.yaml：那个文件现在只在"没有注入版本"时
# （本地 go run / go test）充当兜底，构建流程不该改动开发者的工作区。
if [ -n "${VERSION:-}" ]; then
    GOWS_VERSION="${VERSION#v}"
else
    # 未显式指定时取 git describe，让产物命名与二进制内嵌版本保持一致。
    GOWS_VERSION=$(git -C "$ROOT_DIR" describe --tags --always 2>/dev/null | sed 's/^v//' || true)
fi
export GOWS_VERSION

VERSION="${VERSION:-${GOWS_VERSION:-0.0.0}}"
SEMVER="${VERSION#v}"

# ── Update signing public key ─────────────────────────────────────────────────
# 客户端验签用的公钥由签名私钥推导，并在构建时注入二进制
# （internal/updater.EmbeddedPublicKey）。它必须内嵌而不是随配置分发：
# 否则能改配置文件的人就能把信任锚换成自己的密钥，验签形同虚设。
#
# 注入值优先于配置里的 update.public_key，因此一旦这里注入，
# 客户端就会强制要求签名 —— 缺失或验签失败都会中止安装。
if [ -n "$SIGN_KEY" ]; then
    if [ ! -f "$SIGN_KEY" ]; then
        echo "❌ sign key not found: $SIGN_KEY"
        exit 1
    fi
    require openssl
    PUB_DER="$(mktemp)"
    trap 'rm -f "$PUB_DER"' EXIT
    if ! openssl pkey -in "$SIGN_KEY" -pubout -outform DER -out "$PUB_DER" >/dev/null 2>&1; then
        echo "❌ cannot derive a public key from $SIGN_KEY (expecting an Ed25519 key)"
        exit 1
    fi
    # Ed25519 的 SubjectPublicKeyInfo 固定为 12 字节前缀 + 32 字节原始公钥，
    # 末尾没有其它内容，因此取最后 32 字节即可。
    PUB_SIZE="$(wc -c < "$PUB_DER" | tr -d ' ')"
    if [ "$PUB_SIZE" -ne 44 ]; then
        echo "❌ $SIGN_KEY is not an Ed25519 key (derived public key is $PUB_SIZE bytes, want 44)"
        exit 1
    fi
    GOWS_UPDATE_PUBLIC_KEY="$(tail -c 32 "$PUB_DER" | base64 | tr -d '\n')"
    export GOWS_UPDATE_PUBLIC_KEY
    echo "🔑 Update signing public key: $GOWS_UPDATE_PUBLIC_KEY"
else
    echo "⚠  --sign-key not given: the binary embeds no public key, so clients skip signature verification"
fi

if [ -n "$GOWS_VERSION" ]; then
    # build/config.yml 的版本字段要求四段数字形式，所以先剥掉 -3-gabc1234 之类的后缀；
    # 完全推导不出数字版本时（例如仓库还没有 tag）退回 0.0.0。
    NORM_VERSION="${GOWS_VERSION%%-*}"
    case "$NORM_VERSION" in
        [0-9]*) ;;
        *) NORM_VERSION="0.0.0" ;;
    esac
    SEGMENTS=$(printf '%s' "$NORM_VERSION" | tr -cd '.' | wc -c | tr -d ' ')
    case "$SEGMENTS" in
        0) NORM_VERSION="$NORM_VERSION.0.0.0" ;;
        1) NORM_VERSION="$NORM_VERSION.0.0" ;;
        2) NORM_VERSION="$NORM_VERSION.0" ;;
    esac

    # build/config.yml (Wails build metadata)
    if command -v yq &>/dev/null; then
        yq eval -i ".info.version = \"$NORM_VERSION\"" "$CONFIG_YML"
    else
        sed -i "s/^  version: \".*\"/  version: \"$NORM_VERSION\"/" "$CONFIG_YML"
    fi

    # 依 config.yml 重生成各平台构建资产（Info.plist、工程配置等）。
    # 注意它会一并重写 build/windows/info.json —— 那是 wails3 的可再生资产，被改写
    # 属预期行为；exe 的版本资源并不依赖它，构建任务会用 cmd/wininfo 另生成一份。
    cd "$ROOT_DIR"
    task common:update:build-assets 2>&1 || true
fi

# ── Frontend deps ─────────────────────────────────────────────────────────────
if [ -f "$ROOT_DIR/frontend/package.json" ]; then
    cd "$ROOT_DIR/frontend"
    if [ -f "pnpm-lock.yaml" ]; then
        pnpm install --frozen-lockfile 2>/dev/null || pnpm install
    elif [ -f "package-lock.json" ]; then
        npm ci 2>/dev/null || npm install
    elif [ -f "yarn.lock" ]; then
        yarn install --frozen-lockfile 2>/dev/null || yarn install
    else
        npm install
    fi
    cd "$ROOT_DIR"
fi

# ── Build each platform ───────────────────────────────────────────────────────
if [ "$RELEASE" = true ]; then
    rm -rf "$RELEASE_DIR"
    mkdir -p "$RELEASE_DIR"
fi

for PLAT in "${PLATFORMS[@]}"; do
    OS="${PLAT%/*}"
    ARCH="${PLAT#*/}"
    EXE_EXT=""
    [ "$OS" = "windows" ] && EXE_EXT=".exe"

    RELEASE_NAME="${APP_NAME}-v${SEMVER}-${OS}-${ARCH}"
    BIN_NAME="${APP_NAME}${EXE_EXT}"

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "→ Building $RELEASE_NAME ..."
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    cd "$ROOT_DIR"
    # GOWS_VERSION is exported above; the Taskfiles turn it into
    # -ldflags "-X webcurl-desktop/internal/version.Version=..." so the binary
    # carries its own version instead of trusting configs/config.yaml.
    if [ "${#BUILD_ARGS[@]}" -gt 0 ]; then
        GOOS="$OS" GOARCH="$ARCH" wails3 build ${BUILD_ARGS[@]}
    else
        GOOS="$OS" GOARCH="$ARCH" wails3 build
    fi

    # The update helper must sit next to the app binary, so build one per target
    # platform. Without it updater.Apply fails at the hand-off step.
    GOOS="$OS" GOARCH="$ARCH" go build -o "$BIN_DIR/updater${EXE_EXT}" ./cmd/updater

    # Copy configs alongside the binary. config.yaml is git-ignored, so fall
    # back to the tracked example on a fresh clone.
    mkdir -p "$BIN_DIR/configs"
    if [ -f "$ROOT_DIR/configs/config.yaml" ]; then
        cp "$ROOT_DIR/configs/config.yaml" "$BIN_DIR/configs/config.yaml"
    else
        echo "   ! configs/config.yaml not found, shipping config.example.yaml"
        cp "$ROOT_DIR/configs/config.example.yaml" "$BIN_DIR/configs/config.yaml"
    fi

    # ── Release packaging ─────────────────────────────────────────────────────
    if [ "$RELEASE" = true ]; then
        # 两个文件都平铺在压缩包根目录下，助手才能直接 rename 到位。
        #
        # 助手（updater）必须一起打进去：绿色版解压后要自带它，否则应用内更新会在
        # 交接那一步以 "update helper not found" 中止。带上它也**不会**导致
        # 「替换一个正在运行的助手」失败 —— 主程序运行的是它自己复制到临时目录的
        # 那份副本，安装目录里这一份并未被占用（见 internal/updater/apply.go）。
        ZIP_FILE="$RELEASE_DIR/${RELEASE_NAME}.zip"
        rm -f "$ZIP_FILE"
        do_zip "$ZIP_FILE" "$BIN_DIR/$BIN_NAME" "$BIN_DIR/updater${EXE_EXT}"

        # Append to combined SHA256SUMS.
        (cd "$RELEASE_DIR" && do_sha256 "$(basename "$ZIP_FILE")") >> "$RELEASE_DIR/SHA256SUMS"

        # Sign if key provided.
        if [ -n "$SIGN_KEY" ]; then
            SIGN_BIN="$RELEASE_DIR/${RELEASE_NAME}.sig"
            if command -v openssl &>/dev/null; then
                openssl pkeyutl -sign -inkey "$SIGN_KEY" -in "$ZIP_FILE" \
                    -out "$SIGN_BIN" -rawin 2>/dev/null || true
            fi
        fi

        echo "   → $ZIP_FILE"
    fi
done

# ── Package ───────────────────────────────────────────────────────────────────
if [ "$PACKAGE" = true ]; then
    cd "$ROOT_DIR"
    task package
fi

# ── Done ─────────────────────────────────────────────────────────────────────
echo ""
if [ "$RELEASE" = true ]; then
    echo "✅ Release built ($OS/$ARCH):"
    ls -lh "$RELEASE_DIR"/*.zip "$RELEASE_DIR"/SHA256SUMS 2>/dev/null
    [ -n "$SIGN_KEY" ] && ls -lh "$RELEASE_DIR"/*.sig 2>/dev/null
    echo ""
    echo "Upload to GitHub release:"
    echo "  Assets: $(ls "$RELEASE_DIR"/*.zip | tr '\n' ' ')"
    echo "  Sidecar: $RELEASE_DIR/SHA256SUMS"
else
    echo "✅ Build complete!"
    echo "   Binary:  $BIN_DIR/$BIN_NAME"
    echo "   Config:  $BIN_DIR/configs/config.yaml"
    echo ""
    echo "   Run: $BIN_DIR/$BIN_NAME"
fi
