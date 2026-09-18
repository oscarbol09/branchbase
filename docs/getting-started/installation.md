# Installation 📦

BranchBase is distributed as a single, lightweight binary with zero runtime dependencies.

---

## 🍺 Homebrew (macOS & Linux)

The easiest way to install and stay updated on macOS or Linux is via Homebrew:

```bash
# Add BranchBase tap and install
brew install oscarbol09/branchbase/branchbase
```

Verify installation:
```bash
branchbase version
```

---

## 🐹 Go Toolchain (`go install`)

If you have Go 1.22+ installed on your system:

```bash
go install github.com/oscarbol09/branchbase/cmd/branchbase@latest
```

Make sure your `$GOPATH/bin` or `$HOME/go/bin` is in your system `$PATH`.

---

## 📦 Pre-Compiled Binaries (GitHub Releases)

Pre-built binaries for Linux, macOS (Apple Silicon & Intel), and Windows are available directly on GitHub:

1. Navigate to the **[Latest Release](https://github.com/oscarbol09/branchbase/releases/latest)**.
2. Download the archive for your operating system and architecture:
   - **macOS Apple Silicon:** `branchbase_*_darwin_arm64.tar.gz`
   - **macOS Intel:** `branchbase_*_darwin_amd64.tar.gz`
   - **Linux x86_64:** `branchbase_*_linux_amd64.tar.gz`
   - **Linux ARM64:** `branchbase_*_linux_arm64.tar.gz`
   - **Windows x64:** `branchbase_*_windows_amd64.zip`
3. Extract and move the binary to your `$PATH`:
   ```bash
   tar -xzf branchbase_*_linux_amd64.tar.gz
   sudo mv branchbase /usr/local/bin/
   ```

---

## 🛠️ Build From Source

```bash
git clone https://github.com/oscarbol09/branchbase.git
cd branchbase
go build -o bin/branchbase ./cmd/branchbase
sudo mv bin/branchbase /usr/local/bin/
```
